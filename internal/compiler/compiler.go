// Package compiler turns a recorded exploration into a routine: code that
// runs without a language model, the capabilities it needs, and tests.
package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/denerFernandes/zodim/internal/capability"
	"github.com/denerFernandes/zodim/internal/llm"
	"github.com/denerFernandes/zodim/internal/routine"
	"github.com/denerFernandes/zodim/internal/runtime"
	"github.com/denerFernandes/zodim/internal/trace"
)

type Compiler struct {
	Model llm.Model
	// Attempts is how many times to try, feeding problems back after a
	// failure. The first attempt is what the proof measures.
	Attempts int
}

type Attempt struct {
	Routine   routine.Routine   `json:"routine"`
	Outcomes  []routine.Outcome `json:"outcomes"`
	Memorized []string          `json:"memorized,omitempty"`
	Invalid   string            `json:"invalid,omitempty"`
	CostUSD   float64           `json:"cost_usd"`
}

func (a Attempt) Accepted() bool {
	if a.Invalid != "" || len(a.Memorized) > 0 || len(a.Outcomes) == 0 {
		return false
	}
	for _, o := range a.Outcomes {
		if !o.Passed {
			return false
		}
	}
	return true
}

func (a Attempt) Problems() []string {
	var p []string
	if a.Invalid != "" {
		p = append(p, a.Invalid)
	}
	for _, m := range a.Memorized {
		p = append(p, "hard-codes recorded data "+m+"; derive it from capability results instead")
	}
	for _, o := range a.Outcomes {
		for _, pr := range o.Problems {
			p = append(p, o.Scenario+": "+pr)
		}
	}
	return p
}

// Compile returns every attempt; the last one is the result.
func (c Compiler) Compile(ctx context.Context, t trace.Trace) ([]Attempt, error) {
	attempts := max(c.Attempts, 1)
	var out []Attempt
	var feedback []string
	var previous []byte
	for i := 0; i < attempts; i++ {
		resp, err := generate(ctx, c.Model, llm.Request{System: system, Prompt: prompt(t, previous, feedback), Schema: schema, MaxCostUSD: 2})
		a := Attempt{CostUSD: resp.CostUSD}
		if err != nil {
			return out, fmt.Errorf("compile %s: %w", t.ID, err)
		}
		if err := json.Unmarshal(resp.Structured, &a.Routine); err != nil {
			a.Invalid = "output is not a routine: " + err.Error()
		} else {
			a = Verify(ctx, a.Routine, t)
			a.CostUSD = resp.CostUSD
		}
		out = append(out, a)
		if a.Accepted() {
			break
		}
		feedback = a.Problems()
		previous = resp.Structured
	}
	return out, nil
}

// generate retries failures that say nothing about the routine, such as the
// model giving up on the output format. They do not count as attempts.
func generate(ctx context.Context, m llm.Model, r llm.Request) (llm.Response, error) {
	var resp llm.Response
	var err error
	for i := 0; i < 3; i++ {
		resp, err = m.Generate(ctx, r)
		if err == nil || !transient(err) {
			return resp, err
		}
	}
	return resp, err
}

func transient(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "structured_output") || strings.Contains(msg, "no structured output") || strings.Contains(msg, "overloaded")
}

// Verify checks a routine against its own tests, the recorded exploration
// and the hard-coded data check.
func Verify(ctx context.Context, r routine.Routine, t trace.Trace) Attempt {
	a := Attempt{Routine: r}
	if err := r.Manifest.Validate(); err != nil {
		a.Invalid = "manifest: " + err.Error()
		return a
	}
	if err := r.Manifest.Starts(); err != nil {
		a.Invalid = "manifest: " + err.Error()
		return a
	}
	for name := range r.Manifest.Judgments {
		if _, ok := t.Judgments[name]; !ok {
			a.Invalid = fmt.Sprintf("judgment %q was never made while exploring, so it cannot be tested; use only %s", name, judgmentList(t))
			return a
		}
	}
	if strings.TrimSpace(r.Code) == "" {
		a.Invalid = "empty code"
		return a
	}
	replay := t.Replay()
	if w := r.Manifest.Watch; w != nil {
		replay.Event = WatchEvent(t, *w)
	}
	a.Outcomes = append(a.Outcomes, routine.Check(ctx, r, "replay of the exploration", replay))
	for _, test := range r.Tests {
		a.Outcomes = append(a.Outcomes, routine.Check(ctx, r, "test "+test.Name, test.Scenario))
	}
	a.Memorized = routine.Memorized(r, t)
	return a
}

