package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OpencodeCLI drives the opencode CLI with the providers the owner signed
// in to there (GitHub Copilot, OpenCode Go or Zen, OpenRouter, DeepSeek…).
// Every opencode tool is denied (shell, files, web, sub-agents, skills),
// and so is any MCP server of the owner's own opencode config; only
// Pimpo's MCP server is allowed, so the agent reaches the world through
// Pimpo's tools, where the policy checks every call. Project config, Claude
// Code prompts and skills, external plugins, sharing and auto-update are off, and
// each run's session is deleted afterwards.
//
// A model is "opencode:<provider>/<model>", as `opencode models` lists it.
type OpencodeCLI struct {
	Binary string
}

// OpencodeBinary finds the opencode CLI.
func OpencodeBinary() string {
	if p, err := exec.LookPath("opencode"); err == nil {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".opencode", "bin", "opencode")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// IsOpencode says whether a model runs through opencode.
func IsOpencode(model string) bool { return strings.HasPrefix(model, "opencode:") }

// opencodeEnv turns off everything of the owner's opencode setup that
// could add tools or instructions. opencode's default plugins stay on:
// they sign in to OAuth providers such as GitHub Copilot, and any tool
// they could add is denied like the rest.
var opencodeEnv = []string{
	"OPENCODE_DISABLE_PROJECT_CONFIG=1", "OPENCODE_DISABLE_CLAUDE_CODE=1", "OPENCODE_DISABLE_EXTERNAL_SKILLS=1",
	"OPENCODE_DISABLE_AUTOUPDATE=1", "OPENCODE_DISABLE_SHARE=1", "OPENCODE_PURE=1",
}

// opencodeConfig is the run's own config: a Pimpo agent with the system
// prompt, every tool denied but Pimpo's, and Pimpo's MCP server when
// there is one. The last matching permission rule wins, so "*" comes first.
func opencodeConfig(system, mcpURL string) []byte {
	perm := map[string]any{"*": "deny"}
	cfg := map[string]any{"share": "disabled", "autoupdate": false}
	if mcpURL != "" {
		perm = map[string]any{"*": "deny", "pimpo_*": "allow"}
		cfg["mcp"] = map[string]any{"pimpo": map[string]any{"type": "remote", "url": mcpURL, "enabled": true, "oauth": false, "timeout": 30000}}
	}
	cfg["permission"] = perm
	agent := map[string]any{"mode": "primary", "permission": perm}
	if system != "" {
		agent["prompt"] = system
	}
	cfg["agent"] = map[string]any{"pimpo": agent}
	b, _ := json.Marshal(cfg)
	return b
}

// opencodeEffort maps Pimpo's levels onto opencode's model variants.
func opencodeEffort(e string) string {
	if e == "" {
		return ""
	}
	return e // low, medium, high and max are opencode variant names too
}

func (c OpencodeCLI) run(ctx context.Context, prompt, system, model, effort, mcpURL string) (Response, error) {
	bin := c.Binary
	if bin == "" {
		bin = OpencodeBinary()
	}
	if bin == "" {
		return Response{}, errors.New("opencode: not installed")
	}
	name := strings.TrimPrefix(model, "opencode:")
	if name == "" || !strings.Contains(name, "/") {
		return Response{}, fmt.Errorf("opencode: %q is not provider/model", name)
	}
	dir, err := os.MkdirTemp("", "pimpo-opencode-")
	if err != nil {
		return Response{}, err
	}
	defer os.RemoveAll(dir)
	args := []string{"run", "--format", "json", "--pure", "--agent", "pimpo", "-m", name, "--dir", dir}
	if v := opencodeEffort(effort); v != "" {
		args = append(args, "--variant", v)
	}
	args = append(args, prompt)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), opencodeEnv...), "OPENCODE_CONFIG_CONTENT="+string(opencodeConfig(system, mcpURL)))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	resp, session, failure := readOpencode(stdout.Bytes())
	if session != "" {
		// Nothing of Pimpo's stays in the owner's opencode history.
		del := exec.Command(bin, "session", "delete", session)
		del.Env = append(os.Environ(), opencodeEnv...)
		del.Run()
	}
	// opencode can exit 0 after a provider error, so the events decide.
	if failure != "" {
		return resp, fmt.Errorf("opencode: %s", failure)
	}
	if runErr != nil {
		return resp, fmt.Errorf("opencode: %v: %s", runErr, strings.TrimSpace(lastLines(stderr.String(), 3)))
	}
	if resp.Text == "" {
		return resp, errors.New("opencode: no answer")
	}
	return resp, nil
}

