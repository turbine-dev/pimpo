package app

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/remote"
)

type tsFake struct {
	mu    sync.Mutex
	state string
	auth  string
}

func (f *tsFake) Start() error { return nil }
func (f *tsFake) State(context.Context) (string, string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, f.auth, "pimpo.tail9.ts.net.", nil
}
func (f *tsFake) Login(context.Context) error {
	f.mu.Lock()
	f.auth = "https://login.tailscale.com/a/xyz"
	f.mu.Unlock()
	return nil
}
func (f *tsFake) Funnel(context.Context) (bool, string, error) { return true, "", nil }
func (f *tsFake) ListenFunnel() (net.Listener, error)          { return net.Listen("tcp", "127.0.0.1:0") }
func (f *tsFake) Close() error                                 { return nil }

func TestPhoneLinksWithTailscaleAndHome(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	node := &tsFake{state: "NeedsLogin"}
	ta.AttachRemote(t.TempDir(), func() remote.Node { return node })
	ta.LAN.Port = 0
	if _, err := remote.HomeAddress(); err != nil {
		t.Skip("no home network here")
	}

	_, st := ta.do(t, "GET", "/api/remote", nil)
	if st["tailscale"].(map[string]any)["state"] != "off" {
		t.Fatalf("%v", st)
	}
	ta.do(t, "POST", "/api/remote/tailscale/on", nil)
	var auth string
	for i := 0; i < 50 && auth == ""; i++ {
		time.Sleep(30 * time.Millisecond)
		_, st = ta.do(t, "GET", "/api/remote", nil)
		auth, _ = st["tailscale"].(map[string]any)["auth_url"].(string)
	}
	if auth != "https://login.tailscale.com/a/xyz" {
		t.Fatalf("no login link: %v", st)
	}
	node.mu.Lock()
	node.state, node.auth = "Running", ""
	node.mu.Unlock()
	for i := 0; i < 50; i++ {
		time.Sleep(30 * time.Millisecond)
		if base, _ := ta.Events.Get(ctx, "public_url"); base == "https://pimpo.tail9.ts.net" {
			break
		}
	}
	if base, _ := ta.Events.Get(ctx, "public_url"); base != "https://pimpo.tail9.ts.net" {
		t.Fatalf("public url %q", base)
	}

	_, st = ta.do(t, "POST", "/api/remote/lan/on", nil)
	home, _ := st["lan"].(map[string]any)["url"].(string)
	if !strings.HasPrefix(home, "http://") {
		t.Fatalf("lan %v", st)
	}
	_, out := ta.do(t, "POST", "/api/pairing", map[string]string{"base": "https://pimpo.tail9.ts.net", "device": "Celular"})
	link := out["link"].(string)
	if !strings.HasPrefix(link, "https://pimpo.tail9.ts.net/auth?token=") || !strings.Contains(link, "#home=http") || !strings.HasPrefix(out["home"].(string), home+"/auth?token=") {
		t.Fatalf("links %v", out)
	}
	if on, _ := ta.Events.Get(ctx, "remote.lan"); on != "on" {
		t.Fatal("the home network choice was not remembered")
	}
	ta.do(t, "POST", "/api/remote/tailscale/off", nil)
	if base, _ := ta.Events.Get(ctx, "public_url"); base != "" {
		t.Fatalf("the public link stayed after turning Tailscale off: %q", base)
	}
	ta.do(t, "POST", "/api/remote/lan/off", nil)
}
