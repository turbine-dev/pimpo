package desktop

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParsePathSkipsWhatRcFilesPrint(t *testing.T) {
	unixOnly(t)
	got := parsePath("Welcome back!\n" + marker + "/a/bin:/b/bin" + marker + "\nbye")
	if strings.Join(got, ",") != "/a/bin,/b/bin" {
		t.Fatalf("%v", got)
	}
	if parsePath("no markers") != nil {
		t.Fatal("parsed garbage")
	}
}

func TestMergePathKeepsOrderWithoutRepeats(t *testing.T) {
	unixOnly(t)
	if got := mergePath([]string{"/usr/bin", "/bin"}, []string{"/opt/x", "/usr/bin", ""}); got != "/usr/bin:/bin:/opt/x" {
		t.Fatal(got)
	}
}

// With a bare PATH, like an app opened from the Finder, tools the shell
// knows about are found again.
func TestShellPathFindsWhatTheShellFinds(t *testing.T) {
	unixOnly(t)
	if os.Getenv("CI") != "" {
		t.Skip("CI runners put go on PATH without telling the login shell")
	}
	want, err := exec.LookPath("go")
	if err != nil || os.Getenv("SHELL") == "" {
		t.Skip("no shell or go here")
	}
	// CI puts go on PATH without telling the login shell.
	if out, err := exec.Command(os.Getenv("SHELL"), "-lc", "PATH=/usr/bin:/bin; . /etc/profile >/dev/null 2>&1; command -v go").Output(); err != nil || len(out) == 0 {
		if out, err := exec.Command(os.Getenv("SHELL"), "-ilc", "command -v go").Output(); err != nil || len(out) == 0 {
			t.Skip("the login shell does not know go")
		}
	}
	t.Setenv("PATH", "/usr/bin:/bin")
	ShellPath()
	if got, err := exec.LookPath("go"); err != nil || got == "" {
		t.Fatalf("go (%s) not found with PATH %s", want, os.Getenv("PATH"))
	}
}

// A claude installed with nvm is found at once, without waiting for the
// shell, as an app opened from the Finder needs.
func TestKnownPathFindsNvmAtOnce(t *testing.T) {
	unixOnly(t)
	home := t.TempDir()
	bin := filepath.Join(home, ".nvm", "versions", "node", "v24.1.0", "bin")
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("PATH", "/usr/bin:/bin")
	KnownPath()
	if got, err := exec.LookPath("claude"); err != nil || got != filepath.Join(bin, "claude") {
		t.Fatalf("claude not found: %v %s (PATH %s)", err, got, os.Getenv("PATH"))
	}
}

// The login shell's PATH is a macOS and Linux matter.
func unixOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no login shell PATH on Windows")
	}
}