const system = `You compile a recorded task into a routine for Zodim, a personal agent.
A routine is plain JavaScript that runs on a schedule WITHOUT a language model. It must do for new data what the agent did in the recording.

Rules for the code:
- Plain JavaScript (ES2020), not TypeScript: no type annotations. Define exactly: async function run() { ... }  (no modules, no import/export, no require, no fetch, no timers).
- Reach the world ONLY through the capability objects listed in the manifest, called exactly as documented. Nothing else exists.
- now() returns the current time as an ISO 8601 string in the owner's time zone; use it for every date. Never hard-code dates.
- log(text) writes a debug line.
- Intl, toLocaleString, toLocaleDateString and friends DO NOT EXIST. Use these helpers (all dates are ISO strings, in the owner's zone and language):
  dates.today() -> start of today; dates.startOfDay(iso, offsetDays) -> midnight of that day plus offset;
  dates.addDays(iso, n); dates.addHours(iso, n); dates.diffDays(a, b) -> whole calendar days from a to b;
  dates.sameDay(a, b); dates.isBefore(a, b); dates.weekday(iso) -> 0 Sunday..6 Saturday;
  dates.format(iso, pattern) with EEEE (weekday name), EEE, d, dd, MMMM (month name), MMM, MM, yyyy, HH, mm, and 'literal text' in single quotes;
  dates.parse(text) -> ISO date of the first date found in free text (29/09/2026, 30/09, "5 de outubro", "Oct 3, 2026") or null;
  money.find(text) -> the first amount as written ("R$ 1.482,35") or null; money.parse(text) -> number or null; money.format(number, "BRL"|"USD"|"EUR").
  Calendar all-day events have a plain date (2026-09-25) as start; timed events have a full ISO time.
- Never copy data from the recording into the code (names, subjects, ids, amounts). Derive everything from capability results at run time.
- Settings the owner may want to change later (a city, a threshold, an email address or sender they named, a list of days, a language, a choice among options) are NEVER written in the code: declare each one in manifest.params and read it as params.<name>. params is read-only.
- When the routine sends a message to the owner, use notify.send({text}) (not telegram.send) and declare a parameter {"name":"destinos","label":"Onde avisar","type":"destinations","default":[]}; the owner picks one or more bots or channels.
- Subjective decisions ("is this important?", "is this a newsletter?", "does this need a reply?") must use a judgment: declare it in manifest.judgments as {name: "yes/no question about one item"} and call await judge.<name>(item), which returns {p} (probability of yes). Treat p >= 0.5 as yes. Pass the whole item. You may only declare the judgments the agent made while exploring (listed in the prompt); everything else must be decided by plain code (dates, amounts, keywords, fields like replied or labels).
- Objective decisions (dates, amounts, senders, keywords the user named) are plain code.
- Text the agent COMPOSED from an item's content (a one-line summary, what an email asks for, a suggested reply) cannot be plain code: declare it in manifest.writes as {name: "instruction in the request's language"} and call await write.<name>(item), which returns {text}. A small model writes it each run, so use writes only for composed text, never for facts plain code can copy (sender, subject, date, amount), and call it only for the items that will be sent. Copying fields is always better than writing.
- Keep messages concise and readable. Send nothing when there is nothing worth sending, unless the user asked for a message every time.

Rules for the manifest:
- schedule: a 5-field cron expression matching the request, or "" when the routine reacts to something new (see watch).
- watch: when the request is about reacting to something new ("when an email from X arrives", "whenever this feed has a new post", "if the front door opens", "me avise quando chegar…"), declare {capability, args, key, every} instead of a schedule. capability is the read capability you call to find the items (also listed in capabilities); args are its arguments, with {{param}} for values that come from params; key is the field that identifies one item (id, link, entity_id); every is how often to check ("10m"; at least "5m", "30m" or "1h" when minutes do not matter). Zodim calls it without a model and runs the routine only with the items it has not seen, as event.items (each item shaped like that capability's results). Work on event.items and do not call the watched capability again. Tests of such a routine set event: {items: [...]} with new fictional items, and one test should have items that must not produce a message.
- locale: the language the user wrote the request in, "pt-BR" or "en-US". dates.format uses it for weekday and month names, so write messages and test expectations in that language.
- capabilities: the minimum set the code calls. Scoped capabilities need the host, e.g. "http.getJSON:api.open-meteo.com". The host is fixed; values in the URL's query (latitude, longitude, currency) can come from params.
- params: each {name (JavaScript identifier), label (short, in the request's language), type, default, options, help}. Types: text, number, boolean, date (YYYY-MM-DD), time (HH:MM), location (default {"name","latitude","longitude","timezone"}), select and multiselect (with options), email, destinations. Every param except destinations has a default taken from the request.

Rules for tests: write 2 or 3 scenarios with NEW fictional data (not the recording) covering the normal case and an edge case (nothing to report, several items, an item that must be excluded). Each has now, responses (canned results for read calls, in the order the code makes them), optional writes (canned texts: write name -> identifying substring of the item -> text), optional params (values for this scenario; at least one test should change a param from its default), judgments (labels for the new items: judgment name -> {identifying substring of the item: probability}) and expect (checks on write calls: capability, optional count, contains, not_contains). Expectations must follow from the data and the request. Dates in tests: write now and every timestamp in UTC (ending in Z), and keep items at least 3 hours inside or outside any time window, so an item's side of the line never depends on time-zone arithmetic.`