// readOpencode reads the run's events: the last step's text is the answer,
// each finished step adds its cost.
func readOpencode(out []byte) (resp Response, session, failure string) {
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 32<<20)
	var step strings.Builder
	for sc.Scan() {
		var e struct {
			Type      string `json:"type"`
			SessionID string `json:"sessionID"`
			Part      struct {
				Type   string  `json:"type"`
				Text   string  `json:"text"`
				Reason string  `json:"reason"`
				Cost   float64 `json:"cost"`
			} `json:"part"`
			Error struct {
				Name string `json:"name"`
				Data struct {
					Message string `json:"message"`
				} `json:"data"`
			} `json:"error"`
		}
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		if e.SessionID != "" {
			session = e.SessionID
		}
		switch e.Type {
		case "step_start":
			step.Reset()
		case "text":
			step.WriteString(e.Part.Text)
		case "step_finish":
			resp.CostUSD += e.Part.Cost
			if t := strings.TrimSpace(step.String()); t != "" {
				resp.Text = t
			}
		case "error":
			// The first error says what went wrong; later ones are the
			// run giving up.
			if failure != "" {
				continue
			}
			failure = firstNonEmpty(e.Error.Data.Message, e.Error.Name, "the provider refused")
			if len(failure) > 300 {
				failure = failure[:300]
			}
		}
	}
	if t := strings.TrimSpace(step.String()); t != "" {
		resp.Text = t
	}
	return resp, session, failure
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// Generate answers one prompt with no tools at all; with a schema the
// answer is asked for as JSON of that shape.
func (c OpencodeCLI) Generate(ctx context.Context, r Request) (Response, error) {
	prompt := r.Prompt
	if len(r.Schema) > 0 {
		prompt += "\n\nAnswer with only a JSON value matching this JSON schema, no other text:\n" + string(r.Schema)
	}
	resp, err := c.run(ctx, prompt, r.System, r.Model, r.Effort, "")
	if err != nil {
		return resp, err
	}
	if len(r.Schema) > 0 {
		js := extractJSON(resp.Text)
		if js == nil {
			return resp, errors.New("opencode: no structured output")
		}
		resp.Structured = js
	}
	if r.MaxCostUSD > 0 && resp.CostUSD > r.MaxCostUSD {
		return resp, ErrCostLimit
	}
	return resp, nil
}

// Run is the agent loop with Pimpo's tools, and nothing else, over MCP.
func (c OpencodeCLI) Run(ctx context.Context, r AgentRequest) (Response, error) {
	resp, err := c.run(ctx, r.Prompt, r.System, r.Model, r.Effort, r.MCPURL)
	if err == nil && r.MaxCostUSD > 0 && resp.CostUSD > r.MaxCostUSD {
		return resp, ErrCostLimit
	}
	return resp, err
}

// extractJSON finds the JSON value in a model's answer, fenced or not.
func extractJSON(text string) json.RawMessage {
	t := strings.TrimSpace(text)
	if i := strings.Index(t, "```"); i >= 0 {
		rest := t[i+3:]
		rest = strings.TrimPrefix(rest, "json")
		if j := strings.Index(rest, "```"); j >= 0 {
			t = strings.TrimSpace(rest[:j])
		}
	}
	if json.Valid([]byte(t)) {
		return json.RawMessage(t)
	}
	for _, open := range []string{"{", "["} {
		i := strings.Index(t, open)
		if i < 0 {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(t[i:]))
		var v json.RawMessage
		if dec.Decode(&v) == nil {
			return v
		}
	}
	return nil
}

// OpencodeModel is a model the owner can use through opencode.
type OpencodeModel struct {
	ID       string `json:"id"` // opencode:<provider>/<model>
	Provider string `json:"provider"`
	Name     string `json:"name"`
	// Subscription says the provider is paid by a monthly plan (GitHub
	// Copilot, OpenCode Go, or any provider signed in with OAuth), so its
	// reported cost is not money spent.
	Subscription bool `json:"subscription"`
}

// OpencodeModels lists the models of the providers the owner signed in to
// in opencode.
func OpencodeModels(ctx context.Context) ([]OpencodeModel, error) {
	bin := OpencodeBinary()
	if bin == "" {
		return nil, errors.New("opencode is not installed")
	}
	cmd := exec.CommandContext(ctx, bin, "models")
	cmd.Env = append(os.Environ(), opencodeEnv...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("opencode models: %w", err)
	}
	subs := opencodeSubscriptions(ctx, bin)
	var list []OpencodeModel
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		provider, name, ok := strings.Cut(line, "/")
		// OpenCode's own models (the free tier) refuse to run outside the
		// OpenCode agent, so they are left out.
		if !ok || strings.ContainsAny(line, " \t") || name == "" || provider == "opencode" {
			continue
		}
		list = append(list, OpencodeModel{ID: "opencode:" + line, Provider: provider, Name: name, Subscription: subs[provider]})
	}
	return list, nil
}

// OpencodeSubscription says whether a provider is paid by a plan.
func OpencodeSubscription(ctx context.Context, provider string) bool {
	bin := OpencodeBinary()
	return bin != "" && opencodeSubscriptions(ctx, bin)[provider]
}

var ansi = strings.NewReplacer("\x1b[0m", "", "\x1b[1m", "", "\x1b[2m", "", "\x1b[22m", "", "\x1b[39m", "", "\x1b[90m", "", "\x1b[32m", "", "\x1b[36m", "")

// opencodeSubscriptions reads `opencode auth list` (names and kinds, no
// secrets): OAuth sign-ins are plans; OpenCode Go is a plan with a key.
func opencodeSubscriptions(ctx context.Context, bin string) map[string]bool {
	subs := map[string]bool{"opencode-go": true, "github-copilot": true}
	cmd := exec.CommandContext(ctx, bin, "auth", "list")
	cmd.Env = append(os.Environ(), opencodeEnv...)
	out, _ := cmd.CombinedOutput()
	for _, line := range strings.Split(ansi.Replace(string(out)), "\n") {
		f := strings.Fields(strings.Trim(line, "│●┌└ "))
		if len(f) < 2 || f[len(f)-1] != "oauth" {
			continue
		}
		id := strings.ToLower(strings.Join(f[:len(f)-1], "-"))
		subs[id] = true
	}
	return subs
}
