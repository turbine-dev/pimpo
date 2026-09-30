package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/policy"
	"github.com/turbine-dev/pimpo/internal/protect"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/protection"
)

// Guard lets agents that are not Pimpo, such as OpenClaw and Hermes, ask
// Pimpo's rules and the shared protection list before they run a tool.
// Their tools are sorted into a few guard.* capabilities, so the owner
// writes rules for them like for anything else. Pimpo cannot undo what
// another agent does, so writes and tools it cannot sort ask first, unless
// a rule allows them (writes inside folders the owner named, for example).

func init() {
	for _, s := range []capability.Spec{
		{Name: "guard.exec", Risk: capability.Irreversible, Signature: "another agent runs a command or code", Returns: ""},
		{Name: "guard.send", Risk: capability.Irreversible, Signature: "another agent sends a message, email or post", Returns: ""},
		{Name: "guard.write", Risk: capability.Irreversible, Signature: "another agent writes or edits files", Returns: ""},
		{Name: "guard.delete", Risk: capability.Irreversible, Signature: "another agent deletes something", Returns: ""},
		{Name: "guard.web", Risk: capability.Read, Signature: "another agent reads a web page or API", Returns: "", Scoped: true},
		{Name: "guard.read", Risk: capability.Read, Signature: "another agent reads files or searches", Returns: ""},
		{Name: "guard.other", Risk: capability.Irreversible, Signature: "another agent uses some other tool", Returns: ""},
	} {
		capability.Register(s)
	}
}

var guardKinds = []struct {
	cap string
	re  *regexp.Regexp
}{
	{"guard.delete", regexp.MustCompile(`(?i)(delete|remove|rm\b|trash|unlink|drop)`)},
	{"guard.exec", regexp.MustCompile(`(?i)(exec|bash|shell|terminal|command|run_?code|python|process|spawn|code_mode)`)},
	{"guard.send", regexp.MustCompile(`(?i)(send|message|email|mail|post|tweet|reply|notify|sms|whatsapp|telegram|slack|discord)`)},
	{"guard.web", regexp.MustCompile(`(?i)(web|fetch|http|browser|navigate|url|crawl|scrape|search_web)`)},
	{"guard.write", regexp.MustCompile(`(?i)(write|edit|patch|create|update|save|apply|move|rename)`)},
	{"guard.read", regexp.MustCompile(`(?i)(read|view|list|search|grep|find|get|cat|glob|memory_search|recall)`)},
}

var destructive = regexp.MustCompile(`(?i)(\brm\s+-\w*[rf]|\brmdir\b|\bdel\s+/|\bshred\b|\bmkfs|\bdd\s+if=|git\s+push\s+.*--force|drop\s+(table|database))`)

// guardTool normalizes a tool name before it is sorted, so full-width or
// other compatibility forms read as the plain letters they look like.
func guardTool(tool string) string { return strings.ToLower(norm.NFKC.String(tool)) }

// guardCapability sorts a foreign tool into one of the guard capabilities.
// A name that still has non-ASCII letters after normalizing may be a
// look-alike ("dеlete" with a Cyrillic е), so it is not sorted at all.
func guardCapability(tool string, params map[string]any) (string, string) {
	tool = guardTool(tool)
	c := "guard.other"
	ascii := true
	for _, r := range tool {
		ascii = ascii && r < utf8.RuneSelf
	}
	for _, k := range guardKinds {
		if ascii && k.re.MatchString(tool) {
			c = k.cap
			break
		}
	}
	if c == "guard.exec" {
		cmd, _ := params["command"].(string)
		if cmd == "" {
			cmd, _ = params["cmd"].(string)
		}
		if destructive.MatchString(cmd) {
			c = "guard.delete"
		}
	}
	scope := ""
	for _, key := range []string{"url", "href", "uri", "link"} {
		if v, ok := params[key].(string); ok {
			if u, err := url.Parse(v); err == nil && u.Hostname() != "" {
				scope = strings.ToLower(u.Hostname())
				if c == "guard.other" || c == "guard.read" {
					c = "guard.web"
				}
			}
		}
	}
	return c, scope
}

// pathKeys are the parameters that name the files a tool touches.
var pathKeys = []string{"path", "file_path", "filepath", "file", "filename", "target", "target_file", "destination", "dest", "source", "old_path", "new_path", "directory", "dir", "cwd", "paths"}

// guardPaths are the files a tool call names, absolute and cleaned. A
// relative path means nothing here (Pimpo does not know the agent's
// folder), so any relative path makes the list empty and no folder rule
// matches.
func guardPaths(params map[string]any) []string {
	var out []string
	add := func(v any) bool {
		p, ok := v.(string)
		if !ok || !filepath.IsAbs(p) {
			return false
		}
		out = append(out, filepath.Clean(p))
		return true
	}
	for _, k := range pathKeys {
		v, ok := params[k]
		if !ok {
			continue
		}
		list, isList := v.([]any)
		if !isList {
			list = []any{v}
		}
		for _, item := range list {
			if !add(item) {
				return nil
			}
		}
	}
	return out
}

