package usage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/runtime"
	"github.com/turbine-dev/pimpo/internal/store"
)

// Five days of a 07:00 routine: two runs finish, one fails and is fixed
// by the next, one is late but logged, one never happens (silent), and
// while paused nothing is expected.
func TestReportFindsSilentFailures(t *testing.T) {
	ctx := context.Background()
	ev, _ := event.Open(filepath.Join(t.TempDir(), "p.db"))
	defer ev.Close()
	st, _ := store.Open(ev.DB())
	day := func(d, h, m int) time.Time { return time.Date(2026, 9, d, h, m, 0, 0, time.UTC) }
	now := day(29, 12, 0)
	st.SaveRoutine(ctx, "brief", routine.Routine{Name: "Resumo", Code: "x", Manifest: runtime.Manifest{Schedule: "0 7 * * *", Capabilities: []string{"notify.send"}}}, "t", "owner")
	ev.DB().Exec(`UPDATE routines SET created_at = ? WHERE id = 'brief'`, day(21, 12, 0).Format(time.RFC3339Nano))
	clock := day(21, 12, 0)
	ev.SetClock(func() time.Time { return clock })
	at := func(t time.Time, typ string, data map[string]any) {
		clock = t
		ev.Append(ctx, typ, "routine:brief#1", data)
	}
	run := func(t time.Time, outcome, errText string) {
		ev.DB().Exec(`INSERT INTO runs (routine, version, started_at, ended_at, outcome, error, cost_usd, calls) VALUES ('brief', 1, ?, ?, ?, ?, 0.01, 2)`,
			t.Format(time.RFC3339Nano), t.Add(time.Second).Format(time.RFC3339Nano), outcome, errText)
		at(t, "routine.run.started", map[string]any{"routine": "brief", "trigger": "schedule"})
		if outcome == store.RunOK {
			at(t.Add(time.Second), "routine.run.finished", map[string]any{"routine": "brief"})
		} else {
			at(t.Add(time.Second), "routine.run.failed", map[string]any{"routine": "brief", "error": errText})
		}
	}
	at(day(21, 12, 0), "routine.created", map[string]any{"routine": "brief"})
	run(day(22, 7, 0), store.RunOK, "")                      // 22: fine
	run(day(23, 7, 1), store.RunFailed, "gmail.search: 401") // 23: failed, stops the routine
	at(day(23, 9, 0), "routine.resumed", map[string]any{"routine": "brief"})
	at(day(24, 7, 40), "routine.run.missed", map[string]any{"routine": "brief", "due": day(24, 7, 0), "action": "ran late"}) // 24: late, logged
	run(day(24, 7, 40), store.RunOK, "")
	// 25: nothing at all: silent
	at(day(26, 1, 0), "routine.paused", map[string]any{"routine": "brief"}) // 26-27: paused
	at(day(27, 20, 0), "routine.resumed", map[string]any{"routine": "brief"})
	run(day(28, 7, 0), store.RunOK, "")
	run(day(29, 7, 0), store.RunOK, "")
	clock = now
	at(day(28, 7, 0), "approval.requested", map[string]any{"id": "a1"})
	at(day(28, 7, 5), "approval.resolved", map[string]any{"id": "a1", "answer": "deny"})

	rep, err := Build(ctx, ev, st, now, 7, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Routines) != 1 {
		t.Fatalf("%+v", rep.Routines)
	}
	u := rep.Routines[0]
	if u.Runs != 4 || u.Failed != 1 || u.Late != 1 || u.Approvals.Asked != 1 || u.Approvals.Denied != 1 {
		t.Fatalf("counts %+v", u)
	}
	if len(u.Silent) != 1 || !u.Silent[0].Equal(day(25, 7, 0)) {
		t.Fatalf("silent %v (expected %d)", u.Silent, u.Expected)
	}
	if u.Expected != 5 { // 23 to 29 minus the paused 26 and 27; 22 is before the window
		t.Fatalf("expected %d", u.Expected)
	}
	if len(u.Failures) != 1 || !u.Failures[0].Fixed {
		t.Fatalf("failures %+v", u.Failures)
	}
	md := rep.Markdown(time.UTC)
	if !strings.Contains(md, "1 silent failures") || !strings.Contains(md, "due 2026-09-25 07:00") || !strings.Contains(md, "a later run finished") {
		t.Fatal(md)
	}
}

func TestTimesWhilePimpoWasOffAndImportedPausedRoutines(t *testing.T) {
	ctx := context.Background()
	ev, _ := event.Open(filepath.Join(t.TempDir(), "p.db"))
	defer ev.Close()
	st, _ := store.Open(ev.DB())
	day := func(d, h int) time.Time { return time.Date(2026, 9, d, h, 0, 0, 0, time.UTC) }
	st.SaveRoutine(ctx, "brief", routine.Routine{Name: "Resumo", Code: "x", Manifest: runtime.Manifest{Schedule: "0 7 * * *", Capabilities: []string{"notify.send"}}}, "t", "owner")
	st.SaveRoutine(ctx, "old", routine.Routine{Name: "Importada", Code: "x", Manifest: runtime.Manifest{Schedule: "0 8 * * *", Capabilities: []string{"notify.send"}}}, "t", "owner")
	st.SetRoutineState(ctx, "old", store.RoutinePaused)
	ev.DB().Exec(`UPDATE routines SET created_at = ?`, day(20, 0).Format(time.RFC3339Nano))
	clock := day(25, 9)
	ev.SetClock(func() time.Time { return clock })
	// Pimpo was off from the 22nd to the 25th: it last beat at 22 06:00.
	ev.Append(ctx, "system.started", "system", map[string]any{"last_alive": day(22, 6).Format(time.RFC3339)})
	clock = day(26, 12)
	ev.Append(ctx, "system.gap", "system", map[string]any{"from": day(26, 5).Format(time.RFC3339), "to": day(26, 8).Format(time.RFC3339)})
	rep, err := Build(ctx, ev, st, day(27, 12), 7, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	var brief Routine
	for _, u := range rep.Routines {
		if u.ID == "old" {
			t.Fatalf("a routine imported paused counts: %+v", u)
		}
		brief = u
	}
	// 21 to 27 at 07:00: 22, 23, 24 (off since 22 06:00, back 25 09:00), 25
	// and 26 (asleep 05:00-08:00) while off; 21 and 27 expected and silent.
	if brief.WhileOff != 5 || brief.Expected != 2 || len(brief.Silent) != 2 {
		t.Fatalf("off %d expected %d silent %v", brief.WhileOff, brief.Expected, brief.Silent)
	}
}

// Each person's report covers only their own routines.
func TestReportIsOnlyTheirsOwn(t *testing.T) {
	ctx := context.Background()
	ev, _ := event.Open(filepath.Join(t.TempDir(), "p.db"))
	defer ev.Close()
	st, _ := store.Open(ev.DB())
	for _, id := range []string{"mine", "anas"} {
		st.SaveRoutine(ctx, id, routine.Routine{Name: id, Code: "x", Manifest: runtime.Manifest{Schedule: "0 7 * * *", Capabilities: []string{"notify.send"}}}, "t", "owner")
	}
	st.SetRoutinePerson(ctx, "anas", "ana")
	names := func(person string) string {
		rep, err := BuildFor(ctx, ev, st, time.Now(), 7, time.UTC, person)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, u := range rep.Routines {
			out = append(out, u.ID)
		}
		return strings.Join(out, ",")
	}
	if got := names("owner"); got != "mine" {
		t.Fatalf("the owner's report: %s", got)
	}
	if got := names("ana"); got != "anas" {
		t.Fatalf("ana's report: %s", got)
	}
}
