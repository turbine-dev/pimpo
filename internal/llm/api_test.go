package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// fakeMCP serves one tool that returns the weather.
func fakeMCP(t *testing.T, calls *[]string) *httptest.Server {
	var mu sync.Mutex
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		var result any
		switch req.Method {
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{"name": "weather_today", "description": "the weather", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"city": map[string]string{"type": "string"}}}}}}
		case "tools/call":
			mu.Lock()
			*calls = append(*calls, string(req.Params))
			mu.Unlock()
			result = map[string]any{"content": []map[string]string{{"type": "text", "text": `{"temp":24}`}}}
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
}

func TestAnthropicAgentLoop(t *testing.T) {
	var calls []string
	mcp := fakeMCP(t, &calls)
	defer mcp.Close()
	step := 0
	var second map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "ak" || r.Header.Get("anthropic-version") == "" || r.URL.Path != "/messages" {
			w.WriteHeader(401)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		step++
		if step == 1 {
			sys, _ := body["system"].([]any)
			if body["model"] == "claude-sonnet-5" && (len(sys) != 1 || sys[0].(map[string]any)["text"] != "Be brief." || sys[0].(map[string]any)["cache_control"] == nil || len(body["tools"].([]any)) != 1) {
				t.Errorf("first request %v", body)
			}
			io.WriteString(w, `{"content":[{"type":"text","text":"Vou ver."},{"type":"tool_use","id":"tu1","name":"weather_today","input":{"city":"Lisboa"}}],"usage":{"input_tokens":1000,"output_tokens":100}}`)
			return
		}
		second = body
		io.WriteString(w, `{"content":[{"type":"text","text":"Faz 24 graus em Lisboa."}],"usage":{"input_tokens":200,"output_tokens":50,"cache_creation_input_tokens":100,"cache_read_input_tokens":1000}}`)
	}))
	defer api.Close()
	a := API{Provider: "anthropic", Key: "ak", Base: api.URL, Model: "claude-sonnet-5", PriceIn: 3, PriceOut: 15}
	resp, err := a.Run(context.Background(), AgentRequest{System: "Be brief.", Prompt: "Tempo em Lisboa?", MCPURL: mcp.URL})
	if err != nil || resp.Text != "Faz 24 graus em Lisboa." {
		t.Fatalf("%+v %v", resp, err)
	}
	if want := (1200*3.0+150*15.0)/1e6 + (100*1.25+1000*0.1)*3.0/1e6; math.Abs(resp.CostUSD-want) > 1e-12 {
		t.Fatalf("cost %v, want %v", resp.CostUSD, want)
	}
	if len(calls) != 1 || !strings.Contains(calls[0], `"city":"Lisboa"`) {
		t.Fatalf("%v", calls)
	}
	msgs := second["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)["content"].([]any)[0].(map[string]any)
	if last["type"] != "tool_result" || last["tool_use_id"] != "tu1" || last["content"] != `{"temp":24}` || last["cache_control"] == nil {
		t.Fatalf("tool result %v", last)
	}
	// Only the newest message carries a breakpoint; the first prompt,
	// cached on the previous turn, is sent as it was kept.
	if first := msgs[0].(map[string]any); first["content"] != "Tempo em Lisboa?" {
		t.Fatalf("older message changed: %v", first)
	}

	step = 0
	_, err = API{Provider: "anthropic", Key: "ak", Base: api.URL, Model: "x", PriceIn: 3, PriceOut: 15}.Run(context.Background(), AgentRequest{Prompt: "?", MCPURL: mcp.URL, MaxCostUSD: 0.001})
	if !errors.Is(err, ErrCostLimit) {
		t.Fatalf("cost limit: %v", err)
	}
	if _, err := (API{Provider: "anthropic", Key: "bad", Base: api.URL}).Generate(context.Background(), Request{Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "refused the API key") {
		t.Fatalf("%v", err)
	}
}

func TestOpenAICompatibleLoopAndStructuredOutput(t *testing.T) {
	var calls []string
	mcp := fakeMCP(t, &calls)
	defer mcp.Close()
	step := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if r.Header.Get("Authorization") != "Bearer ok" {
			w.WriteHeader(401)
			return
		}
		if tc, ok := body["tool_choice"].(map[string]any); ok {
			if tc["function"].(map[string]any)["name"] != "answer" {
				t.Errorf("forced tool %v", tc)
			}
			io.WriteString(w, `{"choices":[{"message":{"content":"","tool_calls":[{"id":"c9","function":{"name":"answer","arguments":"{\"name\":\"Bom dia\"}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
			return
		}
		step++
		if step == 1 {
			msgs := body["messages"].([]any)
			if msgs[0].(map[string]any)["role"] != "system" {
				t.Errorf("no system message %v", msgs)
			}
			io.WriteString(w, `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"c1","function":{"name":"weather_today","arguments":"{\"city\":\"Porto\"}"}}]}}],"usage":{"prompt_tokens":100,"completion_tokens":10}}`)
			return
		}
		msgs := body["messages"].([]any)
		last := msgs[len(msgs)-1].(map[string]any)
		if last["role"] != "tool" || last["tool_call_id"] != "c1" {
			t.Errorf("tool message %v", last)
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"24 graus no Porto."}}],"usage":{"prompt_tokens":120,"completion_tokens":8}}`)
	}))
	defer api.Close()
	a := API{Provider: "ollama", Key: "ok", Base: api.URL, Model: "qwen3:8b"}
	resp, err := a.Run(context.Background(), AgentRequest{System: "s", Prompt: "Tempo no Porto?", MCPURL: mcp.URL})
	if err != nil || resp.Text != "24 graus no Porto." || resp.CostUSD != 0 || !strings.Contains(calls[0], "Porto") {
		t.Fatalf("%+v %v %v", resp, err, calls)
	}
	out, err := a.Generate(context.Background(), Request{Prompt: "nome?", Schema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)})
	if err != nil || string(out.Structured) != `{"name":"Bom dia"}` {
		t.Fatalf("%s %v", out.Structured, err)
	}
}

