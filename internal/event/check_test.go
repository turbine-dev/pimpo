package event

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// filled makes a database with enough events to span many pages.
func filled(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pimpo.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 300; i++ {
		if _, err := s.Append(ctx, "note", "system", map[string]string{"text": strings.Repeat("x", 200)}); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	return path
}

// scribble overwrites part of a database file with garbage, as a failing
// disk or a cut power supply would.
func scribble(t *testing.T, path string, at int64, n int) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteAt([]byte(strings.Repeat("\xde\xad\xbe\xef", n/4)), at); err != nil {
		t.Fatal(err)
	}
}

func TestCheckPassesAHealthyDatabase(t *testing.T) {
	path := filled(t)
	if err := CheckFile(path); err != nil {
		t.Fatal(err)
	}
	if err := CheckFile(filepath.Join(t.TempDir(), "none.db")); err != nil {
		t.Fatalf("a database not made yet is not damaged: %v", err)
	}
}

func TestCheckFindsADamagedFile(t *testing.T) {
	for name, at := range map[string]int64{"header": 0, "pages": 4096 * 3} {
		t.Run(name, func(t *testing.T) {
			path := filled(t)
			before, _ := os.ReadFile(path)
			scribble(t, path, at, 4096*2)
			scribbled, _ := os.ReadFile(path)
			if err := CheckFile(path); !errors.Is(err, ErrDamaged) {
				t.Fatalf("want damaged, got %v", err)
			}
			if after, _ := os.ReadFile(path); string(after) != string(scribbled) || len(after) != len(before) {
				t.Fatal("the check wrote to the damaged file")
			}
		})
	}
}

func TestCheckFindsABrokenHistoryTail(t *testing.T) {
	path := filled(t)
	s, _ := Open(path)
	s.DB().Exec(`UPDATE events SET data = '{"text":"edited"}' WHERE id = (SELECT MAX(id) FROM events) - 3`)
	err := Check(context.Background(), s.DB())
	s.Close()
	if !errors.Is(err, ErrDamaged) || !strings.Contains(err.Error(), "history") {
		t.Fatalf("want a broken history, got %v", err)
	}
}

func TestCheckReadsALeftoverJournal(t *testing.T) {
	path := filled(t)
	s, _ := Open(path)
	s.Append(context.Background(), "note", "system", map[string]string{"text": "after"})
	// Copy the files while the store is open, as a crash would leave them.
	crashed := filepath.Join(t.TempDir(), "pimpo.db")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if b, err := os.ReadFile(path + suffix); err == nil {
			os.WriteFile(crashed+suffix, b, 0o600)
		}
	}
	s.Close()
	if _, err := os.Stat(crashed + "-wal"); err != nil {
		t.Skip("no journal left to test with")
	}
	os.Remove(crashed + "-shm")
	if err := CheckFile(crashed); err != nil {
		t.Fatalf("a journal left by a crash is not damage: %v", err)
	}
}
