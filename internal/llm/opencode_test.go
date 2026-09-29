package llm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A fake opencode that finds its database locked on the first two runs,
// as happens when several start at once, then answers.
func TestOpencodeRetriesALockedDatabase(t *testing.T) {
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	bin := filepath.Join(dir, "opencode")
	script := `#!/bin/sh
case "$1" in session) exit 0 ;; esac
n=$(cat "` + count + `" 2>/dev/null || echo 0); n=$((n+1)); echo $n > "` + count + `"
if [ $n -le 2 ]; then echo "Error: Unexpected error" >&2; echo "database is locked" >&2; exit 1; fi
echo '{"type":"step_start","sessionID":"s1"}'
echo '{"type":"text","sessionID":"s1","part":{"type":"text","text":"ok"}}'
echo '{"type":"step_finish","sessionID":"s1","part":{"type":"step-finish","cost":0.001}}'
`
	os.WriteFile(bin, []byte(script), 0o755)
	old := opencodeLockPause
	opencodeLockPause = 10 * time.Millisecond
	defer func() { opencodeLockPause = old }()
	resp, err := OpencodeCLI{Binary: bin}.Generate(context.Background(), Request{Prompt: "hi", Model: "opencode:deepseek/deepseek-flash"})
	if err != nil || resp.Text != "ok" {
		t.Fatalf("%+v %v", resp, err)
	}
	if b, _ := os.ReadFile(count); strings.TrimSpace(string(b)) != "3" {
		t.Fatalf("ran %s times", b)
	}
}
