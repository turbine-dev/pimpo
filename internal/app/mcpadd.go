package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/denerFernandes/zodim/internal/capability"
	"github.com/denerFernandes/zodim/internal/connector/external"
	"github.com/denerFernandes/zodim/internal/connector/services"
	"github.com/denerFernandes/zodim/internal/people"
	"github.com/denerFernandes/zodim/internal/server"
)

// Adding third-party MCP servers: found in the official registry or typed
// by hand. Zodim connects, lists the tools, and the owner sets what each
// one may do before anything is installed.

// registryBase lets tests point the search at a fake registry.
var registryBase = external.DefaultRegistry

func (a *App) mcpRoutes() {
	a.Server.Handle("GET /api/connectors/registry", a.searchRegistry)
	a.Server.Handle("POST /api/connectors/probe", a.probeServer)
	a.Server.Handle("POST /api/connectors/add", a.addServer)
	a.Server.Handle("DELETE /api/connectors/{name}", a.removeServer)
}

// The registry can take half a minute to answer, so searches are kept
// for a while.
type registryPage struct {
	at      time.Time
	servers []external.Listing
	next    string
}

var (
	registryMu    sync.Mutex
	registryCache = map[string]registryPage{}
)

func (a *App) searchRegistry(w http.ResponseWriter, r *http.Request) {
	q, cursor := strings.TrimSpace(r.URL.Query().Get("q")), r.URL.Query().Get("cursor")
	key := registryBase + "\x00" + strings.ToLower(q) + "\x00" + cursor
	registryMu.Lock()
	page, ok := registryCache[key]
	registryMu.Unlock()
	if !ok || time.Since(page.at) > 10*time.Minute {
		list, next, err := external.Search(r.Context(), nil, registryBase, q, cursor)
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 502, Msg: err.Error()})
			return
		}
		page = registryPage{time.Now(), list, next}
		registryMu.Lock()
		if len(registryCache) > 200 {
			clear(registryCache)
		}
		registryCache[key] = page
		registryMu.Unlock()
	}
	server.WriteJSON(w, 200, map[string]any{"servers": page.servers, "next": page.next})
}

type serverSource struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	URL     string            `json:"url"`
	Env     map[string]string `json:"env"`
	Headers map[string]string `json:"headers"`
	// ArgValues fill the {placeholders} a registry listing leaves in Args.
	ArgValues map[string]string `json:"arg_values"`
}

var placeholder = regexp.MustCompile(`\{([^{}]+)\}`)

func (s serverSource) endpoint(dir string) (external.Endpoint, error) {
	e := external.Endpoint{Name: s.Name, Dir: dir, Command: strings.TrimSpace(s.Command), URL: strings.TrimSpace(s.URL), Env: map[string]string{}, Headers: map[string]string{}}
	if (e.Command == "") == (e.URL == "") {
		return e, fmt.Errorf("give a command or a url")
	}
	if e.URL != "" && !strings.HasPrefix(e.URL, "https://") {
		return e, fmt.Errorf("a remote server needs an https address")
	}
	for _, arg := range s.Args {
		var missing string
		arg = placeholder.ReplaceAllStringFunc(arg, func(m string) string {
			v := strings.TrimSpace(s.ArgValues[m[1:len(m)-1]])
			if v == "" {
				missing = m[1 : len(m)-1]
			}
			return v
		})
		if missing != "" {
			return e, fmt.Errorf("fill in %s", missing)
		}
		e.Args = append(e.Args, arg)
	}
	for k, v := range s.Env {
		if v = strings.TrimSpace(v); v != "" {
			e.Env[k] = v
		}
	}
	for k, v := range s.Headers {
		if v = strings.TrimSpace(v); v != "" && !placeholder.MatchString(v) {
			e.Headers[k] = v
		}
	}
	return e, nil
}

func ownerOnly(w http.ResponseWriter, r *http.Request) bool {
	if people.From(r.Context()) != people.OwnerID {
		server.WriteError(w, server.StatusError{Status: 403, Msg: "only the owner adds connectors"})
		return false
	}
	return true
}

type probedTool struct {
	Tool        string `json:"tool"`
	Capability  string `json:"capability"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description"`
	Risk        string `json:"risk"`
}

var nonMethod = regexp.MustCompile(`[^a-z0-9_]+`)

// methodName makes a capability method from a tool name, unique in seen.
func methodName(tool string, seen map[string]bool) string {
	m := strings.Trim(nonMethod.ReplaceAllString(strings.ToLower(tool), "_"), "_")
	if m == "" {
		m = "tool"
	}
	base := m
	for i := 2; seen[m]; i++ {
		m = fmt.Sprintf("%s_%d", base, i)
	}
	seen[m] = true
	return m
}

func probed(name string, tools []external.Tool) []probedTool {
	seen := map[string]bool{}
	out := []probedTool{}
	for _, t := range tools {
		out = append(out, probedTool{Tool: t.Name, Capability: name + "." + methodName(t.Name, seen), Title: t.Title, Description: t.Description, Risk: t.SuggestedRisk()})
	}
	return out
}