// Effort reaches each provider in its own field; thinking comes back to
// Anthropic unchanged after a tool call; a model that refuses the level
// answers without it.
func TestEffort(t *testing.T) {
	var calls []string
	mcp := fakeMCP(t, &calls)
	defer mcp.Close()
	var bodies []map[string]any
	var mu sync.Mutex
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		bodies = append(bodies, body)
		n := len(bodies)
		mu.Unlock()
		if r.URL.Path == "/messages" {
			if n == 1 {
				io.WriteString(w, `{"content":[{"type":"thinking","thinking":"hm","signature":"sig1"},{"type":"tool_use","id":"tu1","name":"weather_today","input":{}}],"usage":{"input_tokens":1,"output_tokens":1}}`)
				return
			}
			io.WriteString(w, `{"content":[{"type":"text","text":"24"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
			return
		}
		if _, ok := body["reasoning_effort"]; ok && body["model"] == "old" {
			w.WriteHeader(400)
			io.WriteString(w, `{"error":{"message":"Unrecognized request argument supplied: reasoning_effort"}}`)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{}}`)
	}))
	defer api.Close()
	ctx := context.Background()

	if _, err := (API{Provider: "anthropic", Key: "k", Base: api.URL, Model: "claude-sonnet-5"}).Run(ctx, AgentRequest{Prompt: "tempo?", MCPURL: mcp.URL, Effort: "max"}); err != nil {
		t.Fatal(err)
	}
	if oc, _ := bodies[0]["output_config"].(map[string]any); oc["effort"] != "max" || bodies[0]["max_tokens"] != float64(32000) {
		t.Errorf("anthropic effort: %v %v", bodies[0]["output_config"], bodies[0]["max_tokens"])
	}
	msgs := bodies[1]["messages"].([]any)
	first := msgs[1].(map[string]any)["content"].([]any)[0].(map[string]any)
	if first["type"] != "thinking" || first["signature"] != "sig1" {
		t.Errorf("thinking not sent back: %v", first)
	}

	bodies = nil
	(API{Provider: "openrouter", Base: api.URL, Model: "x"}).Generate(ctx, Request{Prompt: "a", Effort: "high"})
	(API{Provider: "openai", Base: api.URL, Model: "x"}).Generate(ctx, Request{Prompt: "a", Effort: "max"})
	(API{Provider: "deepseek", Base: api.URL, Model: "x"}).Generate(ctx, Request{Prompt: "a", Effort: "low"})
	if r, _ := bodies[0]["reasoning"].(map[string]any); r["effort"] != "high" {
		t.Errorf("openrouter: %v", bodies[0])
	}
	if bodies[1]["reasoning_effort"] != "high" {
		t.Errorf("openai max should be high: %v", bodies[1])
	}
	if _, ok := bodies[2]["reasoning_effort"]; ok {
		t.Errorf("deepseek got an effort: %v", bodies[2])
	}

	bodies = nil
	resp, err := (API{Provider: "openai", Base: api.URL, Model: "old"}).Generate(ctx, Request{Prompt: "a", Effort: "low"})
	if err != nil || resp.Text != "ok" || len(bodies) != 2 {
		t.Fatalf("refused effort should retry without it: %v %v %d", resp, err, len(bodies))
	}
	if _, ok := bodies[1]["reasoning_effort"]; ok {
		t.Error("retry still sent the effort")
	}
}

// Claude Code gets the level as --effort.
func TestClaudeCLIEffort(t *testing.T) {
	dir := t.TempDir()
	bin := dir + "/claude"
	os.WriteFile(bin, []byte("#!/bin/sh\necho \"$@\" > "+dir+"/args\necho '{\"result\":\"ok\"}'\n"), 0o755)
	if _, err := (ClaudeCLI{Binary: bin}).Generate(context.Background(), Request{Prompt: "a", Effort: "max"}); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(dir + "/args")
	if !strings.Contains(string(args), "--effort max") {
		t.Fatalf("args %s", args)
	}
}
