package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/denerFernandes/pimpo/internal/event"
	"github.com/denerFernandes/pimpo/internal/owner"

	"github.com/denerFernandes/pimpo/internal/llm"
)

func TestExploreWithAnAPIModel(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := t.Context()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "sk-ant" {
			w.WriteHeader(401)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1000,"output_tokens":10}}`)
	}))
	defer api.Close()
	modelBase["anthropic"] = api.URL
	defer delete(modelBase, "anthropic")

	s := ta.Settings(ctx)
	s.ExploreModel = "anthropic:claude-sonnet-5"
	if code, out := ta.do(t, "PUT", "/api/settings", s); code != 400 {
		t.Fatalf("chose a model with no price: %d %v", code, out)
	}
	s.Models = []ModelOption{{ID: "anthropic:claude-sonnet-5", PriceIn: 3, PriceOut: 15}}
	if code, out := ta.do(t, "PUT", "/api/settings", s); code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	if code, out := ta.do(t, "POST", "/api/models/test", map[string]string{"id": "anthropic:claude-sonnet-5"}); code != 502 {
		t.Fatalf("worked without a key: %d %v", code, out)
	}
	if code, _ := ta.do(t, "PUT", "/api/models/keys/anthropic", map[string]string{"key": "sk-ant"}); code != 200 {
		t.Fatal(code)
	}
	_, keys := ta.do(t, "GET", "/api/models", nil)
	if keys["keys"].(map[string]any)["anthropic"] != true {
		t.Fatalf("%v", keys)
	}
	code, out := ta.do(t, "POST", "/api/models/test", map[string]string{"id": "anthropic:claude-sonnet-5"})
	if code != 200 || out["text"] != "ok" || out["cost_usd"] != (1000*3.0+10*15.0)/1e6 {
		t.Fatalf("%d %v", code, out)
	}
	resp, err := claude{ta.App}.Generate(ctx, llm.Request{Prompt: "x", Model: "anthropic:claude-sonnet-5"})
	if err != nil || resp.Text != "ok" {
		t.Fatalf("%v %v", resp, err)
	}
	if _, err := (claude{ta.App}).Generate(ctx, llm.Request{Prompt: "x", Model: "openai:gpt-x"}); err == nil {
		t.Fatal("ran a model with no price")
	}
}

// When the model of a job fails for a provider reason, the job moves to its
// fallback; the owner hears once, and again when the model is back.
func TestFallbackModels(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := t.Context()
	down := true
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down {
			w.WriteHeader(429)
			io.WriteString(w, `{"error":{"message":"rate limit"}}`)
			return
		}
		io.WriteString(w, `{"content":[{"type":"text","text":"from primary"}],"usage":{"input_tokens":10,"output_tokens":1}}`)
	}))
	defer primary.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"from backup"}}],"usage":{"prompt_tokens":10,"completion_tokens":1}}`)
	}))
	defer backup.Close()
	modelBase["anthropic"], modelBase["openai"] = primary.URL, backup.URL
	defer delete(modelBase, "anthropic")
	defer delete(modelBase, "openai")
	ta.do(t, "PUT", "/api/models/keys/anthropic", map[string]string{"key": "a"})
	ta.do(t, "PUT", "/api/models/keys/openai", map[string]string{"key": "o"})

	s := ta.Settings(ctx)
	s.Models = []ModelOption{{ID: "anthropic:claude-sonnet-5", PriceIn: 3, PriceOut: 15}}
	s.CompileModel = "anthropic:claude-sonnet-5"
	s.Fallbacks = map[string][]string{"compile": {"openai:gpt-5-mini"}}
	if code, _ := ta.do(t, "PUT", "/api/settings", s); code != 400 {
		t.Fatal("accepted a fallback with no price")
	}
	s.Models = append(s.Models, ModelOption{ID: "openai:gpt-5-mini", PriceIn: 0.25, PriceOut: 2})
	if code, out := ta.do(t, "PUT", "/api/settings", s); code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	c := claude{ta.App}
	for i := 0; i < 2; i++ {
		resp, err := c.Generate(ctx, llm.Request{Prompt: "x"})
		if err != nil || resp.Text != "from backup" {
			t.Fatalf("fallback: %v %v", resp, err)
		}
	}
	down = false
	if resp, err := c.Generate(ctx, llm.Request{Prompt: "x"}); err != nil || resp.Text != "from primary" {
		t.Fatalf("recovery: %v %v", resp, err)
	}
	// Spending lands on the model that answered, fallbacks included.
	_, cost := ta.do(t, "GET", "/api/cost", nil)
	byModel := cost["by_model"].(map[string]any)
	calls := cost["calls_by_model"].(map[string]any)
	if calls["openai:gpt-5-mini"] != float64(2) || calls["anthropic:claude-sonnet-5"] != float64(1) || byModel["openai:gpt-5-mini"].(float64) <= 0 || cost["by_job"].(map[string]any)["compile"] == nil {
		t.Fatalf("cost by model %v", cost)
	}
	evs, _ := ta.Events.List(ctx, event.Query{Types: []string{owner.EventNotice}})
	var texts []string
	for _, e := range evs {
		var n struct{ Text string }
		e.Decode(&n)
		texts = append(texts, n.Text)
	}
	if len(texts) != 2 || !strings.Contains(texts[0], "limite de uso") || !strings.Contains(texts[0], "openai:gpt-5-mini") || !strings.Contains(texts[1], "voltou") {
		t.Fatalf("notices %q", texts)
	}
}
