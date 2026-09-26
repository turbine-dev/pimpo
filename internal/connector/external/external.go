// Package external runs connectors written by anyone, in any language, as
// separate processes speaking MCP over stdio. A connector ships a
// connector.json that declares every capability with its risk; Pimpo only
// exposes what is declared, checks that the process offers exactly that,
// and gives it no secrets but the ones it names.
package external

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/denerFernandes/pimpo/internal/capability"
)

type Manifest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Command     string   `json:"command,omitempty"`
	Args        []string `json:"args,omitempty"`
	Env         []string `json:"env,omitempty"`
	// URL reaches a remote MCP server instead of starting a command;
	// Headers names the request headers whose values come from the vault.
	URL     string   `json:"url,omitempty"`
	Headers []string `json:"headers,omitempty"`
	// Imported marks a third-party MCP server added from the registry or
	// by hand: the owner set each tool's risk, it has no contract tests,
	// and tools it adds later stay hidden instead of failing.
	Imported     bool         `json:"imported,omitempty"`
	Source       string       `json:"source,omitempty"`
	Capabilities []Capability `json:"capabilities"`
	Contract     []Case       `json:"contract"`
	// Dir is where the manifest was found; the command runs there.
	Dir string `json:"-"`
}

type Capability struct {
	Name string `json:"name"`
	// Tool is the MCP tool name when it is not <connector>_<method>.
	Tool      string          `json:"tool,omitempty"`
	Risk      string          `json:"risk"`
	Signature string          `json:"signature"`
	Returns   string          `json:"returns"`
	Schema    json.RawMessage `json:"schema,omitempty"`
}

