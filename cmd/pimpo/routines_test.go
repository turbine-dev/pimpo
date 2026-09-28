package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denerFernandes/pimpo/internal/event"
	"github.com/denerFernandes/pimpo/internal/repo"
	"github.com/denerFernandes/pimpo/internal/routine"
	"github.com/denerFernandes/pimpo/internal/runtime"
	"github.com/denerFernandes/pimpo/internal/store"
	"github.com/denerFernandes/pimpo/internal/trace"
)

// Imported routines are checked and arrive paused; one whose tests fail
// is left out, and importing again changes nothing.
func TestImportRoutines(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	ev, err := event.Open(filepath.Join(dir, "pimpo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ev.Close()
	st, _ := store.Open(ev.DB())
	one := 1
	good := routine.Routine{Name: "Olá", Manifest: runtime.Manifest{Schedule: "0 7 * * *", Capabilities: []string{"telegram.send"}},
		Code:  `async function run() { await telegram.send({text: "oi"}) }`,
		Tests: []routine.Test{{Name: "diz oi", Scenario: trace.Scenario{Now: "2026-09-24T07:00:00-03:00", Expect: []trace.Expect{{Capability: "telegram.send", Count: &one}}}}}}
	bad := good
	bad.Name = "Quebrada"
	bad.Code = `async function run() {}`
	folder := t.TempDir()
	repo.Write(folder, "ola", good)
	repo.Write(folder, "quebrada", bad)
	var out bytes.Buffer
	n, err := importRoutines(ctx, ev, st, folder, false, &out)
	if err != nil || n != 1 {
		t.Fatalf("%d %v\n%s", n, err, out.String())
	}
	if r, err := st.Routine(ctx, "ola"); err != nil || r.State != store.RoutinePaused {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := st.Routine(ctx, "quebrada"); err == nil {
		t.Fatal("installed a routine whose test fails")
	}
	out.Reset()
	if n, _ := importRoutines(ctx, ev, st, folder, false, &out); n != 0 || !strings.Contains(out.String(), "already installed") {
		t.Fatalf("%d %s", n, out.String())
	}
}
