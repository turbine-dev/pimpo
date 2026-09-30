package scheduler

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/connector"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/runtime"
	"github.com/turbine-dev/pimpo/internal/store"
)

type tg struct {
	mu   sync.Mutex
	sent []string
	fail bool
}

func (t *tg) Capabilities() []string { return []string{"telegram.send"} }
func (t *tg) Call(_ context.Context, _, _ string, args any) (any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sent = append(t.sent, args.(map[string]any)["text"].(string))
	return map[string]bool{"ok": true}, nil
}

type notes struct {
	mu   sync.Mutex
	list []explore.Notice
}

func (n *notes) Notify(_ context.Context, x explore.Notice) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.list = append(n.list, x)
	return nil
}

func setup(t *testing.T, code string) (*Scheduler, *tg, *notes) {
	t.Helper()
	ev, err := event.Open(filepath.Join(t.TempDir(), "v.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ev.Close() })
	st, _ := store.Open(ev.DB())
	st.SaveRoutine(context.Background(), "brief", routine.Routine{Name: "Resumo matinal", Code: code,
		Manifest: runtime.Manifest{Schedule: "0 7 * * *", Capabilities: []string{"telegram.send"}}}, "test", "owner")
	bot := &tg{}
	n := &notes{}
	return &Scheduler{Env: host.Env{Router: connector.NewRouter(bot), Events: ev}, Store: st, Notify: n, Zone: time.UTC}, bot, n
}

func TestRunNowRecordsTheRun(t *testing.T) {
	s, bot, _ := setup(t, `async function run() { await telegram.send({text: "bom dia " + dates.format(now(), "dd/MM")}); }`)
	s.Now = func() time.Time { return time.Date(2026, 9, 24, 7, 0, 0, 0, time.UTC) }
	run, err := s.RunNow(context.Background(), "brief", "manual")
	if err != nil || run.Outcome != store.RunOK || run.Calls != 1 {
		t.Fatalf("%+v %v", run, err)
	}
	if bot.sent[0] != "bom dia 24/09" {
		t.Fatalf("sent %v", bot.sent)
	}
	runs, _ := s.Store.Runs(context.Background(), "brief", 5)
	if len(runs) != 1 || runs[0].Outcome != store.RunOK {
		t.Fatalf("runs %+v", runs)
	}
}

func TestFailureAlertsAndPauses(t *testing.T) {
	s, _, n := setup(t, `async function run() { throw new Error("formato da API mudou"); }`)
	if _, err := s.RunNow(context.Background(), "brief", "manual"); err == nil {
		t.Fatal("failure not reported")
	}
	r, _ := s.Store.Routine(context.Background(), "brief")
	if r.State != store.RoutineBroken {
		t.Fatalf("state %s", r.State)
	}
	if len(n.list) != 1 || !strings.Contains(n.list[0].Text, "formato da API mudou") || n.list[0].Actions[1].Data != "repair:brief" {
		t.Fatalf("alert %+v", n.list)
	}
}

func TestCatchUpRunsARecentlyMissedRunOnce(t *testing.T) {
	s, bot, _ := setup(t, `async function run() { await telegram.send({text: "ok"}); }`)
	// Created yesterday, never ran; today's 07:00 passed two hours ago.
	s.Store.DB().Exec(`UPDATE routines SET created_at = ?`, time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano))
	s.Now = func() time.Time { return time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	s.Wait()
	if len(bot.sent) != 1 {
		t.Fatalf("catch-up sent %d messages", len(bot.sent))
	}
	if next := s.Next("brief"); next.IsZero() {
		t.Fatal("routine not scheduled")
	}
}

func TestPausedRoutinesAreNotScheduled(t *testing.T) {
	s, _, _ := setup(t, `async function run() {}`)
	s.Store.SetRoutineState(context.Background(), "brief", store.RoutinePaused)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	if !s.Next("brief").IsZero() {
		t.Fatal("paused routine is scheduled")
	}
}

func TestWebhookOnlyRoutinesAreNotScheduledOrFailed(t *testing.T) {
	s, _, _ := setup(t, `async function run() {}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Store.SaveRoutine(ctx, "hook", routine.Routine{Name: "Pedido novo", Code: `async function run() {}`,
		Manifest: runtime.Manifest{Webhook: true, Capabilities: []string{"telegram.send"}}}, "test", "owner")
	s.Start(ctx)
	s.Changed(ctx, "hook")
	if !s.Next("hook").IsZero() {
		t.Fatal("a routine without a schedule got one")
	}
	evs, _ := s.Env.Events.List(ctx, event.Query{Types: []string{EventRunFailed}})
	if len(evs) != 0 {
		t.Fatalf("false failures: %+v", evs)
	}
}

// A run reports its start, each step by the capability's name, and its end.
func TestRunReportsItsProgress(t *testing.T) {
	s, _, _ := setup(t, `async function run() { await telegram.send({text: "um"}); await telegram.send({text: "dois"}); }`)
	var mu sync.Mutex
	var seen []RunProgress
	s.Progress = func(_ context.Context, p RunProgress) {
		mu.Lock()
		seen = append(seen, p)
		mu.Unlock()
	}
	if _, err := s.RunNow(context.Background(), "brief", "manual"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 4 || seen[0].State != store.RunRunning || seen[0].Name != "Resumo matinal" {
		t.Fatalf("%+v", seen)
	}
	if seen[2].Steps != 2 || seen[2].Step != "telegram.send" || seen[3].State != store.RunOK || seen[3].Steps != 2 {
		t.Fatalf("%+v", seen)
	}
}
