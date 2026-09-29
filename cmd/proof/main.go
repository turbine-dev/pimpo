// Command proof runs the F0 compiler proof: compile every recorded task in
// a directory and check each routine against the recording, its own tests,
// and a holdout scenario the compiler never saw.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/compiler"
	// The services' capabilities (RSS, GitHub, Todoist, Home Assistant…)
	// join the catalog the compiler sees, as they do in the app.
	_ "github.com/turbine-dev/pimpo/internal/connector/services"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/repo"
	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/trace"
)

type result struct {
	ID            string             `json:"id"`
	FirstAccepted bool               `json:"first_accepted"`
	FirstHoldout  bool               `json:"first_holdout"`
	Accepted      bool               `json:"accepted"`
	Holdout       *routine.Outcome   `json:"holdout,omitempty"`
	Attempts      []compiler.Attempt `json:"attempts"`
	Seconds       float64            `json:"seconds"`
	Error         string             `json:"error,omitempty"`
}

func main() {
	dir := flag.String("dir", "testdata/proof", "directory of traces")
	out := flag.String("out", "docs/proof", "where to write the report")
	model := flag.String("model", "sonnet", "model for the compiler")
	workers := flag.Int("workers", 3, "parallel compilations")
	only := flag.String("only", "", "run only traces whose id contains one of these, comma-separated")
	attempts := flag.Int("attempts", 3, "compile attempts, with feedback, as the app makes")
	export := flag.String("export", "", "also write the accepted routines that pass the holdout to this folder, in the repository layout (pimpo routines import reads it)")
	reuse := flag.Bool("reuse", false, "compile nothing: export from the results already in -out")
	flag.Parse()
	if *reuse {
		var saved []result
		b, err := os.ReadFile(filepath.Join(*out, "results.json"))
		if err == nil {
			err = json.Unmarshal(b, &saved)
		}
		if err != nil || *export == "" {
			fmt.Fprintln(os.Stderr, "-reuse needs -export and a results.json in -out:", err)
			os.Exit(1)
		}
		exportRoutines(*export, saved)
		return
	}

	paths, _ := filepath.Glob(filepath.Join(*dir, "*.json"))
	sort.Strings(paths)
	c := compiler.Compiler{Model: llm.ClaudeCLI{Model: *model}, Attempts: *attempts}
	results := make([]result, len(paths))
	var wg sync.WaitGroup
	sem := make(chan struct{}, *workers)
	for i, p := range paths {
		t, err := trace.Load(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if *only != "" && !slices.ContainsFunc(strings.Split(*only, ","), func(o string) bool { return strings.Contains(t.ID, o) }) {
			continue
		}
		wg.Add(1)
		go func(i int, t trace.Trace) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = run(c, t)
			r := results[i]
			fmt.Printf("%-24s first=%v holdout=%v final=%v (%.0fs)%s\n", r.ID, r.FirstAccepted, r.FirstHoldout, r.Accepted, r.Seconds, errSuffix(r.Error))
		}(i, t)
	}
	wg.Wait()
	write(*out, results)
	if *export != "" {
		exportRoutines(*export, results)
	}
}

func run(c compiler.Compiler, t trace.Trace) result {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	attempts, err := c.Compile(ctx, t)
	r := result{ID: t.ID, Attempts: attempts, Seconds: time.Since(start).Seconds()}
	if err != nil {
		r.Error = err.Error()
	}
	if len(attempts) == 0 {
		return r
	}
	holdout := func(a compiler.Attempt) *routine.Outcome {
		if t.Holdout == nil || a.Invalid != "" {
			return nil
		}
		// The holdout gets its own time, so a long compile does not fail it.
		hctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		o := routine.Check(hctx, a.Routine, "holdout", *t.Holdout)
		return &o
	}
	first := attempts[0]
	r.FirstAccepted = first.Accepted()
	if h := holdout(first); h != nil {
		r.FirstHoldout = h.Passed
	}
	last := attempts[len(attempts)-1]
	r.Accepted = last.Accepted()
	r.Holdout = holdout(last)
	return r
}

func write(dir string, results []result) {
	var kept []result
	for _, r := range results {
		if r.ID != "" {
			kept = append(kept, r)
		}
	}
	os.MkdirAll(dir, 0o755)
	b, _ := json.MarshalIndent(kept, "", " ")
	os.WriteFile(filepath.Join(dir, "results.json"), b, 0o644)

	var md strings.Builder
	first, gate, final, finalHold := 0, 0, 0, 0
	cost := 0.0
	md.WriteString("# Compiler proof\n\n| Task | First attempt accepted | First attempt passes holdout | Accepted after retry | Final passes holdout | Cost |\n|---|---|---|---|---|---|\n")
	for _, r := range kept {
		c := 0.0
		for _, a := range r.Attempts {
			c += a.CostUSD
		}
		cost += c
		fh := r.Holdout != nil && r.Holdout.Passed
		if r.FirstAccepted {
			first++
		}
		if r.FirstAccepted && r.FirstHoldout {
			gate++
		}
		if r.Accepted {
			final++
		}
		if r.Accepted && fh {
			finalHold++
		}
		fmt.Fprintf(&md, "| %s | %s | %s | %s | %s | $%.2f |\n", r.ID, mark(r.FirstAccepted), mark(r.FirstHoldout), mark(r.Accepted), mark(fh), c)
	}
	fmt.Fprintf(&md, "\n**Gate (first attempt accepted and passes the holdout): %d/%d.** First attempt accepted: %d. After one retry: %d accepted, %d also pass the holdout. Total cost $%.2f.\n", gate, len(kept), first, final, finalHold, cost)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte(md.String()), 0o644)
	fmt.Print("\n" + md.String())
}

// exportRoutines writes each accepted routine that also passed the
// holdout; its id is the trace's, without the numbering.
func exportRoutines(dir string, results []result) {
	n := 0
	for _, r := range results {
		if !r.Accepted || r.Holdout == nil || !r.Holdout.Passed {
			continue
		}
		id := r.ID
		if parts := strings.SplitN(id, "-", 3); len(parts) == 3 {
			id = parts[2]
		}
		if err := repo.Write(dir, id, r.Attempts[len(r.Attempts)-1].Routine); err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		n++
	}
	fmt.Printf("Wrote %d routines to %s\n", n, dir)
}

func mark(ok bool) string {
	if ok {
		return "yes"
	}
	return "no"
}

func errSuffix(e string) string {
	if e == "" {
		return ""
	}
	return " error: " + e
}
