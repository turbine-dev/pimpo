// Package llm talks to language models. The first backend drives Claude
// Code headless with the user's own subscription; API backends implement
// the same interface.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type Request struct {
	System string
	Prompt string
	// Schema, when set, asks for structured output matching this JSON schema.
	Schema json.RawMessage
	Model  string
	// MaxCostUSD stops the call if it would cost more.
	MaxCostUSD float64
}

type Response struct {
	Text       string
	Structured json.RawMessage
	CostUSD    float64
}

type Model interface {
	Generate(ctx context.Context, r Request) (Response, error)
}

// ClaudeCLI runs `claude -p` with every built-in tool disabled, so the model
// can only answer.
type ClaudeCLI struct {
	Binary string
	Model  string
}

func (c ClaudeCLI) Generate(ctx context.Context, r Request) (Response, error) {
	bin := c.Binary
	if bin == "" {
		bin = "claude"
	}
	args := []string{"-p", r.Prompt, "--output-format", "json", "--tools", "", "--no-session-persistence", "--strict-mcp-config"}
	if m := firstNonEmpty(r.Model, c.Model); m != "" {
		args = append(args, "--model", m)
	}
	if r.System != "" {
		args = append(args, "--system-prompt", r.System)
	}
	if len(r.Schema) > 0 {
		args = append(args, "--json-schema", string(r.Schema))
	}
	if r.MaxCostUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", r.MaxCostUSD))
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = "/"
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	var out struct {
		Result     string          `json:"result"`
		Structured json.RawMessage `json:"structured_output"`
		Cost       float64         `json:"total_cost_usd"`
		IsError    bool            `json:"is_error"`
		Subtype    string          `json:"subtype"`
	}
	if err := json.Unmarshal(lastLine(stdout.Bytes()), &out); err != nil {
		if runErr != nil {
			return Response{}, fmt.Errorf("claude: %w: %s", runErr, strings.TrimSpace(stderr.String()))
		}
		return Response{}, fmt.Errorf("claude: unreadable output: %w", err)
	}
	resp := Response{Text: out.Result, Structured: out.Structured, CostUSD: out.Cost}
	if out.IsError {
		return resp, fmt.Errorf("claude: %s: %s", out.Subtype, out.Result)
	}
	if len(r.Schema) > 0 && len(out.Structured) == 0 {
		return resp, errors.New("claude: no structured output")
	}
	return resp, nil
}

func lastLine(b []byte) []byte {
	b = bytes.TrimSpace(b)
	if i := bytes.LastIndexByte(b, '\n'); i >= 0 {
		return b[i+1:]
	}
	return b
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// Fake replays canned responses in order, for tests.
type Fake struct {
	mu        sync.Mutex
	Responses []Response
	Requests  []Request
}

func (f *Fake) Generate(_ context.Context, r Request) (Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Requests = append(f.Requests, r)
	if len(f.Responses) == 0 {
		return Response{}, errors.New("fake llm: no responses left")
	}
	resp := f.Responses[0]
	f.Responses = f.Responses[1:]
	return resp, nil
}

// AgentRequest runs a model that may call tools served over MCP.
type AgentRequest struct {
	System     string
	Prompt     string
	MCPURL     string
	Model      string
	MaxCostUSD float64
	MaxTurns   int
}

type Agent interface {
	Run(ctx context.Context, r AgentRequest) (Response, error)
}

// Run drives Claude Code with only the Zodim MCP tools: no shell, no files,
// no web. Every tool call is pre-approved because Zodim's own policy checks
// it on the other side of the MCP connection.
func (c ClaudeCLI) Run(ctx context.Context, r AgentRequest) (Response, error) {
	bin := c.Binary
	if bin == "" {
		bin = "claude"
	}
	cfg, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"zodim": map[string]string{"type": "http", "url": r.MCPURL}}})
	args := []string{"-p", r.Prompt, "--output-format", "json", "--tools", "", "--no-session-persistence",
		"--strict-mcp-config", "--mcp-config", string(cfg), "--allowed-tools", "mcp__zodim"}
	if m := firstNonEmpty(r.Model, c.Model); m != "" {
		args = append(args, "--model", m)
	}
	if r.System != "" {
		args = append(args, "--system-prompt", r.System)
	}
	if r.MaxCostUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", r.MaxCostUSD))
	}
	if r.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprint(r.MaxTurns))
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = "/"
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	var out struct {
		Result  string  `json:"result"`
		Cost    float64 `json:"total_cost_usd"`
		IsError bool    `json:"is_error"`
		Subtype string  `json:"subtype"`
	}
	if err := json.Unmarshal(lastLine(stdout.Bytes()), &out); err != nil {
		if runErr != nil {
			return Response{}, fmt.Errorf("claude: %w: %s", runErr, strings.TrimSpace(stderr.String()))
		}
		return Response{}, fmt.Errorf("claude: unreadable output: %w", err)
	}
	resp := Response{Text: out.Result, CostUSD: out.Cost}
	if out.IsError {
		return resp, fmt.Errorf("claude: %s: %s", out.Subtype, out.Result)
	}
	return resp, nil
}

// FakeAgent calls tools through a callback, for tests.
type FakeAgent struct {
	Script func(ctx context.Context, r AgentRequest) (Response, error)
}

func (f FakeAgent) Run(ctx context.Context, r AgentRequest) (Response, error) {
	return f.Script(ctx, r)
}
