package snapshot

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
)

// withEvents makes a database spanning many pages, with a marker value.
func withEvents(t *testing.T, path, marker string) {
	t.Helper()
	ev, err := event.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 200; i++ {
		ev.Append(ctx, "note", "system", map[string]string{"text": strings.Repeat("x", 200)})
	}
	ev.Put(ctx, "marker", marker)
	ev.Put(ctx, "session_token", "token-"+marker)
	ev.Close()
}

// scribble overwrites pages in the middle of a database file with garbage,
// as a failing disk would.
func scribble(t *testing.T, path string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteAt(bytes.Repeat([]byte{0xde, 0xad, 0xbe, 0xef}, 2048), 4096*3)
}

func marker(t *testing.T, path string) string {
	t.Helper()
	ev, err := event.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ev.Close()
	v, _ := ev.Get(context.Background(), "marker")
	return v
}

// snap makes a snapshot of a database with the given marker; damaged
// scribbles over the copy afterwards.
func snap(t *testing.T, home, name, value string, damaged bool) {
	t.Helper()
	dir := filepath.Join(home, "snapshots", name)
	os.MkdirAll(dir, 0o700)
	withEvents(t, filepath.Join(dir, "pimpo.db"), value)
	if damaged {
		scribble(t, filepath.Join(dir, "pimpo.db"))
	}
}

func TestSnapshotRefusesADamagedDatabase(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "pimpo.db")
	withEvents(t, path, "now")
	scribble(t, path)
	db, _ := sql.Open("sqlite", path)
	defer db.Close()
	if _, err := Create(db, home, "daily"); !errors.Is(err, event.ErrDamaged) {
		t.Fatalf("want a refusal, got %v", err)
	}
	if list, _ := List(home); len(list) != 0 {
		t.Fatalf("a damaged database was copied: %+v", list)
	}
}

func TestDamagedSnapshotsAreMarkedAndSkipped(t *testing.T) {
	home := t.TempDir()
	snap(t, home, "20260901-100000-daily", "old", false)
	snap(t, home, "20260902-100000-daily", "newer", false)
	snap(t, home, "20260903-100000-daily", "newest", true)
	list, _ := Checked(home)
	if len(list) != 3 || !list[0].Damaged || list[1].Damaged || list[2].Damaged {
		t.Fatalf("checked %+v", list)
	}
	if s, ok := NewestGood(home); !ok || s.Name != "20260902-100000-daily" {
		t.Fatalf("newest good %+v", s)
	}
	if err := Stage(home, "20260903-100000-daily"); !errors.Is(err, event.ErrDamaged) {
		t.Fatalf("staged a damaged copy: %v", err)
	}
	if err := Restore(home, "20260903-100000-daily", nil); !errors.Is(err, event.ErrDamaged) {
		t.Fatalf("restored a damaged copy: %v", err)
	}
}

func TestQuarantineAndRecoverKeepTheDamagedFile(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "pimpo.db")
	withEvents(t, path, "damaged")
	os.WriteFile(path+"-wal", []byte("journal"), 0o600)
	scribble(t, path)
	damaged, _ := os.ReadFile(path)
	snap(t, home, "20260902-100000-daily", "good", false)
	snap(t, home, "20260903-100000-daily", "bad", true)

	rec, err := Quarantine(home, "the database is damaged: test")
	if err != nil {
		t.Fatal(err)
	}
	if exists(path) || exists(path+"-wal") {
		t.Fatal("the damaged files were left in place")
	}
	if got, ok := Recovering(home); !ok || got.Folder != rec.Folder || got.Token == "" {
		t.Fatalf("recovering %+v %v", got, ok)
	}
	if err := rec.RecoverFrom(home, "20260903-100000-daily"); !errors.Is(err, event.ErrDamaged) {
		t.Fatalf("recovered from a damaged copy: %v", err)
	}
	// Something opened pimpo.db meanwhile: it is set aside too, not lost.
	os.WriteFile(path, []byte("made meanwhile"), 0o600)

	good, _ := NewestGood(home)
	if err := rec.RecoverFrom(home, good.Name); err != nil {
		t.Fatal(err)
	}
	if v := marker(t, path); v != "good" {
		t.Fatalf("restored %q", v)
	}
	folder := filepath.Dir(rec.Quarantined(home))
	if b, _ := os.ReadFile(rec.Quarantined(home)); !bytes.Equal(b, damaged) {
		t.Fatal("the damaged file was changed")
	}
	if b, _ := os.ReadFile(filepath.Join(folder, "2-pimpo.db")); string(b) != "made meanwhile" {
		t.Fatal("the file made meanwhile was replaced")
	}
	if b, _ := os.ReadFile(filepath.Join(folder, "pimpo.db-wal")); string(b) != "journal" {
		t.Fatal("the journal was not kept")
	}
	if _, ok := Recovering(home); ok {
		t.Fatal("still in recovery")
	}
}

func TestStartFreshAndExport(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "pimpo.db")
	withEvents(t, path, "damaged")
	scribble(t, path)
	rec, err := Quarantine(home, "damaged")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := rec.Export(home, &buf); err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, f := range z.File {
		names = append(names, f.Name)
	}
	if !strings.Contains(strings.Join(names, ","), "pimpo.db") {
		t.Fatalf("export has %v", names)
	}
	if err := rec.StartFresh(home); err != nil {
		t.Fatal(err)
	}
	if _, ok := Recovering(home); ok || !exists(rec.Quarantined(home)) {
		t.Fatal("start fresh must end the recovery and keep the damaged file")
	}
}

func TestSalvageReadsWhoMaySignIn(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "pimpo.db")
	withEvents(t, path, "x")
	ev, _ := event.Open(path)
	ev.Put(context.Background(), "devices", `[{"hash":"aa","created":"`+time.Now().Format(time.RFC3339)+`"},{"hash":"bb","person":"ana"},{"hash":"cc","invite":true}]`)
	ev.Close()
	rec, _ := Quarantine(home, "damaged")
	token, hashes := rec.Salvage(home)
	if token != "token-x" || len(hashes) != 1 || hashes[0] != "aa" {
		t.Fatalf("salvaged %q %v", token, hashes)
	}
}
