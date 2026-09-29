package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/repo"
	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/store"

	// Routines may call the services' capabilities; their specs must be
	// known for the audit.
	_ "github.com/turbine-dev/pimpo/internal/connector/services"
)

func routinesCmd(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "import" {
		return errors.New("usage: pimpo routines import [--active] FOLDER")
	}
	fs := flag.NewFlagSet("routines import", flag.ContinueOnError)
	dir := fs.String("data", "", "data directory (default ~/.pimpo)")
	active := fs.Bool("active", false, "install them active instead of paused")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: pimpo routines import [--active] FOLDER")
	}
	ev, err := event.Open(filepath.Join(dataDir(*dir), "pimpo.db"))
	if err != nil {
		return err
	}
	defer ev.Close()
	st, err := store.Open(ev.DB())
	if err != nil {
		return err
	}
	n, err := importRoutines(context.Background(), ev, st, fs.Arg(0), *active, out)
	if err == nil {
		fmt.Fprintf(out, "\n%d routines installed.\n", n)
	}
	return err
}

// importRoutines installs the routines of a folder in the repository's
// layout after the same checks a routine from the repository gets:
// manifest, its own tests, and an audit of what it really calls. They
// arrive paused unless active is set, so nothing runs before the owner
// reviews its settings. A running Pimpo lists them at once; the scheduler
// picks one up when it is resumed.
func importRoutines(ctx context.Context, ev *event.Store, st *store.Store, folder string, active bool, out io.Writer) (int, error) {
	found, broken := repo.Read(folder)
	for id, why := range broken {
		fmt.Fprintf(out, "✗ %s: %s\n", id, why)
	}
	if len(found) == 0 {
		return 0, fmt.Errorf("no routines in %s (expected %s/<id>/routine.json)", folder, repo.Dir)
	}
	n := 0
	for _, id := range repo.Sorted(found) {
		r := found[id]
		if cur, err := st.Routine(ctx, id); err == nil && repo.Hash(cur.Body) == repo.Hash(r) {
			fmt.Fprintf(out, "= %s: already installed\n", r.Name)
			continue
		}
		if problems := checkRoutine(ctx, r); len(problems) > 0 {
			fmt.Fprintf(out, "✗ %s: %s\n", r.Name, strings.Join(problems, "; "))
			continue
		}
		if _, err := st.SaveRoutine(ctx, id, r, "imported from "+folder, "human:owner"); err != nil {
			return n, err
		}
		state := store.RoutineActive
		if !active {
			state = store.RoutinePaused
			if err := st.SetRoutineState(ctx, id, state); err != nil {
				return n, err
			}
		}
		ev.Append(ctx, "routine.imported", "human:owner", map[string]string{"routine": id, "state": state})
		fmt.Fprintf(out, "✓ %s (%s)\n", r.Name, state)
		n++
	}
	return n, nil
}

func checkRoutine(ctx context.Context, r routine.Routine) []string {
	if err := r.Manifest.Validate(); err != nil {
		return []string{"manifest: " + err.Error()}
	}
	var problems []string
	for _, t := range r.Tests {
		if o := routine.Check(ctx, r, t.Name, t.Scenario); !o.Passed {
			problems = append(problems, "test "+t.Name+": "+strings.Join(o.Problems, "; "))
		}
	}
	_, audit := routine.Audit(ctx, r)
	return append(problems, audit...)
}
