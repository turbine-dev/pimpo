package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denerFernandes/vigia/internal/event"
	"github.com/denerFernandes/vigia/internal/snapshot"
)

func TestNewVersionTakesASnapshotFirst(t *testing.T) {
	home := t.TempDir()
	s, _ := event.Open(filepath.Join(home, "vigia.db"))
	defer s.Close()
	ctx := context.Background()
	version = "0.3.0"
	if err := guardVersion(ctx, s, home); err != nil {
		t.Fatal(err)
	}
	if list, _ := snapshot.List(home); len(list) != 0 {
		t.Fatal("first run should not snapshot")
	}
	version = "0.4.0"
	if err := guardVersion(ctx, s, home); err != nil {
		t.Fatal(err)
	}
	list, _ := snapshot.List(home)
	if len(list) != 1 || !strings.Contains(list[0].Label, "before-0.4.0") {
		t.Fatalf("snapshots %+v", list)
	}
	if err := guardVersion(ctx, s, home); err != nil {
		t.Fatal(err)
	}
	if list, _ := snapshot.List(home); len(list) != 1 {
		t.Fatal("same version snapshotted again")
	}
}

func TestUnknownCommand(t *testing.T) {
	if err := run([]string{"nope"}); err == nil || !strings.Contains(err.Error(), "snapshots") {
		t.Fatalf("got %v", err)
	}
}
