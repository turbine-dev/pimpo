package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/denerFernandes/zodim/internal/llm"
)

// The Guard plugins for OpenClaw and Hermes run their own test suites
// against a real Zodim, with a paired device token.
func TestGuardPluginsAgainstZodim(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	_, pair := ta.do(t, "POST", "/api/pairing", map[string]string{"base": "http://127.0.0.1:7788", "device": "Guard"})
	token := strings.TrimPrefix(pair["link"].(string), "http://127.0.0.1:7788/auth?token=")
	env := append(os.Environ(), "ZODIM_URL="+ta.srv.URL, "ZODIM_TOKEN="+token)
	root, _ := filepath.Abs("../../guard")

	t.Run("hermes", func(t *testing.T) {
		if _, err := exec.LookPath("python3"); err != nil {
			t.Skip("python3 not installed")
		}
		cmd := exec.Command("python3", "-m", "unittest", "-v", "test_guard")
		cmd.Dir, cmd.Env = filepath.Join(root, "hermes"), env
		out, err := cmd.CombinedOutput()
		if err != nil || !regexp.MustCompile(`test_live .*\.\.\. ok`).Match(out) {
			t.Fatalf("%v\n%s", err, out)
		}
	})
	t.Run("openclaw", func(t *testing.T) {
		node, err := exec.LookPath("node")
		if err != nil {
			t.Skip("node not installed")
		}
		if v, _ := exec.Command(node, "-p", "process.versions.node.split('.')[0]").Output(); strings.TrimSpace(string(v)) < "22" {
			t.Skip("node 22.6+ runs TypeScript directly")
		}
		cmd := exec.Command(node, "--test", "guard.test.ts")
		cmd.Dir, cmd.Env = filepath.Join(root, "openclaw"), env
		out, err := cmd.CombinedOutput()
		if err != nil || strings.Contains(string(out), "# SKIP") || !strings.Contains(string(out), "pass 3") {
			t.Fatalf("%v\n%s", err, out)
		}
	})
}
