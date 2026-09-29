// Package compiler turns a recorded exploration into a routine: code that
// runs without a language model, the capabilities it needs, and tests.
package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/runtime"
	"github.com/turbine-dev/pimpo/internal/trace"
)

type Compiler struct {
	Model llm.Model
	// Attempts is how many times to try, feeding problems back after a
	// failure. The first attempt is what the proof measures.
	Attempts int
	// Installed lists the owner's routines a new one may build on, and
	// Helpers loads one to run it in the checks.
	Installed func(ctx context.Context) []Installed
	Helpers   func(ctx context.Context, id string) (runtime.Helper, error)
}

// Installed is a routine the compiler may reuse with routines.run.
type Installed struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Capabilities []string        `json:"capabilities"`
	Params       []runtime.Param `json:"params,omitempty"`
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
	var installed []Installed
	if c.Installed != nil {
		installed = c.Installed(ctx)
	}
	if c.Helpers != nil {
		ctx = routine.WithLibrary(ctx, c.Helpers)
	}
	for i := 0; i < attempts; i++ {
		resp, err := generate(ctx, c.Model, llm.Request{System: system, Prompt: prompt(t, previous, feedback) + installedList(installed), Schema: schema, MaxCostUSD: 2})
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
	// A watching routine is woken in the replay the way the scheduler
	// would: routine.Check asks the watched capability with the watch's
	// arguments.
	replay := t.Replay()
	a.Outcomes = append(a.Outcomes, routine.Check(ctx, r, "replay of the exploration", replay))
	for _, test := range r.Tests {
		a.Outcomes = append(a.Outcomes, routine.Check(ctx, r, "test "+test.Name, test.Scenario))
	}
	a.Memorized = routine.Memorized(r, t)
	return a
}

const system = `You compile a recorded task into a routine for Pimpo, a personal agent.
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
- The recording is one sample: new items will be worded differently. Search at least as broadly as the agent did (the same query terms, or more), never narrower, and filter in code or by judgment. When code extracts values from free text (tracking codes, amounts, due dates, order numbers), accept the forms real messages use, in the request's language and in English: a code after any label ("Rastreio:", "Código de rastreamento", "Objeto", "Tracking number") or with none; prefer money.find, money.parse and dates.parse over patterns of your own.
- Settings the owner may want to change later (a city, a threshold, an email address or sender they named, a list of days, a language, a choice among options) are NEVER written in the code: declare each one in manifest.params and read it as params.<name>. params is read-only.
- When the routine sends a message to the owner, use notify.send({text}) (not telegram.send) and declare a parameter {"name":"destinos","label":"Onde avisar","type":"destinations","default":[]}; the owner picks one or more bots or channels.
- Subjective decisions ("is this important?", "is this a newsletter?", "does this need a reply?") must use a judgment: declare it in manifest.judgments as {name: "yes/no question about one item"} and call await judge.<name>(item), which returns {p} (probability of yes). Treat p >= 0.5 as yes. Pass the whole item. You may only declare the judgments the agent made while exploring (listed in the prompt); everything else must be decided by plain code (dates, amounts, keywords, fields like replied or labels).
- Objective decisions (dates, amounts, senders, keywords the user named) are plain code.
- Text the agent COMPOSED from an item's content (a one-line summary, what an email asks for, a suggested reply) cannot be plain code: declare it in manifest.writes as {name: "instruction in the request's language"} and call await write.<name>(item), which returns {text}. A small model writes it each run, so use writes only for composed text, never for facts plain code can copy (sender, subject, date, amount), and call it only for the items that will be sent. Copying fields is always better than writing.
- Keep messages concise and readable. Send nothing when there is nothing worth sending, unless the user asked for a message every time.
- state keeps small plain data between runs (up to 64 KB): state.get(key) -> value or null, state.set(key, value), state.delete(key), state.keys(). Use it only when the request compares with earlier runs or must not repeat itself ("tell me if the price dropped since yesterday", "only when it changed", "a weekly total", "don't send the same item twice"). It is saved only when the run succeeds. Keep it small: latest values, short lists (slice to the last N), not whole API responses.
- ask.owner({question, options, key}) asks the person the routine works for, with 2 to 6 options as buttons, and returns at once. Their answer runs this same routine again with event.answer = {key, question, choice, index, asked}. So when the request asks the owner something and acts on the reply ("ask me every night if I worked out and keep count", "ask before archiving"), the code starts with: if (event.answer) { handle event.answer.choice, keep it in state if it matters, reply with notify.send if useful; return }, and otherwise does the scheduled work and asks. A test covers each: one with no event (it asks), one with event: {answer: {key, question, choice, index: 0}} (it handles the answer).
- routines.run(id, params) runs another installed routine (listed in the prompt) and returns what its run() returns; most existing routines return nothing and only send messages. Use it when the request builds on a routine the owner already has, and declare the id in manifest.uses. Everything the other routine touches must also be in your capabilities.

Rules for the manifest:
- schedule: a 5-field cron expression matching the request, or "" when the routine reacts to something new (see watch).
- watch: when the request is about reacting to something new ("when an email from X arrives", "whenever this feed has a new post", "if the front door opens", "me avise quando chegar…"), declare {capability, args, key, every} instead of a schedule. capability is the read capability you call to find the items (also listed in capabilities); args are its arguments, with {{param}} for values that come from params; key is the field that identifies one item (id, link, entity_id); every is how often to check ("10m"; at least "5m", "30m" or "1h" when minutes do not matter). Pimpo calls it without a model and runs the routine only with the items it has not seen, as event.items (each item shaped like that capability's results). Work on event.items and do not call the watched capability again. Tests of such a routine set event: {items: [...]} with new fictional items, and one test should have items that must not produce a message.
- webhook: when the request is about reacting when another service calls Pimpo ("when my iPhone Shortcut runs", "when Zapier/IFTTT/GitHub/a form sends…", "quando o webhook chegar"), set manifest.webhook to true, with schedule "" and no watch: the owner turns on the routine's webhook, and each call runs it with event.webhook = {method, query, body} (body is the parsed JSON, form fields or text). Read what you need from event.webhook.body, and do nothing when event.webhook is missing. Tests of such a routine set event: {webhook: {method: "POST", query: {}, body: {...}}}.
- locale: the language the user wrote the request in: "pt-BR", "en-US", "es-ES", "fr-FR", "de-DE", "it-IT", "ja-JP", "zh-CN", "ko-KR" or "ru-RU". dates.format uses it for weekday and month names, so write messages and test expectations in that language.
- capabilities: the minimum set the code calls. Scoped capabilities need the host, e.g. "http.getJSON:api.open-meteo.com". The host is fixed; values in the URL's query (latitude, longitude, currency) can come from params.
- uses: ids of installed routines this one runs with routines.run (omit when none).
- params: each {name (JavaScript identifier), label (short, in the request's language), type, default, options, help}. Types: text, number, boolean, date (YYYY-MM-DD), time (HH:MM), location (default {"name","latitude","longitude","timezone"}), select and multiselect (with options), email, destinations. Every param except destinations has a default taken from the request.

Rules for tests: write 2 or 3 scenarios with NEW fictional data (not the recording) covering the normal case and an edge case (nothing to report, several items, an item that must be excluded). At least one test words its items differently from the recording (other labels, senders, formats and order), as real new data would be. Each has now, responses (canned results for read calls, in the order the code makes them), optional writes (canned texts: write name -> identifying substring of the item -> text), optional params (values for this scenario; at least one test should change a param from its default), judgments (labels for the new items: judgment name -> {identifying substring of the item: probability}), optional state (what earlier runs kept) and expect_state (key -> value the run must keep), and expect (checks on write calls: capability, optional count, contains, not_contains). A routine that uses state needs a test with no earlier state and one with state that changes the outcome. The canned responses also feed the routines it uses, in call order. Expectations must follow from the data and the request. Dates in tests: write now and every timestamp in UTC (ending in Z), and keep items at least 3 hours inside or outside any time window, so an item's side of the line never depends on time-zone arithmetic.`

