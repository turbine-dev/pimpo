package remote

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeNode struct {
	mu     sync.Mutex
	state  string
	auth   string
	logins int
	funnel error
	enable string
	ln     net.Listener
	closed bool
}

func (f *fakeNode) Start() error { return nil }
func (f *fakeNode) State(context.Context) (string, string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, f.auth, "pimpo-dener.tail1234.ts.net.", nil
}
func (f *fakeNode) Login(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logins++
	f.auth = "https://login.tailscale.com/a/abc123"
	return nil
}
func (f *fakeNode) Funnel(context.Context) (bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.enable == "", f.enable, nil
}
func (f *fakeNode) ListenFunnel() (net.Listener, error) {
	if f.funnel != nil {
		return nil, f.funnel
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	f.ln = ln
	return ln, err
}
func (f *fakeNode) Close() error { f.closed = true; return nil }
func (f *fakeNode) login() {
	f.mu.Lock()
	f.state, f.auth = "Running", ""
	f.mu.Unlock()
}

func wait(t *testing.T, r *Remote, state string) Status {
	t.Helper()
	for i := 0; i < 100; i++ {
		if s := r.Status(); s.State == state {
			return s
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("never reached %s: %+v", state, r.Status())
	return Status{}
}

func TestLoginThenPublish(t *testing.T) {
	node := &fakeNode{state: "NeedsLogin"}
	var got string
	r := &Remote{NewNode: func() Node { return node }, OnURL: func(u string) { got = u },
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "pimpo") })}
	if r.Status().State != "off" {
		t.Fatal("should start off")
	}
	r.Start(context.Background())
	s := wait(t, r, "needs_login")
	if s.AuthURL != "https://login.tailscale.com/a/abc123" || node.logins != 1 {
		t.Fatalf("%+v logins %d", s, node.logins)
	}
	node.mu.Lock()
	node.enable = "https://login.tailscale.com/f/funnel?node=abc"
	node.mu.Unlock()
	node.login()
	if s = wait(t, r, "needs_funnel"); s.AuthURL != "https://login.tailscale.com/f/funnel?node=abc" {
		t.Fatalf("%+v", s)
	}
	node.mu.Lock()
	node.enable = ""
	node.mu.Unlock()
	s = wait(t, r, "running")
	if s.URL != "https://pimpo-dener.tail1234.ts.net" || got != s.URL {
		t.Fatalf("%+v %q", s, got)
	}
	resp, err := http.Get("http://" + node.ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "pimpo" {
		t.Fatalf("served %q", b)
	}
	r.Stop()
	if !node.closed || r.Status().State != "off" {
		t.Fatal("stop")
	}
}

func TestFunnelRefusalIsExplained(t *testing.T) {
	node := &fakeNode{state: "Running", funnel: errors.New(`Funnel not available; "funnel" node attribute not set`)}
	r := &Remote{NewNode: func() Node { return node }, Handler: http.NotFoundHandler()}
	r.Start(context.Background())
	s := wait(t, r, "error")
	if !strings.Contains(s.Error, "admin console") {
		t.Fatalf("%+v", s)
	}
}

func TestLANUsesOnlyHomeAddresses(t *testing.T) {
	l := &LAN{Port: 0, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok") }),
		Addr: func() (net.IP, error) { return net.ParseIP("8.8.8.8"), nil }}
	if _, err := l.Start(); err == nil {
		t.Fatal("listened on a public address")
	}
	// 127.0.0.1 is not private for net.IP, so the test serves a loopback
	// listener standing in for a home address.
	l.Addr = func() (net.IP, error) { return net.ParseIP("127.0.0.1"), nil }
	if _, err := l.Start(); err == nil {
		t.Fatal("loopback is not a home network")
	}
	if ip, err := HomeAddress(); err == nil && !ip.IsPrivate() {
		t.Fatalf("home address %s is not private", ip)
	}
}

func TestLANServesOnTheHomeAddress(t *testing.T) {
	if _, err := HomeAddress(); err != nil {
		t.Skip("no home network here")
	}
	l := &LAN{Port: 0, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok") })}
	url, err := l.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer l.Stop()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(resp.Body); string(b) != "ok" || !strings.HasPrefix(url, "http://") {
		t.Fatalf("%s %q", url, b)
	}
}

func TestHTTPSRefusalNamesHTTPS(t *testing.T) {
	msg := friendly(errors.New("Funnel not available; HTTPS must be enabled. See https://tailscale.com/s/https."))
	if !strings.Contains(msg, "HTTPS certificates are off") {
		t.Fatal(msg)
	}
}

// With the main server on every interface, the home address is only
// reported, not listened on again.
func TestLANAlreadyServed(t *testing.T) {
	l := &LAN{Port: 8123, Served: true, Addr: func() (net.IP, error) { return net.ParseIP("192.168.1.20"), nil }}
	url, err := l.Start()
	if err != nil || url != "http://192.168.1.20:8123" || l.URL() != url {
		t.Fatalf("%q %v", url, err)
	}
	l.Stop()
	if l.URL() != "" {
		t.Fatal("still reported after stop")
	}
}
