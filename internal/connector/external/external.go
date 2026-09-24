// Package external runs connectors written by anyone, in any language, as
// separate processes speaking MCP over stdio. A connector ships a
// connector.json that declares every capability with its risk; Vigia only
// exposes what is declared, checks that the process offers exactly that,
// and gives it no secrets but the ones it names.
package external

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/denerFernandes/vigia/internal/capability"
)

type Manifest struct {
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	Command      string       `json:"command"`
	Args         []string     `json:"args,omitempty"`
	Env          []string     `json:"env,omitempty"`
	Capabilities []Capability `json:"capabilities"`
	Contract     []Case       `json:"contract"`
	// Dir is where the manifest was found; the command runs there.
	Dir string `json:"-"`
}

type Capability struct {
	Name      string          `json:"name"`
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
	if man.Command == "" {
		return man, errors.New("connector.json needs a command")
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
			return man, fmt.Errorf("capability %q is already provided by Vigia", c.Name)
		}
		if _, ok := risks[c.Risk]; !ok {
			return man, fmt.Errorf("capability %q: risk must be read, notify, reversible or irreversible", c.Name)
		}
		if c.Signature == "" || c.Returns == "" {
			return man, fmt.Errorf("capability %q needs a signature and what it returns", c.Name)
		}
		if risks[c.Risk] == capability.Read && !covered[c.Name] {
			return man, fmt.Errorf("capability %q has no contract test", c.Name)
		}
	}
	for _, e := range man.Env {
		if !regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`).MatchString(e) {
			return man, fmt.Errorf("env %q must be an upper-case variable name", e)
		}
	}
	return man, nil
}

var risks = map[string]capability.Risk{"read": capability.Read, "notify": capability.Notify, "reversible": capability.Reversible, "irreversible": capability.Irreversible}

// external remembers capabilities registered from connectors, so reloading
// one does not look like a clash with Vigia's own.
var external = map[string]bool{}

// Register adds the manifest's capabilities to the catalog.
func (m Manifest) Register() {
	for _, c := range m.Capabilities {
		external[c.Name] = true
		capability.Register(capability.Spec{Name: c.Name, Risk: risks[c.Risk], Signature: c.Signature, Returns: c.Returns, Schema: string(c.Schema)})
	}
}

// Secrets supplies the values of the env vars a connector declares.
type Secrets func(ctx context.Context, name string) (string, error)

// Connector is a running (or startable) external connector.
type Connector struct {
	Manifest
	Secrets Secrets
	Timeout time.Duration

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  *bufio.Scanner
	nextID int
}

func (c *Connector) Capabilities() []string {
	var out []string
	for _, cap := range c.Manifest.Capabilities {
		out = append(out, cap.Name)
	}
	return out
}

func tool(capName string) string { return strings.Replace(capName, ".", "_", 1) }

type rpcResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// start launches the process with a clean environment and checks it
// offers exactly the declared tools.
func (c *Connector) start(ctx context.Context) error {
	cmd := exec.Command(c.Command, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "LANG=C.UTF-8"}
	for _, e := range c.Env {
		v := ""
		if c.Secrets != nil {
			v, _ = c.Secrets(ctx, e)
		}
		cmd.Env = append(cmd.Env, e+"="+v)
	}
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", c.Name, err)
	}
	c.cmd, c.stdin = cmd, in
	c.lines = bufio.NewScanner(out)
	c.lines.Buffer(make([]byte, 1<<20), 16<<20)
	if _, err := c.rpc(ctx, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "vigia", "version": "1"}}); err != nil {
		c.stop()
		return fmt.Errorf("%s did not initialize: %w", c.Name, err)
	}
	c.notify("notifications/initialized")
	raw, err := c.rpc(ctx, "tools/list", map[string]any{})
	if err != nil {
		c.stop()
		return err
	}
	var list struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	json.Unmarshal(raw, &list)
	var offered, declared []string
	for _, t := range list.Tools {
		offered = append(offered, t.Name)
	}
	for _, cap := range c.Manifest.Capabilities {
		declared = append(declared, tool(cap.Name))
	}
	sort.Strings(offered)
	sort.Strings(declared)
	if strings.Join(offered, ",") != strings.Join(declared, ",") {
		c.stop()
		return fmt.Errorf("%s offers tools %v but declares %v", c.Name, offered, declared)
	}
	return nil
}

func (c *Connector) stop() {
	if c.cmd != nil && c.cmd.Process != nil {
		c.stdin.Close()
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
	c.cmd = nil
}

// Close stops the process.
func (c *Connector) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stop()
}

func (c *Connector) notify(method string) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method})
	c.stdin.Write(append(b, '\n'))
}

func (c *Connector) rpc(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.nextID++
	id := c.nextID
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if _, err := c.stdin.Write(append(b, '\n')); err != nil {
		return nil, err
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	type line struct {
		b   []byte
		err error
	}
	got := make(chan line, 1)
	go func() {
		for c.lines.Scan() {
			var r rpcResponse
			if json.Unmarshal(c.lines.Bytes(), &r) == nil && r.ID == id {
				got <- line{b: append([]byte{}, c.lines.Bytes()...)}
				return
			}
		}
		err := c.lines.Err()
		if err == nil {
			err = io.EOF
		}
		got <- line{err: err}
	}()
	select {
	case l := <-got:
		if l.err != nil {
			return nil, fmt.Errorf("%s stopped: %w", c.Name, l.err)
		}
		var r rpcResponse
		json.Unmarshal(l.b, &r)
		if r.Error != nil {
			return nil, errors.New(r.Error.Message)
		}
		return r.Result, nil
	case <-time.After(timeout):
		c.stop()
		return nil, fmt.Errorf("%s did not answer in %s", c.Name, timeout)
	case <-ctx.Done():
		c.stop()
		return nil, ctx.Err()
	}
}

// Call runs one capability, starting the process if needed.
func (c *Connector) Call(ctx context.Context, name, _ string, args any) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd == nil {
		if err := c.start(ctx); err != nil {
			return nil, err
		}
	}
	raw, err := c.rpc(ctx, "tools/call", map[string]any{"name": tool(name), "arguments": args})
	if err != nil {
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
			if c.cmd == nil {
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