var schema = json.RawMessage(`{
 "type":"object","additionalProperties":false,
 "required":["name","description","manifest","code","tests"],
 "properties":{
  "name":{"type":"string"},
  "description":{"type":"string"},
  "manifest":{"type":"object","additionalProperties":false,"required":["schedule","capabilities"],"properties":{
    "schedule":{"type":"string"},
    "webhook":{"type":"boolean"},
    "watch":{"type":"object","additionalProperties":false,"required":["capability","key"],"properties":{
      "capability":{"type":"string"},"args":{"type":"object"},"key":{"type":"string"},"every":{"type":"string"}}},
    "capabilities":{"type":"array","items":{"type":"string"}},
    "judgments":{"type":"object","additionalProperties":{"type":"string"}},
    "writes":{"type":"object","additionalProperties":{"type":"string"}},
    "locale":{"type":"string","enum":["pt-BR","en-US","es-ES","fr-FR","de-DE","it-IT","ja-JP","zh-CN","ko-KR","ru-RU"]},
    "uses":{"type":"array","items":{"type":"string"}},
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
    "state":{"type":"object"},
    "expect_state":{"type":"object"},
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
	fmt.Fprintf(&b, "\nThe user asked: %q\nRecorded at: %s\n", t.Request, t.Now)
	if t.Event != nil {
		fmt.Fprintf(&b, "The recorded run was started with this event (the routine gets it as event): %s\n", compact(mustJSON(t.Event)))
	}
	b.WriteString("\nRecorded calls, in order:\n")
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

// installedList tells the model which routines it may build on.
func installedList(list []Installed) string {
	if len(list) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nInstalled routines you may run with routines.run(id, params) (declare them in manifest.uses):\n")
	for _, r := range list {
		fmt.Fprintf(&b, "- %s: %s — %s; touches %s", r.ID, r.Name, r.Description, strings.Join(r.Capabilities, ", "))
		if len(r.Params) > 0 {
			var ps []string
			for _, p := range r.Params {
				ps = append(ps, p.Name)
			}
			fmt.Fprintf(&b, "; params %s", strings.Join(ps, ", "))
		}
		b.WriteString("\n")
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
