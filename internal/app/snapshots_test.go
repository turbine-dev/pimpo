package app

import (
	"context"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/owner"
	"github.com/turbine-dev/pimpo/internal/snapshot"
)

func TestSnapshotsFromTheApp(t *testing.T) {
	ta := newApp(t, nil, nil)
	ta.Home = t.TempDir()
	code, made := ta.do(t, "POST", "/api/snapshots", nil)
	if code != 200 || made["label"] != "manual" {
		t.Fatalf("create %d %v", code, made)
	}
	name := made["name"].(string)
	if code, _ := ta.do(t, "POST", "/api/snapshots/restore", map[string]string{"name": "../../etc"}); code != 404 {
		t.Fatalf("staged a path: %d", code)
	}
	if code, out := ta.do(t, "POST", "/api/snapshots/restore", map[string]string{"name": name}); code != 200 || out["restart"] != true {
		t.Fatalf("stage %d %v", code, out)
	}
	_, list := ta.do(t, "GET", "/api/snapshots", nil)
	if list["staged"] != name || len(list["snapshots"].([]any)) != 1 {
		t.Fatalf("list %v", list)
	}
	ta.do(t, "DELETE", "/api/snapshots/restore", nil)
	if snapshot.Staged(ta.Home) != "" {
		t.Fatal("restore not cancelled")
	}
}

func TestUpgradeIsAnnouncedOnce(t *testing.T) {
	ta := newApp(t, nil, nil)
	ctx := context.Background()
	ta.Events.Put(ctx, UpgradeKey, `{"from":"0.9.0","to":"1.0.0","snapshot":"x"}`)
	ta.announceUpgrade(ctx)
	ta.announceUpgrade(ctx)
	evs, _ := ta.Events.List(ctx, event.Query{Types: []string{owner.EventNotice}})
	if len(evs) != 1 || !strings.Contains(string(evs[0].Data), "0.9.0 para 1.0.0") {
		t.Fatalf("notices %d", len(evs))
	}
}

func TestDamagedDatabaseIsNotSnapshotted(t *testing.T) {
	ta := newApp(t, nil, nil)
	ta.Home = t.TempDir()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		ta.Events.Append(ctx, "note", "system", map[string]int{"n": i})
	}
	ta.Events.DB().Exec(`UPDATE events SET data = '{"n":99}' WHERE id = (SELECT MAX(id) FROM events) - 1`)
	if code, _ := ta.do(t, "POST", "/api/snapshots", nil); code != 409 {
		t.Fatalf("snapshot of a damaged database: %d", code)
	}
	if code, _ := ta.do(t, "POST", "/api/snapshots", nil); code != 409 {
		t.Fatalf("second snapshot: %d", code)
	}
	if list, _ := snapshot.List(ta.Home); len(list) != 0 {
		t.Fatalf("copied: %+v", list)
	}
	evs, _ := ta.Events.List(ctx, event.Query{Types: []string{owner.EventNotice}})
	if len(evs) != 1 {
		t.Fatalf("the owner was told %d times", len(evs))
	}
	var db finding
	for _, f := range ta.localFindings(ctx) {
		if f.ID == "database" {
			db = f
		}
	}
	if db.State != "fail" || db.Fix != "doc.fix.database" {
		t.Fatalf("check-up %+v", db)
	}
	if _, st := ta.do(t, "GET", "/api/state", nil); st["healthy"] != false {
		t.Fatalf("state %v", st)
	}
}
