package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// API drives a model over a provider's HTTP API: Anthropic's Messages API,
// or the chat completions API that OpenAI, OpenRouter and Ollama share.
// As an agent it runs its own tool loop against Pimpo's MCP server, so the
// policy still checks every call on the other side.
type API struct {
	// Provider is anthropic, openai, openrouter or ollama.
	Provider string
	Key      string
	// Base overrides the provider's address (Ollama's, or tests).
	Base  string
	Model string
	// PriceIn and PriceOut are USD per million tokens; cost is counted
	// from them and the call stops at MaxCostUSD.
	PriceIn, PriceOut float64
	HTTP              *http.Client
}

// Providers are the API backends Pimpo knows.
var Providers = []string{"anthropic", "openai", "openrouter", "google", "deepseek", "groq", "mistral", "xai", "ollama", "lmstudio", "custom"}

// Bases are the providers' API addresses; all but Anthropic speak the chat
// completions API OpenAI defined. custom has none: the owner gives it.
var Bases = map[string]string{
	"anthropic":  "https://api.anthropic.com/v1",
	"openai":     "https://api.openai.com/v1",
	"openrouter": "https://openrouter.ai/api/v1",
	"google":     "https://generativelanguage.googleapis.com/v1beta/openai",
	"deepseek":   "https://api.deepseek.com/v1",
	"groq":       "https://api.groq.com/openai/v1",
	"mistral":    "https://api.mistral.ai/v1",
	"xai":        "https://api.x.ai/v1",
	"ollama":     "http://127.0.0.1:11434/v1",
	"lmstudio":   "http://127.0.0.1:1234/v1",
}

// ErrCostLimit stops a call that would go over its budget.
var ErrCostLimit = errors.New("stopped at the cost limit")

func (a API) base() string {
	if a.Base != "" {
		return strings.TrimRight(a.Base, "/")
	}
	if b, ok := Bases[a.Provider]; ok {
		return b
	}
	return Bases["openai"]
}

func (a API) cost(in, out int) float64 {
	return (float64(in)*a.PriceIn + float64(out)*a.PriceOut) / 1e6
}

// turnCost prices a turn; Anthropic bills writing the prompt cache at 1.25
// times the input price and reading it at a tenth.
func (a API) turnCost(t turn) float64 {
	return a.cost(t.In, t.Out) + (float64(t.CacheWrite)*1.25+float64(t.CacheRead)*0.1)*a.PriceIn/1e6
}

func (a API) post(ctx context.Context, path string, body, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", a.base()+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.Provider == "anthropic" {
		req.Header.Set("x-api-key", a.Key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else if a.Key != "" {
		req.Header.Set("Authorization", "Bearer "+a.Key)
	}
	client := a.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s is unreachable: %w", a.Provider, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error json.RawMessage `json:"error"`
		}
		json.Unmarshal(raw, &e)
		msg := strings.TrimSpace(string(e.Error))
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		if len(msg) > 300 {
			msg = msg[:300]
		}
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return fmt.Errorf("%s refused the API key", a.Provider)
		}
		return fmt.Errorf("%s answered %d: %s", a.Provider, resp.StatusCode, msg)
	}
	return json.Unmarshal(raw, out)
}

// tool is a function the model may call, in a provider-neutral form.
type tool struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

type toolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// turn is one model answer: text, tool calls, and tokens used.
type turn struct {
	Text    string
	Calls   []toolCall
	In, Out int
	// CacheWrite and CacheRead are input tokens written to and read from
	// the provider's prompt cache, apart from In.
	CacheWrite, CacheRead int
}

// chat keeps a conversation in the provider's own message format.
type chat struct {
	a        API
	system   string
	tools    []tool
	messages []map[string]any
	force    string // a tool the model must call, for structured output
}

func (c *chat) user(text string) {
	c.messages = append(c.messages, map[string]any{"role": "user", "content": text})
}

func (c *chat) send(ctx context.Context) (turn, error) {
	if c.a.Provider == "anthropic" {
		return c.sendAnthropic(ctx)
	}
	return c.sendOpenAI(ctx)
}

