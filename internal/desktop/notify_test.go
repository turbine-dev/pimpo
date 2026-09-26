package desktop

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestNotifyEscapes(t *testing.T) {
	var got []string
	Run = func(_ context.Context, name string, args ...string) error {
		got = append([]string{name}, args...)
		return nil
	}
	Notify(context.Background(), "Pimpo", "Posso apagar \"tudo\"?\" & do shell script \"rm -rf ~\" --")
	cmd := strings.Join(got, " ")
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(cmd, `display notification "Posso apagar \"tudo\"?\" & do shell script \"rm -rf ~\" --" with title "Pimpo"`) {
			t.Fatalf("%s", cmd)
		}
	case "linux":
		if got[0] != "notify-send" || got[len(got)-1] == "" {
			t.Fatalf("%v", got)
		}
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
