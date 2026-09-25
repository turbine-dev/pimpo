package desktop

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestParsePathSkipsWhatRcFilesPrint(t *testing.T) {
	got := parsePath("Welcome back!\n" + marker + "/a/bin:/b/bin" + marker + "\nbye")
	if strings.Join(got, ",") != "/a/bin,/b/bin" {
		t.Fatalf("%v", got)
	}
	if parsePath("no markers") != nil {
		t.Fatal("parsed garbage")
	}
}

func TestMergePathKeepsOrderWithoutRepeats(t *testing.T) {
	if got := mergePath([]string{"/usr/bin", "/bin"}, []string{"/opt/x", "/usr/bin", ""}); got != "/usr/bin:/bin:/opt/x" {
		t.Fatal(got)
	}
}

// With a bare PATH, like an app opened from the Finder, tools the shell
// knows about are found again.
func TestShellPathFindsWhatTheShellFinds(t *testing.T) {
	want, err := exec.LookPath("go")
	if err != nil || os.Getenv("SHELL") == "" {
		t.Skip("no shell or go here")
	}
	t.Setenv("PATH", "/usr/bin:/bin")
	ShellPath()
	if got, err := exec.LookPath("go"); err != nil || got == "" {
		t.Fatalf("go (%s) not found with PATH %s", want, os.Getenv("PATH"))
	}
}
