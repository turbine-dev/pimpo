package snapshot

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/turbine-dev/pimpo/internal/event"
)

func TestCreateListRestore(t *testing.T) {
	home := t.TempDir()
	ev, _ := event.Open(filepath.Join(home, "pimpo.db"))
	ctx := context.Background()
	ev.Put(ctx, "marker", "before")
	os.MkdirAll(filepath.Join(home, "memory"), 0o700)
	os.WriteFile(filepath.Join(home, "memory", "facts.json"), []byte(`[{"text":"old"}]`), 0o600)

	s, err := Create(ev.DB(), home, "before 0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if s.Label != "before-0.2.0" || s.Bytes == 0 {
		t.Fatalf("snapshot %+v", s)
	}
	ev.Put(ctx, "marker", "after")
	os.WriteFile(filepath.Join(home, "memory", "facts.json"), []byte(`[{"text":"new"}]`), 0o600)

	if err := Restore(home, s.Name, ev.DB()); err != nil {
		t.Fatal(err)
	}
	ev2, err := event.Open(filepath.Join(home, "pimpo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ev2.Close()
	if v, _ := ev2.Get(ctx, "marker"); v != "before" {
		t.Fatalf("database not restored: %q", v)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "memory", "facts.json")); string(b) != `[{"text":"old"}]` {
		t.Fatalf("memory not restored: %s", b)
	}
	list, _ := List(home)
	if len(list) != 2 || list[0].Label != "before-restore" {
		t.Fatalf("list %+v", list)
	}
	if err := Restore(home, "nope", nil); err == nil {
		t.Fatal("restored a missing snapshot")
	}
}

func TestStage(t *testing.T) {
	home := t.TempDir()
	ev, _ := event.Open(filepath.Join(home, "pimpo.db"))
	defer ev.Close()
	s, err := Create(ev.DB(), home, "before-2.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "../x", "nope", s.Name + "/.."} {
		if Stage(home, bad) == nil {
			t.Fatalf("staged %q", bad)
		}
	}
	if err := Stage(home, s.Name); err != nil || Staged(home) != s.Name {
		t.Fatalf("stage: %v %q", err, Staged(home))
	}
	Unstage(home)
	if Staged(home) != "" {
		t.Fatal("still staged")
	}
}

// A restore brings back the data, not a device, passkey or person that
// was removed since.
func TestRestoreKeepsWhoMaySignIn(t *testing.T) {
	home := t.TempDir()
	ev, _ := event.Open(filepath.Join(home, "pimpo.db"))
	ctx := context.Background()
	ev.Put(ctx, "devices", `[{"name":"lost phone"}]`)
	ev.Put(ctx, "passkeys", `[{"id":"old"}]`)
	ev.Put(ctx, "people", `[{"id":"ana"},{"id":"ex"}]`)
	ev.Put(ctx, "marker", "before")
	s, err := Create(ev.DB(), home, "manual")
	if err != nil {
		t.Fatal(err)
	}
	ev.Put(ctx, "devices", `[]`)
	ev.DB().Exec(`DELETE FROM kv WHERE key = 'passkeys'`)
	ev.Put(ctx, "people", `[{"id":"ana"}]`)
	ev.Put(ctx, "session_token", "rotated")
	if err := Restore(home, s.Name, ev.DB()); err != nil {
		t.Fatal(err)
	}
	ev2, _ := event.Open(filepath.Join(home, "pimpo.db"))
	defer ev2.Close()
	if v, _ := ev2.Get(ctx, "marker"); v != "before" {
		t.Fatalf("database not restored: %q", v)
	}
	want := map[string]string{"devices": `[]`, "passkeys": "", "people": `[{"id":"ana"}]`, "session_token": "rotated"}
	for k, w := range want {
		if v, _ := ev2.Get(ctx, k); v != w {
			t.Fatalf("%s came back as %q, want %q", k, v, w)
		}
	}
}

func TestSnapshotsSkipSymlinks(t *testing.T) {
	home := t.TempDir()
	ev, _ := event.Open(filepath.Join(home, "pimpo.db"))
	defer ev.Close()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(outside, []byte("private"), 0o600)
	os.MkdirAll(filepath.Join(home, "memory"), 0o700)
	os.WriteFile(filepath.Join(home, "memory", "facts.json"), []byte(`[]`), 0o600)
	if err := os.Symlink(outside, filepath.Join(home, "memory", "link.txt")); err != nil {
		t.Skip("no symlinks here")
	}
	s, err := Create(ev.DB(), home, "manual")
	if err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(dir(home), s.Name, "memory")
	if _, err := os.Lstat(filepath.Join(copied, "link.txt")); err == nil {
		t.Fatal("followed a symlink out of the memory folder")
	}
	if _, err := os.Stat(filepath.Join(copied, "facts.json")); err != nil {
		t.Fatal("memory not copied")
	}
}
