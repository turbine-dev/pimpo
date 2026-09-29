package event

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "pimpo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestChainDetectsTampering(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	for i := range 3 {
		if _, err := s.Append(ctx, "routine.ran", "routine:brief", map[string]int{"n": i}); err != nil {
			t.Fatal(err)
		}
	}
	if bad, err := s.Verify(ctx); err != nil || bad != 0 {
		t.Fatalf("intact log flagged at %d: %v", bad, err)
	}
	if _, err := s.DB().Exec(`UPDATE events SET data = '{"n":99}' WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	if bad, _ := s.Verify(ctx); bad != 2 {
		t.Fatalf("tampered event not found, got %d", bad)
	}
}

func TestListFiltersAndSubscribe(t *testing.T) {
	s := open(t)
	s.SetClock(func() time.Time { return time.Date(2026, 9, 24, 7, 0, 0, 0, time.UTC) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := s.Subscribe(ctx)
	s.Append(ctx, "a", "system", map[string]string{"msg": "hello world"})
	s.Append(ctx, "b", "system", map[string]string{"msg": "other"})
	if got := <-sub; got.Type != "a" || got.Time.Hour() != 7 {
		t.Fatalf("subscriber got %+v", got)
	}
	evs, _ := s.List(ctx, Query{Types: []string{"b"}})
	if len(evs) != 1 || evs[0].Type != "b" {
		t.Fatalf("type filter %+v", evs)
	}
	evs, _ = s.List(ctx, Query{Search: "hello", Newest: true, Limit: 5})
	if len(evs) != 1 || evs[0].Type != "a" {
		t.Fatalf("search %+v", evs)
	}
	s.Put(ctx, "k", "v1")
	s.Put(ctx, "k", "v2")
	if v, _ := s.Get(ctx, "k"); v != "v2" {
		t.Fatalf("kv %q", v)
	}
}

func TestOpenCreatesTheDataFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "pimpo")
	s, err := Open(filepath.Join(dir, "pimpo.db"))
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		t.Fatalf("folder %v %v", fi, err)
	}
	// Windows has no Unix permissions to check.
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o700 {
		t.Fatalf("folder mode %v", fi.Mode().Perm())
	}
}
