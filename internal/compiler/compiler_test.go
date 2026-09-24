package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/denerFernandes/vigia/internal/llm"
	"github.com/denerFernandes/vigia/internal/trace"
)

func one() *int { n := 1; return &n }

var recorded = trace.Trace{
	ID:      "alert",
	Request: "Tell me on Telegram when Ana emails me",
	Now:     "2026-09-24T09:00:00-03:00",
	Calls: []trace.Call{
		{Capability: "gmail.search", Args: json.RawMessage(`{"query":"from:ana@acme.com is:unread"}`), Result: json.RawMessage(`[{"id":"m-9","from":"ana@acme.com","subject":"Board meeting moved"}]`)},
		{Capability: "telegram.send", Args: json.RawMessage(`{"text":"Ana: Board meeting moved"}`), Result: json.RawMessage(`{"ok":true}`)},
	},
	Outcome: "An alert with the subject",
	Expect:  []trace.Expect{{Capability: "telegram.send", Count: one(), Contains: []string{"Board meeting moved"}}},
}

func routineJSON(code string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"name": "ana-alert", "description": "Alerts on Ana's emails",
		"manifest": map[string]any{"schedule": "*/15 * * * *", "capabilities": []string{"gmail.search", "telegram.send"}},
		"code":     code,
		"tests": []any{map[string]any{
			"name": "two emails", "now": "2026-10-01T10:00:00-03:00",
			"responses": []any{map[string]any{"capability": "gmail.search", "result": []any{map[string]any{"id": "x1", "from": "ana@acme.com", "subject": "Budget"}}}},
			"expect":    []any{map[string]any{"capability": "telegram.send", "count": 1, "contains": []string{"Budget"}}},
		}},
	})
	return b
}

const generic = `async function run() {
  const m = await gmail.search({query: "from:ana@acme.com is:unread", days: 1});
  for (const x of m) await telegram.send({text: "Ana: " + x.subject});
}`

func TestCompileAcceptsAGoodRoutine(t *testing.T) {
	fake := &llm.Fake{Responses: []llm.Response{{Structured: routineJSON(generic), CostUSD: 0.12}}}
	attempts, err := Compiler{Model: fake, Attempts: 2}.Compile(context.Background(), recorded)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || !attempts[0].Accepted() || attempts[0].CostUSD != 0.12 {
		t.Fatalf("attempts %+v", attempts)
	}
	if !strings.Contains(fake.Requests[0].Prompt, "gmail.search") || len(fake.Requests[0].Schema) == 0 {
		t.Fatal("prompt misses the recording or the schema")
	}
}

func TestCompileFeedsProblemsBack(t *testing.T) {
	cheat := `async function run() { await telegram.send({text: "Ana: Board meeting moved"}); }`
	fake := &llm.Fake{Responses: []llm.Response{{Structured: routineJSON(cheat)}, {Structured: routineJSON(generic)}}}
	attempts, err := Compiler{Model: fake, Attempts: 2}.Compile(context.Background(), recorded)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 2 || attempts[0].Accepted() || !attempts[1].Accepted() {
		t.Fatalf("attempts %+v", attempts)
	}
	second := fake.Requests[1].Prompt
	if !strings.Contains(second, "hard-codes recorded data") || !strings.Contains(second, "never mentions") {
		t.Fatalf("feedback missing from retry prompt:\n%s", second)
	}
}

func TestInvalidOutputIsRejected(t *testing.T) {
	bad, _ := json.Marshal(map[string]any{"name": "x", "manifest": map[string]any{"capabilities": []string{"bank.transfer"}}, "code": "function run(){}"})
	fake := &llm.Fake{Responses: []llm.Response{{Structured: bad}}}
	attempts, _ := Compiler{Model: fake}.Compile(context.Background(), recorded)
	if attempts[0].Accepted() || !strings.Contains(attempts[0].Invalid, "unknown capability") {
		t.Fatalf("attempt %+v", attempts[0])
	}
}

func TestJudgmentsMustComeFromTheExploration(t *testing.T) {
	var r map[string]any
	json.Unmarshal(routineJSON(generic), &r)
	r["manifest"].(map[string]any)["judgments"] = map[string]string{"invented": "Is it urgent?"}
	b, _ := json.Marshal(r)
	fake := &llm.Fake{Responses: []llm.Response{{Structured: b}}}
	attempts, _ := Compiler{Model: fake}.Compile(context.Background(), recorded)
	if attempts[0].Accepted() || !strings.Contains(attempts[0].Invalid, "never made while exploring") {
		t.Fatalf("attempt %+v", attempts[0].Invalid)
	}
}

type flaky struct {
	fails int
	ok    llm.Response
}

func (f *flaky) Generate(context.Context, llm.Request) (llm.Response, error) {
	if f.fails > 0 {
		f.fails--
		return llm.Response{}, errors.New("claude: error_max_structured_output_retries: ")
	}
	return f.ok, nil
}

func TestTransientFormatErrorsAreRetried(t *testing.T) {
	attempts, err := Compiler{Model: &flaky{fails: 2, ok: llm.Response{Structured: routineJSON(generic)}}}.Compile(context.Background(), recorded)
	if err != nil || len(attempts) != 1 || !attempts[0].Accepted() {
		t.Fatalf("%v %+v", err, attempts)
	}
}
