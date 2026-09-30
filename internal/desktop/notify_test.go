package desktop

import (
	"context"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// Hostile text: ASCII and curly quotes (PowerShell treats U+2018..U+201B
// as quotes too), AppleScript and shell tricks, and a leading dash.
const hostile = "-x Posso apagar \"tudo\"?\" & do shell script \"rm -rf ~\" \u2018) ; calc ; (\u2019 \u201a \u201b ' $(calc) --"

func TestNotifyKeepsTextOutOfScripts(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		name, args, env := notifyCommand(goos, "Pimpo "+hostile, hostile)
		switch goos {
		case "windows":
			if name != "powershell" || args[len(args)-1] != toastScript {
				t.Fatalf("%s %v", name, args)
			}
			for _, a := range args {
				if strings.Contains(a, "rm -rf") || strings.ContainsAny(a, "\u2018\u2019\u201a\u201b") {
					t.Fatalf("user text in the command line: %q", a)
				}
			}
			if !slices.Equal(env, []string{"PIMPO_TITLE=Pimpo " + hostile, "PIMPO_BODY=" + hostile}) {
				t.Fatalf("%q", env)
			}
		default:
			// Text goes after "--" as whole arguments; the script parts before it hold none of it.
			i := slices.Index(args, "--")
			if i < 0 || !slices.Equal(args[i+1:], []string{"Pimpo " + hostile, hostile}) || env != nil {
				t.Fatalf("%s %q %q", goos, args, env)
			}
			for _, a := range args[:i] {
				if strings.Contains(a, "rm -rf") {
					t.Fatalf("user text in the script: %q", a)
				}
			}
		}
	}
}

func TestNotifyRunsTheCommand(t *testing.T) {
	var got []string
	var gotEnv []string
	RunEnv = func(_ context.Context, env []string, name string, args ...string) error {
		got, gotEnv = append([]string{name}, args...), env
		return nil
	}
	Notify(context.Background(), "Pimpo", "  two\n lines ")
	want, wantArgs, wantEnv := notifyCommand(runtime.GOOS, "Pimpo", "two lines")
	if !slices.Equal(got, append([]string{want}, wantArgs...)) || !slices.Equal(gotEnv, wantEnv) {
		t.Fatalf("%q %q", got, gotEnv)
	}
}

func TestOpenOnlyWebAndMailLinks(t *testing.T) {
	var got []string
	Run = func(_ context.Context, name string, args ...string) error {
		got = append([]string{name}, args...)
		return nil
	}
	for _, bad := range []string{"file:///etc/passwd", "javascript:alert(1)", "/Applications/Calculator.app", "smb://host/share"} {
		if err := Open(context.Background(), bad); err == nil {
			t.Errorf("opened %s", bad)
		}
	}
	if err := Open(context.Background(), "https://login.tailscale.com/a/abc"); err != nil || got[len(got)-1] != "https://login.tailscale.com/a/abc" {
		t.Fatalf("%v %v", got, err)
	}
}
