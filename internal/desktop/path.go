package desktop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Apps opened from the Finder or a launcher get a bare PATH, without what
// the owner's shell adds (nvm, Homebrew, ~/.local/bin), so tools like the
// claude CLI look missing. ShellPath adds the login shell's PATH, and the
// usual install folders, to this process.
func ShellPath() {
	parts := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))
	home, _ := os.UserHomeDir()
	extra := append(loginPath(), filepath.Join(home, ".local", "bin"), filepath.Join(home, ".claude", "local"), "/opt/homebrew/bin", "/usr/local/bin")
	os.Setenv("PATH", mergePath(parts, extra))
}

func mergePath(have, add []string) string {
	seen := map[string]bool{}
	var out []string
	for _, p := range append(have, add...) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return strings.Join(out, string(os.PathListSeparator))
}

const marker = "__zodim_path__"

// loginPath asks the owner's shell, interactive so rc files run, for its
// PATH; the markers skip anything the rc files print.
func loginPath() []string {
	sh := os.Getenv("SHELL")
	if sh == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, sh, "-ilc", `printf '`+marker+`%s`+marker+`' "$PATH"`)
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parsePath(string(out))
}

func parsePath(out string) []string {
	i := strings.Index(out, marker)
	j := strings.LastIndex(out, marker)
	if i < 0 || j <= i {
		return nil
	}
	return strings.Split(out[i+len(marker):j], string(os.PathListSeparator))
}
