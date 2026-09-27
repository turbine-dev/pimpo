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

// CodexCLI drives OpenAI's Codex CLI with the owner's ChatGPT login. Codex
// brings tools of its own (a shell, a browser, computer control, apps,
// sub-agents); those that could act outside Pimpo are turned off, the owner's Codex config (and
// the MCP servers it adds, such as computer control) is ignored, and the
// sandbox is read-only, so the agent reaches the world only through
// Pimpo's tools over MCP, where the policy checks every call.
type CodexCLI struct {
	Binary string
	Model  string
}

// CodexBinary finds the Codex CLI: on the PATH, or inside the ChatGPT app.
func CodexBinary() string {
	if p, err := exec.LookPath("codex"); err == nil {
		return p
	}
	for _, p := range []string{"/Applications/ChatGPT.app/Contents/Resources/codex", filepath.Join(os.Getenv("HOME"), "Applications/ChatGPT.app/Contents/Resources/codex")} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// codexOff are the Codex features that could reach the computer or the
// web outside Pimpo; each was checked against the real CLI (a text file,
// an image and a website stayed out of reach). Code mode stays on: it is
// how Codex calls MCP tools, and what is left in it cannot read anything.
var codexOff = []string{"shell_tool", "unified_exec", "browser_use", "browser_use_external", "computer_use", "apps", "multi_agent", "view_image"}

func (c CodexCLI) run(ctx context.Context, prompt, system, model string, extra []string) (string, error) {
	bin := c.Binary
	if bin == "" {
		bin = CodexBinary()
	}
	if bin == "" {
		return "", errors.New("codex: not installed")
	}
	dir, err := os.MkdirTemp("", "pimpo-codex-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	last := filepath.Join(dir, "last.txt")
	args := []string{"exec", "--json", "--ephemeral", "--ignore-user-config", "--ignore-rules", "--skip-git-repo-check",
		"-s", "read-only", "-C", dir, "-o", last}
	for _, f := range codexOff {
		args = append(args, "--disable", f)
	}
	args = append(args, "-c", `web_search="disabled"`)
	if system != "" {
		quoted, _ := json.Marshal(system)
		args = append(args, "-c", "developer_instructions="+string(quoted))
	}
	if m := firstNonEmpty(model, c.Model); m != "" && m != "codex" {
		args = append(args, "-m", strings.TrimPrefix(m, "codex:"))
	}
	args = append(append(args, extra...), "-")
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	// The event stream carries failures the exit code does not explain.
	var failure string
	sc := bufio.NewScanner(&stdout)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var e struct {
			Type  string `json:"type"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		switch e.Type {
		case "turn.failed":
			failure = firstNonEmpty(e.Error.Message, e.Message)
		case "error":
			failure = firstNonEmpty(e.Message, e.Error.Message, failure)
		}
	}
	text, _ := os.ReadFile(last)
	if runErr != nil || (failure != "" && len(bytes.TrimSpace(text)) == 0) {
		why := firstNonEmpty(failure, strings.TrimSpace(stderr.String()))
		if runErr != nil {
			return "", fmt.Errorf("codex: %v: %s", runErr, why)
		}
		return "", fmt.Errorf("codex: %s", why)
	}
	return strings.TrimSpace(string(text)), nil
}

// Generate answers one prompt; with a schema, Codex is held to it.
func (c CodexCLI) Generate(ctx context.Context, r Request) (Response, error) {
	var extra []string
	if len(r.Schema) > 0 {
		f, err := os.CreateTemp("", "pimpo-schema-*.json")
		if err != nil {
			return Response{}, err
		}
		defer os.Remove(f.Name())
		f.Write(r.Schema)
		f.Close()
		extra = append(extra, "--output-schema", f.Name())
	}
	text, err := c.run(ctx, r.Prompt, r.System, r.Model, extra)
	if err != nil {
		return Response{}, err
	}
	resp := Response{Text: text}
	if len(r.Schema) > 0 {
		if !json.Valid([]byte(text)) {
			return resp, errors.New("codex: no structured output")
		}
		resp.Structured = json.RawMessage(text)
	}
	return resp, nil
}

// Run is the agent loop with Pimpo's tools, and nothing else, over MCP.
func (c CodexCLI) Run(ctx context.Context, r AgentRequest) (Response, error) {
	url, _ := json.Marshal(r.MCPURL)
	// Pimpo's tools are pre-approved here because Pimpo's own policy and
	// approvals check every call on the other side.
	extra := []string{"-c", "mcp_servers.pimpo.url=" + string(url), "-c", `mcp_servers.pimpo.default_tools_approval_mode="approve"`}
	text, err := c.run(ctx, r.Prompt, r.System, r.Model, extra)
	return Response{Text: text}, err
}