// Case is one contract test: calling the capability with these arguments
// must succeed and return these top-level keys (in the object, or in the
// first element of a list).
type Case struct {
	Capability string         `json:"capability"`
	Args       map[string]any `json:"args"`
	Keys       []string       `json:"keys"`
}

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9]{1,30}$`)

// Load reads and validates a connector directory.
func Load(dir string) (Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "connector.json"))
	if err != nil {
		return Manifest{}, err
	}
	var man Manifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return Manifest{}, fmt.Errorf("connector.json: %w", err)
	}
	man.Dir = dir
	if !nameRe.MatchString(man.Name) {
		return man, fmt.Errorf("connector name %q must be lowercase letters and digits", man.Name)
	}
	if (man.Command == "") == (man.URL == "") {
		return man, errors.New("connector.json needs a command or a url")
	}
	if man.URL != "" && !strings.HasPrefix(man.URL, "https://") && !strings.HasPrefix(man.URL, "http://127.0.0.1") && !strings.HasPrefix(man.URL, "http://localhost") {
		return man, errors.New("a remote connector needs an https url")
	}
	if len(man.Capabilities) == 0 {
		return man, errors.New("a connector must declare at least one capability")
	}
	covered := map[string]bool{}
	for _, c := range man.Contract {
		covered[c.Capability] = true
	}
	for _, c := range man.Capabilities {
		if !strings.HasPrefix(c.Name, man.Name+".") || strings.Count(c.Name, ".") != 1 {
			return man, fmt.Errorf("capability %q must be named %s.<method>", c.Name, man.Name)
		}
		if _, builtin := capability.Catalog[c.Name]; builtin && !external[c.Name] {
			return man, fmt.Errorf("capability %q is already provided by Pimpo", c.Name)
		}
		if _, ok := risks[c.Risk]; !ok {
			return man, fmt.Errorf("capability %q: risk must be read, notify, reversible or irreversible", c.Name)
		}
		if c.Signature == "" || c.Returns == "" {
			return man, fmt.Errorf("capability %q needs a signature and what it returns", c.Name)
		}
		if risks[c.Risk] == capability.Read && !covered[c.Name] && !man.Imported {
			return man, fmt.Errorf("capability %q has no contract test", c.Name)
		}
	}
	for _, e := range man.Env {
		if !regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`).MatchString(e) {
			return man, fmt.Errorf("env %q must be an upper-case variable name", e)
		}
	}
	for _, h := range man.Headers {
		if !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`).MatchString(h) {
			return man, fmt.Errorf("header %q is not a header name", h)
		}
	}
	return man, nil
}

var risks = map[string]capability.Risk{"read": capability.Read, "notify": capability.Notify, "reversible": capability.Reversible, "irreversible": capability.Irreversible}

// external remembers capabilities registered from connectors, so reloading
// one does not look like a clash with Pimpo's own.
var external = map[string]bool{}

// Register adds the manifest's capabilities to the catalog.
func (m Manifest) Register() {
	for _, c := range m.Capabilities {
		external[c.Name] = true
		capability.Register(capability.Spec{Name: c.Name, Risk: risks[c.Risk], Signature: c.Signature, Returns: c.Returns, Schema: string(c.Schema)})
	}
}

// Unregister removes the manifest's capabilities from the catalog.
func (m Manifest) Unregister() {
	for _, c := range m.Capabilities {
		delete(external, c.Name)
		capability.Unregister(c.Name)
	}
}

// Secrets supplies the values of the env vars a connector declares.
type Secrets func(ctx context.Context, name string) (string, error)

// Connector is a running (or startable) external connector.
type Connector struct {
	Manifest
	Secrets Secrets
	Timeout time.Duration

	mu   sync.Mutex
	conn transport
}

func (c *Connector) Capabilities() []string {
	var out []string
	for _, cap := range c.Manifest.Capabilities {
		out = append(out, cap.Name)
	}
	return out
}

func tool(capName string) string { return strings.Replace(capName, ".", "_", 1) }

func (c Capability) tool() string {
	if c.Tool != "" {
		return c.Tool
	}
	return tool(c.Name)
}

func (m Manifest) toolFor(capName string) string {
	for _, c := range m.Capabilities {
		if c.Name == capName {
			return c.tool()
		}
	}
	return tool(capName)
}

// values resolves the declared env vars and headers from the vault.
func (c *Connector) values(ctx context.Context, names []string) map[string]string {
	out := map[string]string{}
	for _, n := range names {
		v := ""
		if c.Secrets != nil {
			v, _ = c.Secrets(ctx, n)
		}
		out[n] = v
	}
	return out
}

// start connects, initializes and checks the tools on offer: exactly the
// declared ones, or for imported servers at least those.
func (c *Connector) start(ctx context.Context) error {
	conn, err := dial(ctx, Endpoint{Name: c.Name, Dir: c.Dir, Command: c.Command, Args: c.Args, Env: c.values(ctx, c.Env), URL: c.URL, Headers: c.values(ctx, c.Headers)}, c.Timeout)
	if err != nil {
		return err
	}
	tools, err := listTools(ctx, conn)
	if err != nil {
		conn.close()
		return err
	}
	var offered, declared []string
	have := map[string]bool{}
	for _, t := range tools {
		offered = append(offered, t.Name)
		have[t.Name] = true
	}
	for _, cap := range c.Manifest.Capabilities {
		declared = append(declared, cap.tool())
	}
	sort.Strings(offered)
	sort.Strings(declared)
	if c.Imported {
		for _, d := range declared {
			if !have[d] {
				conn.close()
				return fmt.Errorf("%s no longer offers the tool %s; add it again to review what changed", c.Name, d)
			}
		}
	} else if strings.Join(offered, ",") != strings.Join(declared, ",") {
		conn.close()
		return fmt.Errorf("%s offers tools %v but declares %v", c.Name, offered, declared)
	}
	c.conn = conn
	return nil
}

func (c *Connector) stop() {
	if c.conn != nil {
		c.conn.close()
	}
	c.conn = nil
}

// Close stops the process or ends the remote session.
func (c *Connector) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stop()
}

// Call runs one capability, starting the process if needed.
func (c *Connector) Call(ctx context.Context, name, _ string, args any) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		if err := c.start(ctx); err != nil {
			return nil, err
		}
	}
	raw, err := c.conn.rpc(ctx, "tools/call", map[string]any{"name": c.toolFor(name), "arguments": args})
	if err != nil {
		if errors.Is(err, errBroken) {
			c.stop()
		}
		return nil, err
	}
	var res struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StructuredContent any  `json:"structuredContent"`
		IsError           bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	var text strings.Builder
	for _, p := range res.Content {
		if p.Type == "text" {
			text.WriteString(p.Text)
		}
	}
	if res.IsError {
		return nil, fmt.Errorf("%s: %s", name, text.String())
	}
	if res.StructuredContent != nil {
		return res.StructuredContent, nil
	}
	var v any
	if json.Unmarshal([]byte(text.String()), &v) == nil {
		return v, nil
	}
	return map[string]any{"text": text.String()}, nil
}

// Check runs the connector's contract: the tools it offers match the
// manifest and every case returns the promised keys.
func Check(ctx context.Context, m Manifest, secrets Secrets) []string {
	c := &Connector{Manifest: m, Secrets: secrets, Timeout: 20 * time.Second}
	defer c.Close()
	var problems []string
	for _, cs := range m.Contract {
		declared := false
		for _, cap := range m.Capabilities {
			declared = declared || cap.Name == cs.Capability
		}
		if !declared {
			problems = append(problems, "contract tests undeclared "+cs.Capability)
			continue
		}
		if risks[riskOf(m, cs.Capability)] >= capability.Reversible {
			problems = append(problems, cs.Capability+": contract cases may only call read capabilities")
			continue
		}
		out, err := c.Call(ctx, cs.Capability, "", cs.Args)
		if err != nil {
			problems = append(problems, err.Error())
			if c.conn == nil {
				break
			}
			continue
		}
		obj, _ := out.(map[string]any)
		if list, ok := out.([]any); ok && len(list) > 0 {
			obj, _ = list[0].(map[string]any)
		}
		for _, k := range cs.Keys {
			if _, ok := obj[k]; !ok {
				problems = append(problems, fmt.Sprintf("%s: result has no %q", cs.Capability, k))
			}
		}
	}
	return problems
}

func riskOf(m Manifest, name string) string {
	for _, c := range m.Capabilities {
		if c.Name == name {
			return c.Risk
		}
	}
	return ""
}

// Discover loads every connector under dir; broken ones come back as errors
// and are not loaded.
func Discover(dir string) ([]Manifest, []error) {
	entries, _ := os.ReadDir(dir)
	var out []Manifest
	var errs []error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := Load(filepath.Join(dir, e.Name()))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		out = append(out, m)
	}
	return out, errs
}
