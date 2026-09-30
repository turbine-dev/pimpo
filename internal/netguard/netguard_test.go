package netguard

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBlocked(t *testing.T) {
	for _, s := range []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "0.0.0.0",
		"100.64.0.1", "100.100.100.100", "::1", "fe80::1", "fc00::1", "::",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", "64:ff9b::a9fe:a9fe", "64:ff9b::7f00:1",
		"2002:7f00:1::", "224.0.0.1", "255.255.255.255", "::127.0.0.1",
	} {
		if !Blocked(net.ParseIP(s)) {
			t.Errorf("%s is not blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "2606:4700::1111", "64:ff9b::808:808", "::ffff:8.8.8.8", "100.128.0.1"} {
		if Blocked(net.ParseIP(s)) {
			t.Errorf("%s is blocked", s)
		}
	}
}

func TestHostIsWhereGoConnects(t *testing.T) {
	cases := map[string]string{
		"https://allowed.com:x@attacker.tld/?d=SECRET": "",
		"https://allowed.com@attacker.tld/":            "",
		"https://Allowed.com:8443/a?b=c":               "allowed.com",
		"http://[::1]:80/":                             "::1",
		"//evil.tld/":                                  "",
		"evil.tld":                                     "",
		"file:///etc/passwd":                           "",
		"https:///nohost":                              "",
	}
	for raw, want := range cases {
		if got := Host(raw); got != want {
			t.Errorf("Host(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestClientRefusesPrivateAndOtherHosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://localhost:1/x", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := Client(nil, "", false).Get(srv.URL); err == nil || !strings.Contains(err.Error(), "private address") {
		t.Fatalf("reached a loopback address: %v", err)
	}
	if _, err := Client(nil, "127.0.0.1", true).Get(srv.URL); err == nil || !strings.Contains(err.Error(), "outside the allowed host") {
		t.Fatalf("followed a redirect to another host: %v", err)
	}
}
