package desktop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Apps opened from the Finder or a launcher get a bare PATH, without what
// the owner's shell adds (nvm, Homebrew, ~/.local/bin), so tools like the
// claude CLI look missing. KnownPath adds the usual install folders and the
// PATH found last time, at once; ShellPath then asks the login shell, which
// can take seconds on a busy machine, and remembers the answer.
func KnownPath() {
	add := knownDirs()
	add = append(add, cachedPath()...)
	os.Setenv("PATH", mergePath(strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)), add))
}

func ShellPath() {
	KnownPath()
	found := loginPath()
	if len(found) == 0 {
		return
	}
	os.Setenv("PATH", mergePath(strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)), found))
	if f := cacheFile(); f != "" {
		os.MkdirAll(filepath.Dir(f), 0o700)
		os.WriteFile(f, []byte(strings.Join(found, string(os.PathListSeparator))), 0o600)
	}
}

// knownDirs are the folders installers put the claude CLI and its kin in,
// those that exist, newest Node version first.
func knownDirs() []string {
	home, _ := os.UserHomeDir()
	dirs := []string{filepath.Join(home, ".claude", "local"), filepath.Join(home, ".local", "bin")}
	nvm, _ := filepath.Glob(filepath.Join(home, ".nvm", "versions", "node", "*", "bin"))
	sort.Sort(sort.Reverse(sort.StringSlice(nvm)))
	dirs = append(dirs, nvm...)
	dirs = append(dirs, filepath.Join(home, ".volta", "bin"), filepath.Join(home, ".bun", "bin"), filepath.Join(home, ".npm-global", "bin"),
		filepath.Join(home, ".asdf", "shims"), filepath.Join(home, "Library", "pnpm"), "/opt/homebrew/bin", "/usr/local/bin")
	var out []string
	for _, d := range dirs {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			out = append(out, d)
		}
	}
	return out
}

func cacheFile() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "zodim", "shell-path")
}

func cachedPath() []string {
	f := cacheFile()
	if f == "" {
		return nil
	}
	b, err := os.ReadFile(f)
	if err != nil || len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), string(os.PathListSeparator))
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
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
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
