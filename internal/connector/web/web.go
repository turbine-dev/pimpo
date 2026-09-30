// Package web lets routines read JSON and web pages from hosts their
// manifest names.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/turbine-dev/pimpo/internal/netguard"
)

type Web struct {
	HTTP *http.Client
	// AllowPrivate permits loopback and private addresses; tests only.
	AllowPrivate bool
}

func (w *Web) Capabilities() []string { return []string{"http.getJSON", "web.read"} }

func (w *Web) Call(ctx context.Context, name, scope string, args any) (any, error) {
	if name == "web.read" {
		m, _ := args.(map[string]any)
		u, _ := m["url"].(string)
		return w.read(ctx, u, scope)
	}
	raw, _ := args.(string)
	u, err := checkURL(raw, scope)
	if err != nil {
		return nil, err
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

// checkURL accepts an http(s) URL on the allowed host, without user info.
func checkURL(raw, scope string) (*url.URL, error) {
	u, err := netguard.ParseURL(raw)
	if err != nil {
		return nil, err
	}
	if !netguard.SameHost(u, scope) {
		return nil, fmt.Errorf("%s is outside the allowed host %s", u.Hostname(), scope)
	}
	return u, nil
}

// client follows redirects only on the allowed host and, unless
// AllowPrivate, never reaches this computer or a private network.
func (w *Web) client(scope string) *http.Client {
	return netguard.Client(w.HTTP, scope, w.AllowPrivate)
}
