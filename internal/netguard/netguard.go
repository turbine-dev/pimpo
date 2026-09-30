// Package netguard keeps requests made for untrusted input on the host
// they were allowed to reach and off this computer and its networks.
package netguard

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ParseURL accepts only an absolute http(s) URL with a host and no user
// info, so the host a check sees is the host Go connects to.
func ParseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return nil, fmt.Errorf("%q is not an http(s) URL", raw)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%q has a user name in it; give the address without one", raw)
	}
	return u, nil
}

// Host is the lower-case host of a URL ParseURL accepts, or "".
func Host(raw string) string {
	u, err := ParseURL(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// SameHost says whether a URL is on the allowed host, compared exactly.
func SameHost(u *url.URL, host string) bool {
	return host != "" && u.User == nil && strings.EqualFold(u.Hostname(), host)
}

var blockedNets = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{
		"0.0.0.0/8",      // this network
		"100.64.0.0/10",  // carrier-grade NAT, Tailscale
		"192.0.0.0/24",   // protocol assignments
		"198.18.0.0/15",  // benchmarking
		"240.0.0.0/4",    // reserved, broadcast
		"64:ff9b:1::/48", // local-use NAT64
		"fec0::/10",      // old site-local
		"2001:db8::/32",  // documentation
		"::/96",          // old IPv4-compatible
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

var (
	nat64  = netip.MustParsePrefix("64:ff9b::/96")
	sixTo4 = netip.MustParsePrefix("2002::/16")
)

// Blocked says whether an address is this computer, a private or local
// network, or one that hides such an address inside an IPv6 one.
func Blocked(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	return blockedAddr(a.Unmap())
}

func blockedAddr(a netip.Addr) bool {
	if a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsInterfaceLocalMulticast() || a.IsMulticast() || a.IsUnspecified() {
		return true
	}
	for _, p := range blockedNets {
		if p.Contains(a) {
			return true
		}
	}
	if a.Is6() {
		b := a.As16()
		// NAT64 carries an IPv4 address in its last four bytes, 6to4 in
		// bytes 2 to 5.
		if nat64.Contains(a) {
			return blockedAddr(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
		}
		if sixTo4.Contains(a) {
			return blockedAddr(netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}))
		}
	}
	return false
}

// Control refuses, at dial time, to connect to a blocked address; it sees
// the address DNS gave, so a name that points inside is refused too.
func Control(_, address string, _ syscall.RawConn) error {
	host, _, _ := net.SplitHostPort(address)
	if ip := net.ParseIP(host); ip == nil || Blocked(ip) {
		return fmt.Errorf("refusing to connect to private address %s", host)
	}
	return nil
}

// Transport never reaches a blocked address and uses no proxy.
func Transport() *http.Transport {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: Control}
	return &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second}
}

// Client copies base (nil means a 20 second timeout) so that it follows
// redirects only on host (any host when host is ""), never to a URL with
// user info, and, unless allowPrivate, never reaches a blocked address.
func Client(base *http.Client, host string, allowPrivate bool) *http.Client {
	if base == nil {
		base = &http.Client{Timeout: 20 * time.Second}
	}
	c := *base
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.User != nil || (host != "" && !strings.EqualFold(req.URL.Hostname(), host)) {
			return fmt.Errorf("redirect to %s is outside the allowed host", req.URL.Hostname())
		}
		if len(via) > 5 {
			return errors.New("too many redirects")
		}
		return nil
	}
	if !allowPrivate {
		c.Transport = Transport()
	}
	return &c
}
