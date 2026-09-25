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

	"github.com/denerFernandes/zodim/internal/capability"
	"github.com/denerFernandes/zodim/internal/runtime"
	"github.com/denerFernandes/zodim/internal/trace"
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
	if _, err := runtime.Run(ctx, r.Code, r.Manifest, h, runtime.Options{Now: now, Timeout: 5 * time.Second, Params: s.Params, Event: s.Event}); err != nil {
		out.Problems = append(out.Problems, "run failed: "+err.Error())
	}
	out.Writes = h.writes
	for _, e := range s.Expect {
		out.Problems = append(out.Problems, verify(e, h.writes)...)
	}
	out.Passed = len(out.Problems) == 0
	return out
}

// toOwner are the ways of telling the owner something; an expectation on
// one is met by any of them, since the explorer may use telegram.send
// where the routine uses notify.send.
var toOwner = map[string]bool{"telegram.send": true, "whatsapp.send": true, "notify.send": true}

func sameKind(a, b string) bool { return a == b || (toOwner[a] && toOwner[b]) }

func verify(e trace.Expect, writes []Write) []string {
	var text []string
	n := 0
	for _, w := range writes {
		if sameKind(w.Capability, e.Capability) {
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
	// world is, for searchable lists (mail, events), everything any
	// recorded call returned: the routine may ask differently from the
	// explorer, and gets what a real server would answer from that data.
	world     map[string][]any
	responses map[string][]json.RawMessage
	last      map[string]json.RawMessage
	judgments map[string]map[string]float64
	writes    []Write
	now       time.Time
}

// searchable capabilities answer queries over a set of items, so replay
// can answer any query from the items the exploration saw.
var searchable = map[string]bool{"gmail.search": true, "calendar.events": true}

func newScenarioHost(s trace.Scenario) *scenarioHost {
	h := &scenarioHost{world: map[string][]any{}, responses: map[string][]json.RawMessage{}, last: map[string]json.RawMessage{}, judgments: s.Judgments}
	seen := map[string]bool{}
	for _, r := range s.Responses {
		h.responses[r.Capability] = append(h.responses[r.Capability], r.Result)
		if !searchable[r.Capability] {
			continue
		}
		var list []any
		json.Unmarshal(r.Result, &list)
		if _, ok := h.world[r.Capability]; !ok {
			h.world[r.Capability] = []any{}
		}
		for _, it := range list {
			key := r.Capability + "|" + flatten(it)
			if m, ok := it.(map[string]any); ok && str(m["id"]) != "" {
				key = r.Capability + "|" + str(m["id"])
			}
			if !seen[key] {
				seen[key] = true
				h.world[r.Capability] = append(h.world[r.Capability], it)
			}
		}
	}
	return h
}

func (h *scenarioHost) Call(_ context.Context, name, _ string, args any) (any, error) {
	spec := capability.Catalog[name]
	if spec.Writes() {
		h.writes = append(h.writes, Write{Capability: name, Args: args})
		return map[string]any{"ok": true}, nil
	}
	if list, ok := h.world[name]; ok {
		return filterResponse(name, normalize(args), append([]any{}, list...), h.now), nil
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

// Judge answers from the scenario's labels. A label key identifies an item
// by its id or a piece of its text, sometimes both ("INBOX/1 - Contrato");
// the key whose parts match the most text wins. Unlabeled items get a clear
// "no".
func (h *scenarioHost) Judge(_ context.Context, name, _ string, item any) (float64, error) {
	text := flatten(item)
	best, bestScore := 0.05, 0
	for key, p := range h.judgments[name] {
		if score := matchScore(key, text); score > bestScore {
			best, bestScore = p, score
		}
	}
	return best, nil
}

var keyParts = regexp.MustCompile(`\s+[-–—|:]\s+`)

func matchScore(key, text string) int {
	if strings.Contains(text, key) {
		return len(key) * 2
	}
	score := 0
	for _, part := range keyParts.Split(key, -1) {
		part = strings.TrimSpace(part)
		if len(part) >= 3 && strings.Contains(text, part) {
			score += len(part)
		}
	}
	return score
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

// auditHost records every capability a routine calls.
type auditHost struct {
	*scenarioHost
	used map[string]bool
}

func (h *auditHost) Call(ctx context.Context, name, scope string, args any) (any, error) {
	h.used[name] = true
	return h.scenarioHost.Call(ctx, name, scope, args)
}

// Audit runs the routine's own tests with every capability reachable and
// reports what it really calls. A routine that calls anything its manifest
// does not declare, or reaches a host outside its scope, is lying about
// what it touches.
func Audit(ctx context.Context, r Routine) (used []string, problems []string) {
	declared := map[string]bool{}
	for _, entry := range r.Manifest.Capabilities {
		if spec, _, err := capability.Parse(entry); err == nil {
			declared[spec.Name] = true
		} else {
			problems = append(problems, err.Error())
		}
	}
	wide := r.Manifest
	wide.Capabilities = append([]string{}, r.Manifest.Capabilities...)
	for _, name := range capability.Names() {
		if declared[name] {
			continue
		}
		if capability.Catalog[name].Scoped {
			wide.Capabilities = append(wide.Capabilities, name+":undeclared.invalid")
		} else {
			wide.Capabilities = append(wide.Capabilities, name)
		}
	}
	seen := map[string]bool{}
	if len(r.Tests) == 0 {
		problems = append(problems, "the routine has no tests to audit")
	}
	for _, t := range r.Tests {
		now, err := time.Parse(time.RFC3339, t.Now)
		if err != nil {
			now = time.Date(2026, 9, 24, 7, 0, 0, 0, time.UTC)
		}
		h := &auditHost{scenarioHost: newScenarioHost(t.Scenario), used: seen}
		h.now = now
		if _, err := runtime.Run(ctx, r.Code, wide, h, runtime.Options{Now: now, Timeout: 5 * time.Second, Params: t.Params}); err != nil && strings.Contains(err.Error(), "outside the manifest scope") {
			problems = append(problems, t.Name+": "+err.Error())
		}
	}
	for name := range seen {
		used = append(used, name)
		if !declared[name] {
			problems = append(problems, "calls "+name+" without declaring it")
		}
	}
	sort.Strings(used)
	sort.Strings(problems)
	return used, problems
}
