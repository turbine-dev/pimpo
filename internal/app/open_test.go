package app

import (
	"context"
	"testing"

	"github.com/turbine-dev/pimpo/internal/desktop"
	"github.com/turbine-dev/pimpo/internal/llm"
)

func TestDesktopWindowOpensLinksInTheBrowser(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	var opened string
	desktop.Run = func(_ context.Context, _ string, args ...string) error { opened = args[len(args)-1]; return nil }
	if code, _ := ta.do(t, "POST", "/api/open", map[string]string{"url": "https://login.tailscale.com/a/x"}); code != 403 || opened != "" {
		t.Fatalf("outside the desktop app: %d %q", code, opened)
	}
	ta.DesktopNotify = true
	if code, _ := ta.do(t, "POST", "/api/open", map[string]string{"url": "file:///etc/passwd"}); code != 400 || opened != "" {
		t.Fatalf("file link: %d %q", code, opened)
	}
	if code, _ := ta.do(t, "POST", "/api/open", map[string]string{"url": "https://login.tailscale.com/a/x"}); code != 200 || opened != "https://login.tailscale.com/a/x" {
		t.Fatalf("%d %q", code, opened)
	}
}
