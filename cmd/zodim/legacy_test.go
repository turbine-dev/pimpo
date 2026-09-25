package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestAdoptLegacyMovesDataAndRenamesDatabases(t *testing.T) {
	root := t.TempDir()
	old, dir := filepath.Join(root, ".vigia"), filepath.Join(root, ".zodim")
	for _, f := range []string{"vigia.db", "vigia.db-wal", "vigia.lock", "memory/owner.md", "snapshots/2026-09-01/vigia.db"} {
		p := filepath.Join(old, f)
		os.MkdirAll(filepath.Dir(p), 0o700)
		os.WriteFile(p, []byte(f), 0o600)
	}
	os.WriteFile(filepath.Join(old, "vigia.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600)
	if err := adoptLegacy(old, dir); err == nil {
		t.Fatal("moved the data of a Vigia that is still running")
	}
	os.Remove(filepath.Join(old, "vigia.pid"))
	if err := adoptLegacy(old, dir); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"zodim.db", "zodim.db-wal", "zodim.lock", "memory/owner.md", "snapshots/2026-09-01/zodim.db"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("old folder should be gone")
	}
	os.MkdirAll(old, 0o700)
	os.WriteFile(filepath.Join(old, "vigia.db"), []byte("stale"), 0o600)
	adoptLegacy(old, dir)
	if b, _ := os.ReadFile(filepath.Join(dir, "zodim.db")); string(b) != "vigia.db" {
		t.Error("an existing install must not be touched")
	}
}
