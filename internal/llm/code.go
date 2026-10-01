package llm

import (
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

// Coding CLIs that work on files. Unlike agent runs, which reach the world
// only through Pimpo's tools, these get their own tools, confined to one
// folder: reading and editing files and running commands there. They get
// no Pimpo tools, no web, and an environment of their own.
const (
	CoderClaude   = "claude"
	CoderCodex    = "codex"
	CoderOpencode = "opencode"
)

// A CodeRequest has a coding CLI work in Dir.
type CodeRequest struct {
	// Coder is claude, codex, or opencode:<provider>/<model>.
	Coder      string
	Model      string
	Dir        string
	Prompt     string
	System     string
	Env        []string
	Sandbox    bool
	MaxCostUSD float64
	MaxTurns   int
}

type Coder interface {
	Code(ctx context.Context, r CodeRequest) (Response, error)
}

// CodeCLI runs the coding CLIs installed on this computer.
type CodeCLI struct{}

// CoderOf names the CLI a coder setting runs.
func CoderOf(coder string) string {
	switch {
	case coder == "" || coder == CoderClaude || strings.HasPrefix(coder, "claude:"):
		return CoderClaude
	case coder == CoderCodex || strings.HasPrefix(coder, "codex:"):
		return CoderCodex
	case IsOpencode(coder):
		return CoderOpencode
	}
	return ""
}

func (CodeCLI) Code(ctx context.Context, r CodeRequest) (Response, error) {
	if st, err := os.Stat(r.Dir); err != nil || !st.IsDir() {
		return Response{}, fmt.Errorf("no folder to work in: %s", r.Dir)
	}
	// A CLI that confines itself to its folder compares real paths: one
	// reached through a link (macOS's /var is) would be outside it.
	if real, err := filepath.EvalSymlinks(r.Dir); err == nil {
		r.Dir = real
	}
	switch CoderOf(r.Coder) {
	case CoderClaude:
		return codeClaude(ctx, r)
	case CoderCodex:
		return codeCodex(ctx, r)
	case CoderOpencode:
		if r.Sandbox {
			return Response{}, errors.New("opencode has no sandbox; choose Claude Code or Codex, or turn the sandbox off")
		}
		return codeOpencode(ctx, r)
	}
	return Response{}, fmt.Errorf("unknown coding CLI %q", r.Coder)
}

// claudeSandbox keeps every command in Claude Code's sandbox: writes only
// in the folder, the network only through its proxy, and no command
// allowed to leave it.
const claudeSandbox = `{"sandbox":{"enabled":true,"autoAllowBashIfSandboxed":true,"allowUnsandboxedCommands":false}}`

func codeClaude(ctx context.Context, r CodeRequest) (Response, error) {
	// Settings come from the person's own Claude Code only: a repository's
	// settings and hooks do not run. Anything that would ask is refused.
	args := []string{"-p", r.Prompt, "--output-format", "json", "--no-session-persistence", "--strict-mcp-config",
		"--setting-sources", "user", "--permission-mode", "acceptEdits", "--permission-prompts", "none",
		"--allowed-tools", "Read", "Edit", "Write", "Glob", "Grep", "Bash",
		"--disallowed-tools", "WebFetch", "WebSearch"}
	if r.Sandbox {
		args = append(args, "--settings", claudeSandbox)
	}
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}
	if r.System != "" {
		args = append(args, "--append-system-prompt", r.System)
	}
	if r.MaxCostUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", r.MaxCostUSD))
	}
	if r.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprint(r.MaxTurns))
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir, cmd.Env = r.Dir, r.Env
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
			return Response{}, fmt.Errorf("claude: %w: %s", runErr, lastLines(stderr.String(), 3))
		}
		return Response{}, fmt.Errorf("claude: unreadable output: %w", err)
	}
	resp := Response{Text: out.Result, CostUSD: out.Cost}
	if out.IsError {
		return resp, fmt.Errorf("claude: %s: %s", out.Subtype, out.Result)
	}
	return resp, nil
}