func (a *App) probeServer(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var src serverSource
	if err := server.Decode(r, &src); err != nil {
		server.WriteError(w, err)
		return
	}
	if src.Name == "" {
		src.Name = "server"
	}
	e, err := src.endpoint(a.Home)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	tools, err := external.Probe(r.Context(), e)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 422, Msg: err.Error()})
		return
	}
	server.WriteJSON(w, 200, map[string]any{"tools": probed(src.Name, tools)})
}

var connectorName = regexp.MustCompile(`^[a-z][a-z0-9]{1,30}$`)

func (a *App) addServer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ownerOnly(w, r) {
		return
	}
	if a.Home == "" {
		server.WriteError(w, server.StatusError{Status: 503, Msg: "no connectors folder"})
		return
	}
	var req struct {
		serverSource
		Description string            `json:"description"`
		Source      string            `json:"source"`
		Tools       map[string]string `json:"tools"` // tool name → risk
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	bad := func(msg string) { server.WriteError(w, server.StatusError{Status: 400, Msg: msg}) }
	name := req.Name
	if !connectorName.MatchString(name) {
		bad("the name must be 2 to 31 lowercase letters and digits, starting with a letter")
		return
	}
	if a.nameTaken(name) {
		bad(name + " is already in use; choose another name")
		return
	}
	if len(req.Tools) == 0 {
		bad("choose at least one tool")
		return
	}
	e, err := req.endpoint(a.Home)
	if err != nil {
		bad(err.Error())
		return
	}
	// Probe again: what gets installed is what the server offers now.
	tools, err := external.Probe(ctx, e)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 422, Msg: err.Error()})
		return
	}
	m := external.Manifest{Name: name, Description: strings.TrimSpace(req.Description), Command: e.Command, Args: e.Args, URL: e.URL, Imported: true, Source: req.Source}
	seen := map[string]bool{}
	for _, t := range tools {
		method := methodName(t.Name, seen)
		risk, ok := req.Tools[t.Name]
		if !ok {
			continue
		}
		if _, known := map[string]bool{"read": true, "notify": true, "reversible": true, "irreversible": true}[risk]; !known {
			bad("risk must be read, notify, reversible or irreversible")
			return
		}
		desc := strings.Join(strings.Fields(t.Description), " ")
		if desc == "" {
			desc = "the result of " + t.Name
		}
		m.Capabilities = append(m.Capabilities, external.Capability{Name: name + "." + method, Tool: t.Name, Risk: risk,
			Signature: method + "(" + signature(t.InputSchema) + ")", Returns: desc, Schema: t.InputSchema})
		delete(req.Tools, t.Name)
	}
	if len(req.Tools) > 0 {
		for missing := range req.Tools {
			bad("the server no longer offers " + missing)
			return
		}
	}
	for k := range e.Env {
		m.Env = append(m.Env, k)
	}
	for k := range e.Headers {
		m.Headers = append(m.Headers, k)
	}
	dir := filepath.Join(a.Home, "connectors", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		server.WriteError(w, err)
		return
	}
	b, _ := json.MarshalIndent(m, "", " ")
	if err := os.WriteFile(filepath.Join(dir, "connector.json"), b, 0o600); err != nil {
		server.WriteError(w, err)
		return
	}
	if _, err := external.Load(dir); err != nil {
		os.RemoveAll(dir)
		bad(err.Error())
		return
	}
	for k, v := range e.Env {
		a.Vault.Set(ctx, "connector."+name+"."+k, v)
	}
	for k, v := range e.Headers {
		a.Vault.Set(ctx, "connector."+name+"."+k, v)
	}
	a.Events.Append(ctx, "connector.installed", actor(ctx), map[string]any{"name": name, "source": req.Source, "tools": len(m.Capabilities)})
	a.reloadConnectors(w, r)
}

// nameTaken reports whether a connector or a built-in family of
// capabilities already uses name.
func (a *App) nameTaken(name string) bool {
	if _, builtin := services.Get(name); builtin || a.externalConnector(name) != nil {
		return true
	}
	for c := range capability.Catalog {
		if strings.HasPrefix(c, name+".") {
			return true
		}
	}
	return false
}

// signature renders a tool's parameters for the model, like {city, days?}.
func signature(schema json.RawMessage) string {
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if json.Unmarshal(schema, &s) != nil || len(s.Properties) == 0 {
		return ""
	}
	req := map[string]bool{}
	for _, r := range s.Required {
		req[r] = true
	}
	var names []string
	for k := range s.Properties {
		if !req[k] {
			k += "?"
		}
		names = append(names, k)
	}
	sort.Strings(names)
	return "{" + strings.Join(names, ", ") + "}"
}

func (a *App) removeServer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ownerOnly(w, r) {
		return
	}
	name := r.PathValue("name")
	c := a.externalConnector(name)
	if c == nil || !connectorName.MatchString(name) {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such connector"})
		return
	}
	c.Close()
	c.Unregister()
	a.Router.Remove(c.Capabilities()...)
	for _, v := range append(append([]string{}, c.Env...), c.Headers...) {
		a.Vault.Delete(ctx, "connector."+name+"."+v)
	}
	if err := os.RemoveAll(filepath.Join(a.Home, "connectors", name)); err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(ctx, "connector.removed", actor(ctx), map[string]string{"name": name})
	a.reloadConnectors(w, r)
}
