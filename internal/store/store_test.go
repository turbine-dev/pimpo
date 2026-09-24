package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/denerFernandes/vigia/internal/event"
	"github.com/denerFernandes/vigia/internal/routine"
	"github.com/denerFernandes/vigia/internal/runtime"
	"github.com/denerFernandes/vigia/internal/trace"
)

func open(t *testing.T) *Store {
	t.Helper()
	ev, err := event.Open(filepath.Join(t.TempDir(), "v.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ev.Close() })
	s, err := Open(ev.DB())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRoutineVersions(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	body := routine.Routine{Name: "Resumo", Code: "v1", Manifest: runtime.Manifest{Capabilities: []string{"telegram.send"}}}
	r, err := s.SaveRoutine(ctx, "brief", body, "compiled from exploration e1", "owner")
	if err != nil || r.Version != 1 || r.State != RoutineActive {
		t.Fatalf("%+v %v", r, err)
	}
	s.SetRoutineState(ctx, "brief", RoutineBroken)
	body.Code = "v2"
	r, _ = s.SaveRoutine(ctx, "brief", body, "repair: API changed", "owner")
	if r.Version != 2 || r.Body.Code != "v2" || r.State != RoutineActive {
		t.Fatalf("after repair %+v", r)
	}
	vs, _ := s.Versions(ctx, "brief")
	if len(vs) != 2 || vs[1].Body.Code != "v1" || vs[0].Reason != "repair: API changed" {
		t.Fatalf("versions %+v", vs)
	}
	if _, err := s.Routine(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("missing: %v", err)
	}
	if err := s.SetRoutineState(ctx, "nope", RoutinePaused); err != ErrNotFound {
		t.Fatalf("missing state: %v", err)
	}
}

func TestExplorationsAndRuns(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	e := Exploration{ID: "e1", Request: "resumo matinal", State: ExplorationRunning}
	s.SaveExploration(ctx, e)
	e.State, e.Summary, e.Trace = ExplorationReady, "Mandei o resumo", &trace.Trace{ID: "e1", Request: "resumo matinal"}
	s.SaveExploration(ctx, e)
	got, _ := s.Exploration(ctx, "e1")
	if got.State != ExplorationReady || got.Trace == nil || got.Summary != "Mandei o resumo" {
		t.Fatalf("%+v", got)
	}
	ready, _ := s.Explorations(ctx, ExplorationReady)
	if len(ready) != 1 {
		t.Fatalf("ready %+v", ready)
	}
	id, _ := s.StartRun(ctx, "brief", 1)
	s.FinishRun(ctx, id, RunOK, "", 0.002, 4)
	runs, _ := s.Runs(ctx, "brief", 10)
	if len(runs) != 1 || runs[0].Outcome != RunOK || runs[0].Calls != 4 || runs[0].EndedAt.IsZero() {
		t.Fatalf("runs %+v", runs)
	}
	c, _ := s.RunCostSince(ctx, "brief", time.Now().Add(-time.Hour))
	if c != 0.002 {
		t.Fatalf("cost %v", c)
	}
}

func TestOldDatabasesGainNewColumns(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "old.db"))
	defer ev.Close()
	ev.DB().Exec(`CREATE TABLE explorations (id TEXT PRIMARY KEY, request TEXT NOT NULL, state TEXT NOT NULL, trace TEXT, summary TEXT, routine TEXT, cost_usd REAL NOT NULL DEFAULT 0, error TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`)
	s, err := Open(ev.DB())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveExploration(context.Background(), Exploration{ID: "e", Request: "r", State: ExplorationReady, Candidate: &routine.Routine{Code: "x"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ev.DB()); err != nil {
		t.Fatalf("reopening an up-to-date database: %v", err)
	}
}