func codeCodex(ctx context.Context, r CodeRequest) (Response, error) {
	bin := CodexBinary()
	if bin == "" {
		return Response{}, errors.New("codex: not installed")
	}
	tmp, err := os.MkdirTemp("", "pimpo-code-")
	if err != nil {
		return Response{}, err
	}
	defer os.RemoveAll(tmp)
	last := filepath.Join(tmp, "last.txt")
	// Codex's own sandbox writes only in the folder and keeps the network
	// out; without it, commands run as the person does.
	mode := "danger-full-access"
	if r.Sandbox {
		mode = "workspace-write"
	}
	args := []string{"exec", "--json", "--ephemeral", "--ignore-user-config", "--ignore-rules", "--skip-git-repo-check", "-s", mode, "-C", r.Dir, "-o", last,
		"-c", `web_search="disabled"`}
	for _, f := range []string{"browser_use", "browser_use_external", "computer_use", "apps", "multi_agent"} {
		args = append(args, "--disable", f)
	}
	if r.System != "" {
		quoted, _ := json.Marshal(r.System)
		args = append(args, "-c", "developer_instructions="+string(quoted))
	}
	if m := strings.TrimPrefix(firstNonEmpty(r.Model, r.Coder), "codex:"); m != "" && m != CoderCodex {
		args = append(args, "-m", m)
	}
	cmd := exec.CommandContext(ctx, bin, append(args, "-")...)
	cmd.Dir, cmd.Env = r.Dir, r.Env
	cmd.Stdin = strings.NewReader(r.Prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	failure := codexFailure(stdout.Bytes())
	text, _ := os.ReadFile(last)
	if runErr != nil || failure != "" && len(bytes.TrimSpace(text)) == 0 {
		return Response{}, fmt.Errorf("codex: %s", firstNonEmpty(failure, lastLines(stderr.String(), 3), fmt.Sprint(runErr)))
	}
	return Response{Text: strings.TrimSpace(string(text))}, nil
}

// opencodeCoding allows the tools that work on the folder's files and
// commands, and nothing on the web.
func opencodeCoding(system string) []byte {
	perm := map[string]any{"*": "deny", "read": "allow", "edit": "allow", "write": "allow", "glob": "allow", "grep": "allow", "list": "allow", "bash": "allow", "external_directory": "deny"}
	agent := map[string]any{"mode": "primary", "permission": perm}
	if system != "" {
		agent["prompt"] = system
	}
	b, _ := json.Marshal(map[string]any{"share": "disabled", "autoupdate": false, "permission": perm, "agent": map[string]any{"pimpo": agent}})
	return b
}

func codeOpencode(ctx context.Context, r CodeRequest) (Response, error) {
	bin := OpencodeBinary()
	if bin == "" {
		return Response{}, errors.New("opencode: not installed")
	}
	name := strings.TrimPrefix(r.Coder, "opencode:")
	if !strings.Contains(name, "/") {
		return Response{}, fmt.Errorf("opencode: %q is not provider/model", name)
	}
	cmd := exec.CommandContext(ctx, bin, "run", "--format", "json", "--agent", "pimpo", "-m", name, "--dir", r.Dir, r.Prompt)
	cmd.Dir = r.Dir
	env := append(append([]string{}, r.Env...), "OPENCODE_DISABLE_PROJECT_CONFIG=1", "OPENCODE_DISABLE_EXTERNAL_SKILLS=1", "OPENCODE_DISABLE_AUTOUPDATE=1", "OPENCODE_DISABLE_SHARE=1")
	cmd.Env = append(env, "OPENCODE_CONFIG_CONTENT="+string(opencodeCoding(r.System)))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	resp, session, failure := readOpencode(stdout.Bytes())
	if session != "" {
		del := exec.Command(bin, "session", "delete", session)
		del.Env = env
		del.Run()
	}
	if failure != "" {
		return resp, fmt.Errorf("opencode: %s", failure)
	}
	if runErr != nil {
		return resp, fmt.Errorf("opencode: %v: %s", runErr, lastLines(stderr.String(), 3))
	}
	if r.MaxCostUSD > 0 && resp.CostUSD > r.MaxCostUSD {
		return resp, ErrCostLimit
	}
	return resp, nil
}

// codeEnvKeep are the variables a coding CLI needs from Pimpo's own
// environment: where things are, and how each CLI signs in.
var codeEnvKeep = []string{"PATH", "HOME", "USER", "LOGNAME", "SHELL", "LANG", "LC_ALL", "TMPDIR", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
	"ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CONFIG_DIR", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX",
	"OPENAI_API_KEY", "CODEX_HOME"}

// CodeEnv is a coding CLI's environment: what it needs to run and sign in,
// and the variables given, nothing else of Pimpo's.
func CodeEnv(vars map[string]string) []string {
	// Without the person's git config the CLI finds none of their
	// credentials, so it cannot push on its own.
	out := []string{"TERM=dumb", "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
	for _, k := range codeEnvKeep {
		if v, ok := os.LookupEnv(k); ok {
			out = append(out, k+"="+v)
		}
	}
	for k, v := range vars {
		out = append(out, k+"="+v)
	}
	return out
}

// FakeCoder answers code requests through a callback, for tests.
type FakeCoder func(ctx context.Context, r CodeRequest) (Response, error)

func (f FakeCoder) Code(ctx context.Context, r CodeRequest) (Response, error) { return f(ctx, r) }