func (c *chat) sendAnthropic(ctx context.Context) (turn, error) {
	// The tools and system prompt repeat on every turn and every call, and
	// the conversation grows by a turn at a time, so both are cached: one
	// breakpoint after the system prompt (which covers the tools before
	// it) and one on the newest message.
	body := map[string]any{"model": c.a.Model, "max_tokens": 8192, "messages": cachedTail(c.messages)}
	if c.system != "" {
		body["system"] = []map[string]any{{"type": "text", "text": c.system, "cache_control": ephemeral}}
	}
	if len(c.tools) > 0 {
		var ts []map[string]any
		for _, t := range c.tools {
			ts = append(ts, map[string]any{"name": t.Name, "description": t.Description, "input_schema": t.Schema})
		}
		body["tools"] = ts
		if c.force != "" {
			body["tool_choice"] = map[string]string{"type": "tool", "name": c.force}
		}
	}
	var r struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		Usage struct {
			In         int `json:"input_tokens"`
			Out        int `json:"output_tokens"`
			CacheWrite int `json:"cache_creation_input_tokens"`
			CacheRead  int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}
	if err := c.a.post(ctx, "/messages", body, &r); err != nil {
		return turn{}, err
	}
	t := turn{In: r.Usage.In, Out: r.Usage.Out, CacheWrite: r.Usage.CacheWrite, CacheRead: r.Usage.CacheRead}
	var content []map[string]any
	for _, b := range r.Content {
		switch b.Type {
		case "text":
			t.Text += b.Text
			content = append(content, map[string]any{"type": "text", "text": b.Text})
		case "tool_use":
			t.Calls = append(t.Calls, toolCall{b.ID, b.Name, b.Input})
			content = append(content, map[string]any{"type": "tool_use", "id": b.ID, "name": b.Name, "input": b.Input})
		}
	}
	c.messages = append(c.messages, map[string]any{"role": "assistant", "content": content})
	return t, nil
}

func (c *chat) sendOpenAI(ctx context.Context) (turn, error) {
	msgs := c.messages
	if c.system != "" {
		msgs = append([]map[string]any{{"role": "system", "content": c.system}}, msgs...)
	}
	body := map[string]any{"model": c.a.Model, "messages": msgs}
	if len(c.tools) > 0 {
		var ts []map[string]any
		for _, t := range c.tools {
			ts = append(ts, map[string]any{"type": "function", "function": map[string]any{"name": t.Name, "description": t.Description, "parameters": t.Schema}})
		}
		body["tools"] = ts
		if c.force != "" {
			body["tool_choice"] = map[string]any{"type": "function", "function": map[string]string{"name": c.force}}
		}
	}
	var r struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			In  int `json:"prompt_tokens"`
			Out int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := c.a.post(ctx, "/chat/completions", body, &r); err != nil {
		return turn{}, err
	}
	if len(r.Choices) == 0 {
		return turn{}, fmt.Errorf("%s returned no answer", c.a.Provider)
	}
	m := r.Choices[0].Message
	t := turn{Text: m.Content, In: r.Usage.In, Out: r.Usage.Out}
	msg := map[string]any{"role": "assistant", "content": m.Content}
	if len(m.ToolCalls) > 0 {
		var calls []map[string]any
		for _, tc := range m.ToolCalls {
			args := json.RawMessage(tc.Function.Arguments)
			if !json.Valid(args) {
				args = json.RawMessage(`{}`)
			}
			t.Calls = append(t.Calls, toolCall{tc.ID, tc.Function.Name, args})
			calls = append(calls, map[string]any{"id": tc.ID, "type": "function", "function": map[string]string{"name": tc.Function.Name, "arguments": tc.Function.Arguments}})
		}
		msg["tool_calls"] = calls
	}
	c.messages = append(c.messages, msg)
	return t, nil
}

var ephemeral = map[string]string{"type": "ephemeral"}

// cachedTail copies the messages with a cache breakpoint on the last block
// of the newest one, leaving the kept conversation untouched so old
// breakpoints do not pile up past the provider's limit of four.
func cachedTail(msgs []map[string]any) []map[string]any {
	if len(msgs) == 0 {
		return msgs
	}
	out := append([]map[string]any(nil), msgs...)
	last := map[string]any{}
	for k, v := range out[len(out)-1] {
		last[k] = v
	}
	var blocks []map[string]any
	switch c := last["content"].(type) {
	case string:
		blocks = []map[string]any{{"type": "text", "text": c}}
	case []map[string]any:
		blocks = append([]map[string]any(nil), c...)
	default:
		return out
	}
	if len(blocks) == 0 {
		return out
	}
	tail := map[string]any{}
	for k, v := range blocks[len(blocks)-1] {
		tail[k] = v
	}
	tail["cache_control"] = ephemeral
	blocks[len(blocks)-1] = tail
	last["content"] = blocks
	out[len(out)-1] = last
	return out
}