const guardHostsKey = "guard.hosts"

// firstGuardHost reports whether no agent has reached host through the
// Guard before and the owner never answered about it, and remembers it.
func (a *App) firstGuardHost(ctx context.Context, host string) bool {
	if _, known := a.Rules.KnownHost(ctx, host); known {
		return false
	}
	raw, _ := a.Events.Get(ctx, guardHostsKey)
	seen := map[string]bool{}
	json.Unmarshal([]byte(raw), &seen)
	if seen[host] {
		return false
	}
	seen[host] = true
	b, _ := json.Marshal(seen)
	a.Events.Put(ctx, guardHostsKey, string(b))
	return true
}

func (a *App) guardRoutes() {
	a.Server.Handle("POST /api/guard/check", a.guardCheck)
	a.Server.Handle("GET /api/protection", a.protectionStatus)
	a.Server.Handle("POST /api/protection/{id}/ignore", func(w http.ResponseWriter, r *http.Request) {
		respond(w)(map[string]string{"ignored": r.PathValue("id")}, a.Rules.IgnoreProtection(r.Context(), r.PathValue("id"), "human:owner"))
	})
}

func (a *App) initProtection(ctx context.Context) {
	a.Protect = &protect.Guard{Keys: protection.Keys, Starter: protection.List, URL: a.Settings(ctx).ProtectionURL}
	a.Protect.Init()
	a.Rules.Protect = a.Protect
}

// refreshProtection downloads the community list now and then daily.
func (a *App) refreshProtection(ctx context.Context) {
	for {
		if a.Settings(ctx).ProtectionNetwork {
			a.Protect.URL = a.Settings(ctx).ProtectionURL
			if err := a.Protect.Refresh(ctx); err == nil {
				v, n, _, _ := a.Protect.Status()
				a.Events.Append(ctx, "protect.updated", "system", map[string]int{"version": v, "entries": n})
			}
		}
		select {
		case <-time.After(24 * time.Hour):
		case <-ctx.Done():
			return
		}
	}
}

func (a *App) protectionStatus(w http.ResponseWriter, r *http.Request) {
	v, n, updated, fetched := a.Protect.Status()
	blocked, _ := a.Events.List(r.Context(), event.Query{Types: []string{host.ActionEvent}, Search: `"rule":"protect:`})
	server.WriteJSON(w, 200, map[string]any{"version": v, "entries": n, "updated": updated, "fetched": fetched, "enabled": a.Settings(r.Context()).ProtectionNetwork, "blocked": len(blocked)})
}

type guardRequest struct {
	Agent  string         `json:"agent"`
	Tool   string         `json:"tool"`
	Params map[string]any `json:"params"`
	// Session identifies the other agent's run, so "allow for the rest of
	// this run" works across its calls.
	Session string `json:"session"`
}

type guardAnswer struct {
	// Decision is allow, ask or block.
	Decision   string `json:"decision"`
	Reason     string `json:"reason,omitempty"`
	Capability string `json:"capability"`
	Rule       string `json:"rule,omitempty"`
}

// guardCheck decides one tool call of another agent and records it in the
// receipts. An "ask" answer is for the caller to put to the person with
// its own approval prompt.
func (a *App) guardCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req guardRequest
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	agent := strings.ToLower(regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(req.Agent, ""))
	if agent == "" || req.Tool == "" {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "agent and tool are required"})
		return
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	c, scope := guardCapability(req.Tool, req.Params)
	source := "guard:" + agent
	if req.Session != "" {
		source += "#" + req.Session
	}
	args := map[string]any{"tool": req.Tool, "params": req.Params}
	act := policy.Action{Capability: c, Scope: scope, Args: args, Risk: capability.Catalog[c].Risk, Source: source, Person: "owner", Role: "owner", Paths: guardPaths(req.Params)}
	d := a.Policy.Decide(ctx, act)
	// A host never reached before asks once: a new host is where data
	// would leave to.
	if c == "guard.web" && scope != "" && (d.Verdict == policy.Allow || d.Verdict == policy.Reversible) && d.Rule == "" && a.firstGuardHost(ctx, scope) {
		d = policy.Decision{Verdict: policy.Ask, Reason: "first time reaching " + scope}
	}
	ans := guardAnswer{Capability: c, Reason: d.Reason, Rule: d.Rule}
	switch d.Verdict {
	case policy.Block:
		ans.Decision = "block"
	case policy.Ask:
		ans.Decision = "ask"
	default:
		ans.Decision = "allow"
	}
	rec := host.ActionRecord{Source: source, Capability: c, Scope: scope, Risk: act.Risk.String(), Args: args, Verdict: d.Verdict, Reason: d.Reason, Rule: d.Rule}
	if ans.Decision == "block" {
		rec.Error = d.Reason
	}
	a.Events.Append(ctx, host.ActionEvent, source, rec)
	server.WriteJSON(w, 200, ans)
}
