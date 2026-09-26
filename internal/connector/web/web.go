// Package web lets routines read JSON from hosts their manifest names.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Web struct {
	HTTP *http.Client
	// AllowPrivate permits loopback and private addresses; tests only.
	AllowPrivate bool
}

func (w *Web) Capabilities() []string { return []string{"http.getJSON"} }

func (w *Web) Call(ctx context.Context, _, scope string, args any) (any, error) {
	raw, _ := args.(string)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("%q is not an http(s) URL", raw)
	}
	if !strings.EqualFold(u.Hostname(), scope) {
		return nil, fmt.Errorf("%s is outside the allowed host %s", u.Hostname(), scope)
	}
	client := w.client(scope)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Pimpo/1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s failed: %w", u.Hostname(), errors.Unwrap(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s answered %d", u.Hostname(), resp.StatusCode)
	}
	var v any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&v); err != nil {
		return nil, fmt.Errorf("%s did not return JSON", u.Hostname())
	}
	return v, nil
}

func (w *Web) client(scope string) *http.Client {
	base := w.HTTP
	if base == nil {
		base = &http.Client{Timeout: 20 * time.Second}
	}
	c := *base
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !strings.EqualFold(req.URL.Hostname(), scope) {
			return fmt.Errorf("redirect to %s is outside the allowed host", req.URL.Hostname())
		}
		if len(via) > 5 {
			return errors.New("too many redirects")
		}
		return nil
	}
	if !w.AllowPrivate {
		dialer := &net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscallRawConn) error {
			host, _, _ := net.SplitHostPort(address)
			ip := net.ParseIP(host)
			if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
				return fmt.Errorf("refusing to connect to private address %s", host)
			}
			return nil
		}}
		c.Transport = &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second}
	}
	return &c
}
