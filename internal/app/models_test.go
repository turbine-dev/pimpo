package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/denerFernandes/zodim/internal/llm"
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