// results answers the tool calls of the last turn.
func (c *chat) results(calls []toolCall, outputs []string, failed []bool) {
	if c.a.Provider == "anthropic" {
		var content []map[string]any
		for i, call := range calls {
			content = append(content, map[string]any{"type": "tool_result", "tool_use_id": call.ID, "content": outputs[i], "is_error": failed[i]})
		}
		c.messages = append(c.messages, map[string]any{"role": "user", "content": content})
		return
	}
	for i, call := range calls {
		c.messages = append(c.messages, map[string]any{"role": "tool", "tool_call_id": call.ID, "content": outputs[i]})
	}
}

// Generate answers one prompt; with a schema the answer is a forced tool
// call whose input is the structured output.
func (a API) Generate(ctx context.Context, r Request) (Response, error) {
	c := &chat{a: a, system: r.System}
	c.user(r.Prompt)
	if len(r.Schema) > 0 {
		c.tools = []tool{{Name: "answer", Description: "Give the answer in this exact shape.", Schema: r.Schema}}
		c.force = "answer"
	}
	t, err := c.send(ctx)
	if err != nil {
		return Response{}, err
	}
	resp := Response{Text: t.Text, CostUSD: a.turnCost(t)}
	if len(r.Schema) > 0 {
		if len(t.Calls) == 0 {
			return resp, fmt.Errorf("%s gave no structured answer", a.Provider)
		}
		resp.Structured = t.Calls[0].Input
	}
	if r.MaxCostUSD > 0 && resp.CostUSD > r.MaxCostUSD {
		return resp, ErrCostLimit
	}
	return resp, nil
}

// mcpClient calls Pimpo's MCP server for the agent.
type mcpClient struct {
	url  string
	http *http.Client
	id   int
}

func (m *mcpClient) call(ctx context.Context, method string, params any, out any) error {
	m.id++
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": m.id, "method": method, "params": params})
	req, err := http.NewRequestWithContext(ctx, "POST", m.url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var r struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return err
	}
	if r.Error != nil {
		return errors.New(r.Error.Message)
	}
	return json.Unmarshal(r.Result, out)
}

// Run is the agent loop: the model calls Pimpo's tools until it answers.
func (a API) Run(ctx context.Context, r AgentRequest) (Response, error) {
	m := &mcpClient{url: r.MCPURL, http: &http.Client{Timeout: 5 * time.Minute}}
	var list struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := m.call(ctx, "tools/list", map[string]any{}, &list); err != nil {
		return Response{}, fmt.Errorf("could not list Pimpo's tools: %w", err)
	}
	c := &chat{a: a, system: r.System}
	for _, t := range list.Tools {
		c.tools = append(c.tools, tool{t.Name, t.Description, t.InputSchema})
	}
	c.user(r.Prompt)
	turns := r.MaxTurns
	if turns <= 0 {
		turns = 40
	}
	var cost float64
	for range turns {
		t, err := c.send(ctx)
		if err != nil {
			return Response{CostUSD: cost}, err
		}
		cost += a.turnCost(t)
		if r.MaxCostUSD > 0 && cost > r.MaxCostUSD {
			return Response{Text: t.Text, CostUSD: cost}, ErrCostLimit
		}
		if len(t.Calls) == 0 {
			return Response{Text: strings.TrimSpace(t.Text), CostUSD: cost}, nil
		}
		outputs := make([]string, len(t.Calls))
		failed := make([]bool, len(t.Calls))
		for i, call := range t.Calls {
			var res struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
				IsError bool `json:"isError"`
			}
			if err := m.call(ctx, "tools/call", map[string]any{"name": call.Name, "arguments": call.Input}, &res); err != nil {
				outputs[i], failed[i] = err.Error(), true
				continue
			}
			var text strings.Builder
			for _, p := range res.Content {
				text.WriteString(p.Text)
			}
			outputs[i], failed[i] = text.String(), res.IsError
		}
		c.results(t.Calls, outputs, failed)
	}
	return Response{CostUSD: cost}, fmt.Errorf("the model did not finish in %d turns", turns)
}
