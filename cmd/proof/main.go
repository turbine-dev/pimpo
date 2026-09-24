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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/denerFernandes/vigia/internal/compiler"
	"github.com/denerFernandes/vigia/internal/llm"
	"github.com/denerFernandes/vigia/internal/routine"
	"github.com/denerFernandes/vigia/internal/trace"
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
	only := flag.String("only", "", "run only traces whose id contains this")
	flag.Parse()

	paths, _ := filepath.Glob(filepath.Join(*dir, "*.json"))
	sort.Strings(paths)
	c := compiler.Compiler{Model: llm.ClaudeCLI{Model: *model}, Attempts: 2}
	results := make([]result, len(paths))
	var wg sync.WaitGroup
	sem := make(chan struct{}, *workers)
	for i, p := range paths {
		t, err := trace.Load(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if *only != "" && !strings.Contains(t.ID, *only) {
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
}

func run(c compiler.Compiler, t trace.Trace) result {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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
		o := routine.Check(ctx, a.Routine, "holdout", *t.Holdout)
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
