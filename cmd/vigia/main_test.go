package main

import (
	"context"
	"os"
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

func TestMigrateDryRunTouchesNothing(t *testing.T) {
	src, data := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(src, "memories"), 0o755)
	os.WriteFile(filepath.Join(src, "memories", "MEMORY.md"), []byte("Gym on Tuesdays"), 0o600)
	var out strings.Builder
	if err := migrateCmd([]string{"hermes", "--home", src, "--data", data}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "1 memories") || !strings.Contains(out.String(), "Nothing imported") {
		t.Fatalf("dry run:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(data, "vigia.db")); err == nil {
		t.Fatal("a dry run touched Vigia's data")
	}
}

// The starter gallery shipped in the binary must always verify.
func TestStarterGalleryVerifies(t *testing.T) {
	var out strings.Builder
	if err := galleryVerify("../../gallery/index.json", &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
}
