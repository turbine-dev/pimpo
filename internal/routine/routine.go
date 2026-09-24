// Package routine holds compiled routines and checks them: a routine is
// accepted only if it passes its own tests, replays the exploration it came
// from, and does not hard-code the data it was compiled from.
package routine

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/denerFernandes/vigia/internal/capability"
	"github.com/denerFernandes/vigia/internal/runtime"
	"github.com/denerFernandes/vigia/internal/trace"
)

type Routine struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Manifest    runtime.Manifest `json:"manifest"`
	Code        string           `json:"code"`
	Tests       []Test           `json:"tests"`
}

// Test is a named scenario written by the compiler.
type Test struct {
	Name string `json:"name"`
	trace.Scenario
}

// Outcome of running a routine against one scenario.
type Outcome struct {
	Scenario string   `json:"scenario"`
	Passed   bool     `json:"passed"`
	Problems []string `json:"problems,omitempty"`
	Writes   []Write  `json:"writes,omitempty"`
}

type Write struct {
	Capability string `json:"capability"`
	Args       any    `json:"args"`
}

// Check runs a routine against a scenario with canned responses and
// verifies the writes it makes.
func Check(ctx context.Context, r Routine, name string, s trace.Scenario) Outcome {
	now, err := time.Parse(time.RFC3339, s.Now)
	if err != nil {
		now = time.Date(2026, 9, 24, 7, 0, 0, 0, time.UTC)
	}
	h := newScenarioHost(s)
	h.now = now
	out := Outcome{Scenario: name}
	if _, err := runtime.Run(ctx, r.Code, r.Manifest, h, runtime.Options{Now: now, Timeout: 5 * time.Second}); err != nil {
		out.Problems = append(out.Problems, "run failed: "+err.Error())
	}
	out.Writes = h.writes
	for _, e := range s.Expect {
		out.Problems = append(out.Problems, verify(e, h.writes)...)
	}
	out.Passed = len(out.Problems) == 0
	return out
}

func verify(e trace.Expect, writes []Write) []string {
	var text []string
	n := 0
	for _, w := range writes {
		if w.Capability == e.Capability {
			n++
			text = append(text, strings.ToLower(flatten(w.Args)))
		}
	}
	joined := strings.Join(text, "\n")
	var problems []string
	if e.Count != nil && n != *e.Count {
		problems = append(problems, fmt.Sprintf("%s called %d times, want %d", e.Capability, n, *e.Count))
	}
	for _, want := range e.Contains {
		if !strings.Contains(joined, strings.ToLower(want)) {
			problems = append(problems, fmt.Sprintf("%s never mentions %q", e.Capability, want))
		}
	}
	for _, bad := range e.NotContains {
		if strings.Contains(joined, strings.ToLower(bad)) {
			problems = append(problems, fmt.Sprintf("%s mentions %q, which it should leave out", e.Capability, bad))
		}
	}
	return problems
}

// Flatten is flatten for other packages.
func Flatten(v any) string { return flatten(Decode(mustJSON(v))) }

// Decode parses JSON into plain values, or nil.
func Decode(raw []byte) any {
	var v any
	json.Unmarshal(raw, &v)
	return v
}

func mustJSON(v any) []byte {
	if b, ok := v.(json.RawMessage); ok {
		return b
	}
	b, _ := json.Marshal(v)
	return b
}

// flatten joins every string and number inside a value, so checks match
// what a person would read rather than JSON syntax.
func flatten(v any) string {
	var parts []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			parts = append(parts, x)
		case float64, int, int64, bool:
			parts = append(parts, fmt.Sprint(x))
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(x[k])
			}
		}
	}
	walk(v)
	return strings.Join(parts, "\n")
}

type scenarioHost struct {
	responses map[string][]json.RawMessage
	last      map[string]json.RawMessage
	judgments map[string]map[string]float64
	writes    []Write
	now       time.Time
}

func newScenarioHost(s trace.Scenario) *scenarioHost {
	h := &scenarioHost{responses: map[string][]json.RawMessage{}, last: map[string]json.RawMessage{}, judgments: s.Judgments}
	for _, r := range s.Responses {
		h.responses[r.Capability] = append(h.responses[r.Capability], r.Result)
	}
	return h
}

func (h *scenarioHost) Call(_ context.Context, name, _ string, args any) (any, error) {
	spec := capability.Catalog[name]
	if spec.Writes() {
		h.writes = append(h.writes, Write{Capability: name, Args: args})
		return map[string]any{"ok": true}, nil
	}
	var raw json.RawMessage
	if q := h.responses[name]; len(q) > 0 {
		raw, h.responses[name] = q[0], q[1:]
		h.last[name] = raw
	} else if l, ok := h.last[name]; ok {
		raw = l
	} else {
		return []any{}, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return filterResponse(name, normalize(args), v, h.now), nil
}

// normalize turns goja's exported values into plain JSON types.
func normalize(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	json.Unmarshal(b, &out)
	return out
}

// Judge answers from the scenario's labels: the first label key found in
// the item decides. Unlabeled items get a clear "no".
func (h *scenarioHost) Judge(_ context.Context, name, _ string, item any) (float64, error) {
	text := flatten(item)
	labels := h.judgments[name]
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		if strings.Contains(text, k) {
			return labels[k], nil
		}
	}
	return 0.05, nil
}

var quoted = regexp.MustCompile("\"([^\"\\\\]|\\\\.){10,}\"|'([^'\\\\]|\\\\.){10,}'|`[^`]{10,}`")

// Memorized reports string literals in the code that copy data from the
// recorded responses, a sign the routine will not work on new data. Values
// the user named in the request (an address, a threshold) or that the agent
// used as a parameter while exploring (a search query) are allowed.
func Memorized(r Routine, t trace.Trace) []string {
	intent := []string{t.Request}
	for _, c := range t.Calls {
		if !capability.Catalog[c.Capability].Writes() {
			intent = append(intent, string(c.Args))
		}
	}
	request := strings.ToLower(strings.Join(intent, "\n"))
	var data []string
	for _, c := range t.Calls {
		if len(c.Result) == 0 || capability.Catalog[c.Capability].Writes() {
			continue
		}
		collectStrings(c.Result, &data)
	}
	var found []string
	for _, lit := range quoted.FindAllString(r.Code, -1) {
		body := strings.ToLower(lit[1 : len(lit)-1])
		for _, d := range data {
			d = strings.ToLower(d)
			if len(d) < 10 || strings.Contains(request, d) || enumLike(d) {
				continue
			}
			// A literal that contains a whole recorded value copies that
			// item. A short phrase that occurs inside recorded text is a
			// keyword filter ("pagamento confirmado"), which generalizes.
			if strings.Contains(body, d) || (strings.Contains(d, body) && len(body) >= 32) {
				found = append(found, lit)
				break
			}
		}
	}
	return found
}

// enumLike reports API vocabulary such as "in_transit" or "CATEGORY_SOCIAL".
func enumLike(s string) bool {
	for _, r := range s {
		if !(r == '_' || r == '-' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return strings.ContainsAny(s, "_-")
}

func collectStrings(raw json.RawMessage, out *[]string) {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return
	}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			*out = append(*out, x)
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(v)
}
