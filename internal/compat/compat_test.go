// Package compat holds the promise of docs/FORMATS.md: data written by an
// earlier Pimpo opens in this one. Each frozen format has a fixture under
// testdata/, written once by the version that froze it and never changed;
// a change that breaks one of these tests breaks people's data.
package compat

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/turbine-dev/pimpo/internal/backup"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/connector/external"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/memory"
	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/store"
)

const pass = "compat-fixture"

type secrets map[string]string

func (s secrets) Names(context.Context) ([]string, error) {
	var out []string
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}
func (s secrets) Get(_ context.Context, k string) (string, error) { return s[k], nil }
func (s secrets) Set(_ context.Context, k, v string) error        { s[k] = v; return nil }

var fixtureRoutine = routine.Routine{
	Name:        "Resumo da manhã",
	Description: "Às 8h, manda um resumo dos e-mails não lidos.",
	Code:        "export default async function (ctx) { const m = await ctx.call('mail.search', {query: 'is:unread'}); await ctx.call('notify.owner', {text: m.length + ' não lidos'}) }",
}

// TestWriteFixtures writes the fixtures for the current format. It runs
// only when asked (PIMPO_WRITE_COMPAT=1), when a format is frozen.
func TestWriteFixtures(t *testing.T) {
	if os.Getenv("PIMPO_WRITE_COMPAT") == "" {
		t.Skip("set PIMPO_WRITE_COMPAT=1 to write the fixtures of a new format")
	}
	ctx := context.Background()
	home := t.TempDir()
	ev, err := event.Open(filepath.Join(home, "pimpo.db"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ev.DB())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveRoutine(ctx, "resumo-da-manha", fixtureRoutine, "compat fixture", "human:owner"); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{"exploration.started", "approval.resolved", "run.finished"} {
		if _, err := ev.Append(ctx, typ, "human:owner", map[string]string{"fixture": typ}); err != nil {
			t.Fatal(err)
		}
	}
	ev.Put(ctx, "fixture.key", "fixture value")
	mem, err := memory.Open(filepath.Join(home, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	mem.AddFor("Prefere respostas curtas.", "preferências", "owner", memory.High, "")
	mem.AddFor("Talvez goste de café.", "preferências", "aprendido: pediu café duas vezes", memory.Learned, "")
	dir := filepath.Join("testdata", itoa(event.Format))
	os.MkdirAll(dir, 0o755)
	var buf bytes.Buffer
	if _, err := backup.Export(ctx, ev.DB(), home, secrets{"fixture.token": "s3cret"}, pass, "fixture", &buf); err != nil {
		t.Fatal(err)
	}
	ev.Close()
	os.WriteFile(filepath.Join(dir, "backup.pimpo"), buf.Bytes(), 0o644)
	b, _ := json.MarshalIndent(fixtureRoutine, "", "  ")
	os.WriteFile(filepath.Join(dir, "routine.json"), b, 0o644)
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// TestOlderDataOpens opens every frozen fixture with this Pimpo.
func TestOlderDataOpens(t *testing.T) {
	dirs, _ := filepath.Glob(filepath.Join("testdata", "*"))
	if len(dirs) == 0 {
		t.Fatal("no fixtures")
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			ctx := context.Background()
			f, err := os.Open(filepath.Join(dir, "backup.pimpo"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			out := filepath.Join(t.TempDir(), "restored")
			// Fixtures of format 1 are not sealed as a whole; they open
			// with pimpo import --unsealed.
			_, sec, err := backup.UnpackUnsealed(f, out, pass)
			if err != nil {
				t.Fatalf("the backup does not open: %v", err)
			}
			if sec["fixture.token"] != "s3cret" {
				t.Fatalf("secrets = %v", sec)
			}
			ev, err := event.Open(filepath.Join(out, "pimpo.db"))
			if err != nil {
				t.Fatalf("the database does not open: %v", err)
			}
			defer ev.Close()
			if bad, err := ev.Verify(ctx); err != nil || bad != 0 {
				t.Fatalf("event chain broken at %d: %v", bad, err)
			}
			if evs, _ := ev.List(ctx, event.Query{}); len(evs) < 3 {
				t.Fatalf("events = %d", len(evs))
			}
			if v, _ := ev.Get(ctx, "fixture.key"); v != "fixture value" {
				t.Fatalf("kv = %q", v)
			}
			st, err := store.Open(ev.DB())
			if err != nil {
				t.Fatal(err)
			}
			r, err := st.Routine(ctx, "resumo-da-manha")
			if err != nil || r.Body.Code != fixtureRoutine.Code {
				t.Fatalf("routine: %+v, %v", r, err)
			}
			if cs, err := company.Open(ev.DB()); err != nil {
				t.Fatalf("company tables on an older database: %v", err)
			} else if list, err := cs.List(ctx); err != nil || len(list) != 0 {
				t.Fatalf("companies on an older database: %v, %v", list, err)
			}
			mem, err := memory.Open(filepath.Join(out, "memory"))
			if err != nil {
				t.Fatal(err)
			}
			facts, _ := mem.List()
			trust := map[memory.Trust]int{}
			for _, f := range facts {
				trust[f.Trust]++
			}
			if trust[memory.High] != 1 || trust[memory.Learned] != 1 {
				t.Fatalf("memory: %+v", facts)
			}
			var rt routine.Routine
			b, _ := os.ReadFile(filepath.Join(dir, "routine.json"))
			if err := json.Unmarshal(b, &rt); err != nil || rt.Code != fixtureRoutine.Code {
				t.Fatalf("routine file: %v", err)
			}
			if m, err := external.Load(filepath.Join(dir, "connectors", "tides")); err != nil || len(m.Capabilities) != 2 {
				t.Fatalf("connector: %+v, %v", m, err)
			}
		})
	}
}

// TestNewerDataIsRefused: a database from a newer format is left alone.
func TestNewerDataIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pimpo.db")
	ev, err := event.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ev.DB().Exec(`PRAGMA user_version = 99`)
	ev.Close()
	if _, err := event.Open(path); err == nil {
		t.Fatal("a newer database opened")
	}
}
