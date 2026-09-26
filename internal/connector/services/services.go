// Package services holds the connectors people set up from the catalog in
// Connections: each one declares the fields it needs, the capabilities it
// offers with their risk, and how to reach the service.
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/denerFernandes/pimpo/internal/capability"
	"github.com/denerFernandes/pimpo/internal/connector"
)

type Field struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	Optional    bool   `json:"optional,omitempty"`
}

// Config reads a field of a connector for whoever the call acts for.
type Config func(ctx context.Context, field string) (string, error)

type Kind struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Help        string            `json:"help"`
	Fields      []Field           `json:"fields"`
	Specs       []capability.Spec `json:"-"`
	// Call serves one capability with the connector's configuration.
	Call func(ctx context.Context, cfg Config, name, scope string, args any) (any, error) `json:"-"`
	// Probe checks the configuration against the real service without
	// changing anything; nil when there is nothing to check.
	Probe func(ctx context.Context, cfg Config) error `json:"-"`
}

var kinds = map[string]Kind{}

func register(k Kind) {
	kinds[k.ID] = k
	for _, s := range k.Specs {
		capability.Register(s)
	}
}

// All lists the catalog in a stable order.
func All() []Kind {
	out := make([]Kind, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

func Get(id string) (Kind, bool) { k, ok := kinds[id]; return k, ok }

// Connector adapts a kind to the router.
func (k Kind) Connector(cfg func(kind string) Config) connector.Connector {
	return adapter{k, cfg}
}

type adapter struct {
	k   Kind
	cfg func(kind string) Config
}

func (a adapter) Capabilities() []string {
	var out []string
	for _, s := range a.k.Specs {
		out = append(out, s.Name)
	}
	return out
}

func (a adapter) Call(ctx context.Context, name, scope string, args any) (any, error) {
	return a.k.Call(ctx, a.cfg(a.k.ID), name, scope, args)
}

// need reads required fields, with an error that says where to fix it.
func need(ctx context.Context, cfg Config, title string, fields ...string) ([]string, error) {
	out := make([]string, len(fields))
	for i, f := range fields {
		v, err := cfg(ctx, f)
		if err != nil || strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("%s is not set up; open Connections", title)
		}
		out[i] = strings.TrimSpace(v)
	}
	return out, nil
}

// BaseURL lets tests point every connector at a fake server.
var BaseURL = map[string]string{}

func base(id, def string) string {
	if b := BaseURL[id]; b != "" {
		return b
	}
	return def
}

var client = &http.Client{Timeout: 20 * time.Second}

// doJSON sends a request and decodes a JSON answer into out.
func doJSON(ctx context.Context, method, url string, headers map[string]string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return fmt.Errorf("%s answered %s: %s", req.URL.Host, resp.Status, msg)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

var errEmpty = errors.New("text is empty")

func obj(props string, required ...string) string {
	req, _ := json.Marshal(required)
	if len(required) == 0 {
		req = []byte("[]")
	}
	return `{"type":"object","required":` + string(req) + `,"properties":{` + props + `}}`
}
