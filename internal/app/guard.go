package app

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/denerFernandes/vigia/internal/capability"
	"github.com/denerFernandes/vigia/internal/event"
	"github.com/denerFernandes/vigia/internal/host"
	"github.com/denerFernandes/vigia/internal/policy"
	"github.com/denerFernandes/vigia/internal/protect"
	"github.com/denerFernandes/vigia/internal/server"
	"github.com/denerFernandes/vigia/protection"
)

// Guard lets agents that are not Vigia, such as OpenClaw and Hermes, ask
// Vigia's rules and the shared protection list before they run a tool.
// Their tools are sorted into a few guard.* capabilities, so the owner
// writes rules for them like for anything else.

func init() {
	for _, s := range []capability.Spec{
		{Name: "guard.exec", Risk: capability.Irreversible, Signature: "another agent runs a command or code", Returns: ""},
		{Name: "guard.send", Risk: capability.Irreversible, Signature: "another agent sends a message, email or post", Returns: ""},
		{Name: "guard.write", Risk: capability.Reversible, Signature: "another agent writes or edits files", Returns: ""},
		{Name: "guard.delete", Risk: capability.Irreversible, Signature: "another agent deletes something", Returns: ""},
		{Name: "guard.web", Risk: capability.Read, Signature: "another agent reads a web page or API", Returns: "", Scoped: true},
		{Name: "guard.read", Risk: capability.Read, Signature: "another agent reads files or searches", Returns: ""},
		{Name: "guard.other", Risk: capability.Reversible, Signature: "another agent uses some other tool", Returns: ""},
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

// guardCapability sorts a foreign tool into one of the guard capabilities.
func guardCapability(tool string, params map[string]any) (string, string) {
	c := "guard.other"
	for _, k := range guardKinds {
		if k.re.MatchString(tool) {
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
	act := policy.Action{Capability: c, Scope: scope, Args: args, Risk: capability.Catalog[c].Risk, Source: source, Person: "owner", Role: "owner"}
	d := a.Policy.Decide(ctx, act)
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
