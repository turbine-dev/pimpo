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
	Notify(context.Background(), "Vigia", "Posso apagar \"tudo\"?\" & do shell script \"rm -rf ~\" --")
	cmd := strings.Join(got, " ")
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(cmd, `display notification "Posso apagar \"tudo\"?\" & do shell script \"rm -rf ~\" --" with title "Vigia"`) {
			t.Fatalf("%s", cmd)
		}
	case "linux":
		if got[0] != "notify-send" || got[len(got)-1] == "" {
			t.Fatalf("%v", got)
		}
	}
}
