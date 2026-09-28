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

// KimiCLI drives the Kimi Code CLI with the owner's Kimi Code plan. Each
// run gets a home of its own (KIMI_CODE_HOME) that links to the owner's
// sign-in and config but holds only Pimpo's MCP server, and an agent whose
// tools are Pimpo's alone: no shell, files, web, sub-agents or skills of
// Kimi's own, so the agent reaches the world through Pimpo's tools, where
// the policy checks every call. The run's session is kept in that home and
// goes with it.
//
// A model is "kimi" (the owner's default) or "kimi:<alias>".
type KimiCLI struct {
	Binary string
	// Home is the owner's Kimi Code home; "" is KIMI_CODE_HOME or
	// ~/.kimi-code.
	Home string
}

// IsKimi says whether a model runs through Kimi Code.
func IsKimi(model string) bool { return model == "kimi" || strings.HasPrefix(model, "kimi:") }

// KimiBinary finds the Kimi Code CLI.
func KimiBinary() string {
	if p, err := exec.LookPath("kimi"); err == nil {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".kimi-code", "bin", "kimi")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// KimiHome is where Kimi Code keeps the owner's sign-in and config.
func KimiHome() string {
	if h := os.Getenv("KIMI_CODE_HOME"); h != "" {
		return h
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kimi-code")
}

// KimiSubscription says whether Kimi Code is signed in with a Kimi Code
// plan (kimi login), whose provider Kimi names managed:kimi-code. An API
// key gives no cost Pimpo could count, so only the plan is used.
func KimiSubscription(home string) bool {
	if home == "" {
		home = KimiHome()
	}
	b, err := os.ReadFile(filepath.Join(home, "config.toml"))
	return err == nil && bytes.Contains(b, []byte("managed:kimi-code"))
}

// kimiAgent is the run's agent: the system prompt, and Pimpo's tools only
// (none at all without an MCP server).
func kimiAgent(system string, tools bool) []byte {
	allowed := "[]"
	if tools {
		allowed = `["mcp__pimpo__*"]`
	}
	// The body is a template; ${…} in the prompt must stay text.
	body := strings.ReplaceAll(system, "${", "$ {")
	if body == "" {
		body = "You are a helpful assistant."
	}
	return []byte("---\nname: pimpo\ndescription: Pimpo's agent, with Pimpo's tools only.\ntools: " + allowed + "\ndisallowedTools: []\nsubagents: []\n---\n" + body + "\n")
}

func (c KimiCLI) run(ctx context.Context, prompt, system, model, effort, mcpURL string) (Response, error) {
	bin := c.Binary
	if bin == "" {
		bin = KimiBinary()
	}
	if bin == "" {
		return Response{}, errors.New("kimi: not installed")
	}
	owner := c.Home
	if owner == "" {
		owner = KimiHome()
	}
	if !KimiSubscription(owner) {
		return Response{}, errors.New("kimi: sign in to Kimi Code with your plan (run `kimi login`); an API key reports no cost Pimpo can count")
	}
	root, err := os.MkdirTemp("", "pimpo-kimi-")
	if err != nil {
		return Response{}, err
	}
	defer os.RemoveAll(root)
	home, work := filepath.Join(root, "home"), filepath.Join(root, "work")
	os.MkdirAll(home, 0o700)
	os.MkdirAll(work, 0o700)
	// The owner's sign-in and settings, linked, not copied; their own MCP
	// servers, skills and sessions stay out.
	for _, name := range []string{"config.toml", "credentials"} {
		if _, err := os.Stat(filepath.Join(owner, name)); err == nil {
			os.Symlink(filepath.Join(owner, name), filepath.Join(home, name))
		}
	}
	servers := map[string]any{}
	if mcpURL != "" {
		servers["pimpo"] = map[string]any{"url": mcpURL}
	}
	mcp, _ := json.Marshal(map[string]any{"mcpServers": servers})
	os.WriteFile(filepath.Join(home, "mcp.json"), mcp, 0o600)
	agent := filepath.Join(root, "pimpo.md")
	os.WriteFile(agent, kimiAgent(system, mcpURL != ""), 0o600)

	args := []string{"-p", prompt, "--output-format", "stream-json", "--agent-file", agent}
	if alias := strings.TrimPrefix(model, "kimi:"); alias != "" && alias != "kimi" {
		args = append(args, "-m", alias)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "KIMI_CODE_HOME="+home)
	if effort != "" {
		cmd.Env = append(cmd.Env, "KIMI_MODEL_THINKING_EFFORT="+effort)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	text := readKimi(stdout.Bytes())
	if runErr != nil {
		why := strings.TrimSpace(lastLines(stderr.String(), 2))
		why = strings.TrimPrefix(why, "error: ")
		return Response{Text: text}, fmt.Errorf("kimi: %s", firstNonEmpty(why, runErr.Error()))
	}
	if text == "" {
		return Response{}, errors.New("kimi: no answer")
	}
	return Response{Text: text}, nil
}

// readKimi takes the answer from the stream: the last assistant message
// with text.
func readKimi(out []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 32<<20)
	text := ""
	for sc.Scan() {
		var m struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(sc.Bytes(), &m) != nil || m.Role != "assistant" {
			continue
		}
		var s string
		if json.Unmarshal(m.Content, &s) != nil {
			var parts []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			json.Unmarshal(m.Content, &parts)
			var b strings.Builder
			for _, p := range parts {
				if p.Type == "text" {
					b.WriteString(p.Text)
				}
			}
			s = b.String()
		}
		if t := strings.TrimSpace(s); t != "" {
			text = t
		}
	}
	return text
}

// Generate answers one prompt with no tools; with a schema the answer is
// asked for as JSON of that shape.
func (c KimiCLI) Generate(ctx context.Context, r Request) (Response, error) {
	prompt := r.Prompt
	if len(r.Schema) > 0 {
		prompt += "\n\nAnswer with only a JSON value matching this JSON schema, no other text:\n" + string(r.Schema)
	}
	resp, err := c.run(ctx, prompt, r.System, r.Model, r.Effort, "")
	if err != nil || len(r.Schema) == 0 {
		return resp, err
	}
	if resp.Structured = extractJSON(resp.Text); resp.Structured == nil {
		return resp, errors.New("kimi: no structured output")
	}
	return resp, nil
}

// Run is the agent loop with Pimpo's tools, and nothing else, over MCP.
func (c KimiCLI) Run(ctx context.Context, r AgentRequest) (Response, error) {
	return c.run(ctx, r.Prompt, r.System, r.Model, r.Effort, r.MCPURL)
}
