package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A stand-in claude that writes down how it was run and answers.
func fakeClaude(t *testing.T) (log string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in tool is a shell script")
	}
	dir := t.TempDir()
	log = filepath.Join(dir, "log")
	script := `#!/bin/sh
printf '%s\n' "$@" > "` + log + `"
echo "PWD=$(pwd)" >> "` + log + `"
env >> "` + log + `"
echo '{"result":"Changed the cart.","total_cost_usd":0.12,"is_error":false}'
`
	os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func TestClaudeCodesInItsFolderOnly(t *testing.T) {
	log := fakeClaude(t)
	t.Setenv("PIMPO_SECRET_THING", "do-not-pass")
	work := t.TempDir()
	resp, err := CodeCLI{}.Code(context.Background(), CodeRequest{Coder: "claude", Model: "opus", Dir: work, Prompt: "Fix it", System: "You are Bia.",
		Env: CodeEnv(map[string]string{"SHOP_API": "x"}), Sandbox: true, MaxCostUSD: 2})
	if err != nil || resp.Text != "Changed the cart." || resp.CostUSD != 0.12 {
		t.Fatalf("%+v %v", resp, err)
	}
	b, _ := os.ReadFile(log)
	got := string(b)
	for _, want := range []string{"--setting-sources\nuser\n", "--permission-prompts\nnone\n", "--disallowed-tools\nWebFetch\nWebSearch\n", "--append-system-prompt\nYou are Bia.\n", "--max-budget-usd\n2.00\n", "--model\nopus\n", "SHOP_API=x", "GIT_CONFIG_GLOBAL=/dev/null"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if !strings.Contains(got, `"sandbox":{"enabled":true`) || strings.Contains(got, "do-not-pass") || strings.Contains(got, "--mcp-config") {
		t.Fatalf("run:\n%s", got)
	}
	if real, _ := filepath.EvalSymlinks(work); !strings.Contains(got, "PWD="+real) && !strings.Contains(got, "PWD="+work) {
		t.Fatalf("not run in its folder:\n%s", got)
	}

	CodeCLI{}.Code(context.Background(), CodeRequest{Coder: "claude", Dir: work, Prompt: "Fix it", Env: CodeEnv(nil)})
	if b, _ := os.ReadFile(log); strings.Contains(string(b), "sandbox") {
		t.Fatal("a sandbox nobody asked for")
	}
}

func TestCodingCLIsAreKnownByName(t *testing.T) {
	for in, want := range map[string]string{"": CoderClaude, "claude:opus": CoderClaude, "codex": CoderCodex, "codex:gpt-5": CoderCodex, "opencode:zen/qwen": CoderOpencode, "vim": ""} {
		if got := CoderOf(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
	if _, err := (CodeCLI{}).Code(context.Background(), CodeRequest{Coder: "opencode:zen/qwen", Dir: t.TempDir(), Sandbox: true}); err == nil || !strings.Contains(err.Error(), "no sandbox") {
		t.Fatalf("opencode in a sandbox: %v", err)
	}
	if _, err := (CodeCLI{}).Code(context.Background(), CodeRequest{Coder: "claude", Dir: "/no/such/folder"}); err == nil {
		t.Fatal("ran without a folder")
	}
	var cfg struct {
		Permission map[string]string `json:"permission"`
	}
	json.Unmarshal(opencodeCoding(""), &cfg)
	if cfg.Permission["*"] != "deny" || cfg.Permission["webfetch"] != "" || cfg.Permission["edit"] != "allow" || cfg.Permission["external_directory"] != "deny" {
		t.Fatalf("opencode's permissions = %v", cfg.Permission)
	}
}