var schema = json.RawMessage(`{
 "type":"object","additionalProperties":false,
 "required":["name","description","manifest","code","tests"],
 "properties":{
  "name":{"type":"string"},
  "description":{"type":"string"},
  "manifest":{"type":"object","additionalProperties":false,"required":["schedule","capabilities"],"properties":{
    "schedule":{"type":"string"},
    "watch":{"type":"object","additionalProperties":false,"required":["capability","key"],"properties":{
      "capability":{"type":"string"},"args":{"type":"object"},"key":{"type":"string"},"every":{"type":"string"}}},
    "capabilities":{"type":"array","items":{"type":"string"}},
    "judgments":{"type":"object","additionalProperties":{"type":"string"}},
    "writes":{"type":"object","additionalProperties":{"type":"string"}},
    "locale":{"type":"string","enum":["pt-BR","en-US"]},
    "params":{"type":"array","items":{"type":"object","required":["name","label","type"],"properties":{
      "name":{"type":"string"},"label":{"type":"string"},
      "type":{"type":"string","enum":["text","number","boolean","date","time","location","select","multiselect","email","destinations"]},
      "default":{},"options":{"type":"array","items":{"type":"string"}},"help":{"type":"string"}}}}}},
  "code":{"type":"string"},
  "tests":{"type":"array","items":{"type":"object","required":["name","now","responses","expect"],"properties":{
    "name":{"type":"string"},
    "now":{"type":"string"},
    "params":{"type":"object"},
    "event":{"type":"object","properties":{"items":{"type":"array"}}},
    "responses":{"type":"array","items":{"type":"object","required":["capability","result"],"properties":{"capability":{"type":"string"},"result":{}}}},
    "judgments":{"type":"object","additionalProperties":{"type":"object","additionalProperties":{"type":"number"}}},
    "writes":{"type":"object","additionalProperties":{"type":"object","additionalProperties":{"type":"string"}}},
    "expect":{"type":"array","items":{"type":"object","required":["capability"],"properties":{
      "capability":{"type":"string"},"count":{"type":"integer"},
      "contains":{"type":"array","items":{"type":"string"}},
      "not_contains":{"type":"array","items":{"type":"string"}}}}}}}}
 }}`)

// WatchEvent is what a watching routine would have been woken with during
// the exploration: every item the agent read from the watched capability.
func WatchEvent(t trace.Trace, w runtime.Watch) map[string]any {
	name := strings.SplitN(w.Capability, ":", 2)[0]
	items := []any{}
	for _, c := range t.Calls {
		if c.Capability != name {
			continue
		}
		var v any
		json.Unmarshal(c.Result, &v)
		if list, ok := v.([]any); ok {
			items = append(items, list...)
		}
	}
	return map[string]any{"items": items}
}

func prompt(t trace.Trace, previous []byte, feedback []string) string {
	var b strings.Builder
	b.WriteString("Capabilities available (call signature -> result):\n")
	for _, n := range capability.Names() {
		s := capability.Catalog[n]
		scope := ""
		if s.Scoped {
			scope = fmt.Sprintf(" (declare with the exact host of the URL, www. included when the URL has it, e.g. %s:www.example.com)", s.Name)
		}
		fmt.Fprintf(&b, "- %s [%s]%s -> %s\n", s.Signature, s.Risk, scope, s.Returns)
	}
	fmt.Fprintf(&b, "\nThe user asked: %q\nRecorded at: %s\n\nRecorded calls, in order:\n", t.Request, t.Now)
	for i, c := range t.Calls {
		fmt.Fprintf(&b, "%d. %s(%s)", i+1, c.Capability, compact(c.Args))
		if len(c.Result) > 0 {
			fmt.Fprintf(&b, "\n   -> %s", compact(c.Result))
		}
		if c.Error != "" {
			fmt.Fprintf(&b, "\n   -> failed: %s", c.Error)
		}
		b.WriteString("\n")
	}
	if len(t.Judgments) > 0 {
		b.WriteString("\nDecisions the agent made while exploring (judgment -> item -> probability of yes):\n")
		names := make([]string, 0, len(t.Judgments))
		for n := range t.Judgments {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			q := ""
			if t.Questions[n] != "" {
				q = fmt.Sprintf(" (asked: %q)", t.Questions[n])
			}
			fmt.Fprintf(&b, "- %s%s: %s\n", n, q, compact(mustJSON(t.Judgments[n])))
		}
	}
	fmt.Fprintf(&b, "\nJudgments you may declare (exactly these names, only if the code needs them): %s\n", judgmentList(t))
	fmt.Fprintf(&b, "\nThe user approved this outcome: %s\n", t.Outcome)
	if len(feedback) > 0 {
		if len(previous) > 0 {
			fmt.Fprintf(&b, "\nYour previous routine:\n%s\n", previous)
		}
		b.WriteString("\nIt was rejected for these problems:\n")
		for _, f := range feedback {
			b.WriteString("- " + f + "\n")
		}
		b.WriteString("\nFor each failing test, first work out from its data what the routine should send. If the code is right and the test's expectation is wrong (for example, an item that is really outside the time window), fix the test; otherwise fix the code. Change only what is wrong.\n")
	}
	return b.String()
}

func judgmentList(t trace.Trace) string {
	if len(t.Judgments) == 0 {
		return "plain code (no judgments)"
	}
	names := make([]string, 0, len(t.Judgments))
	for n := range t.Judgments {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func compact(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
