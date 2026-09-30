package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/approval"
	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/connector/mail"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/policy"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/undo"
)

// approver adapts the approval manager to the host.
type approver struct{ m *approval.Manager }

func (a approver) Ask(ctx context.Context, act policy.Action, reason string) (bool, error) {
	ans, err := a.m.Ask(ctx, act, reason)
	return ans == approval.Always, err
}

// remember turns "always" into a lasting permission: a known host for web
// reads, a specific rule for everything else.
func (a *App) remember(ctx context.Context, act policy.Action) {
	a.changeRules(ctx, func() error {
		if (act.Capability == "http.getJSON" || act.Capability == "web.read") && act.Scope != "" {
			return a.Rules.AllowHost(ctx, act.Scope, true, "human:owner")
		}
		rule := policy.AllowAlways(act)
		rules := a.Rules.Rules(ctx)
		for _, r := range rules {
			if r.ID == rule.ID {
				return nil
			}
		}
		return a.Rules.SaveRules(ctx, append(rules, rule), "human:owner")
	})
}

// describeAction is the sentence shown in approval requests.
func describeAction(ctx context.Context, act policy.Action) string {
	who := strings.TrimPrefix(strings.SplitN(act.Source, "#", 2)[0], "routine:")
	return i18n.T(ctx, "action.wants", "who", who, "what", actionPhrase(ctx, act))
}

// actionPhrase says what an action does, starting with the verb.
func actionPhrase(ctx context.Context, act policy.Action) string {
	args, _ := act.Args.(map[string]any)
	str := func(k string) string { s, _ := args[k].(string); return s }
	vars := []any{"to", fmt.Sprint(args["to"]), "subject", str("subject"), "id", str("id"), "label", str("label"),
		"content", str("content"), "text", str("text"), "entity", str("entity"), "host", act.Scope, "capability", act.Capability}
	switch act.Capability {
	case "gmail.send", "gmail.delete", "gmail.trash", "gmail.archive", "gmail.unsubscribe", "gmail.draft", "gmail.label",
		"todoist.add", "todoist.close", "whatsapp.send_to", "ha.call", "http.getJSON":
		return i18n.T(ctx, "action."+act.Capability, vars...)
	case "ha.critical":
		return i18n.T(ctx, "action.ha.call", vars...)
	}
	return i18n.T(ctx, "action.other", vars...)
}

func (a *App) mailOps(ctx context.Context) (undo.Mail, error) {
	addr, _ := a.Events.Get(ctx, "mail.addr")
	user, _ := a.Events.Get(ctx, "mail.user")
	if addr == "" || user == "" {
		return nil, fmt.Errorf("email is not set up; open Connections")
	}
	smtp, _ := a.Events.Get(ctx, "mail.smtp")
	return &mail.Mail{Account: mail.Account{Addr: addr, Username: user, SMTP: smtp, Insecure: a.MailInsecure,
		Password: func(ctx context.Context) (string, error) { return a.secret(ctx, "mail.password") }}}, nil
}

func (a *App) safetyRoutes() {
	s := a.Server
	s.Handle("GET /api/receipts", a.receipts)
	s.Handle("POST /api/actions/{id}/undo", a.undoAction)
	s.Handle("GET /api/approvals", func(w http.ResponseWriter, r *http.Request) { server.WriteJSON(w, 200, a.myApprovals(r.Context())) })
	s.Handle("POST /api/approvals/{id}/{answer}", a.answerApproval)
	s.Handle("GET /api/rules", func(w http.ResponseWriter, r *http.Request) { server.WriteJSON(w, 200, a.Rules.Rules(r.Context())) })
	s.Handle("PUT /api/rules", a.putRules)
	s.Handle("PUT /api/rules/preset", a.putPreset)
	s.Handle("POST /api/rules/compile", a.compileRule)
	s.Handle("POST /api/rules/test", a.testRule)
	s.Handle("GET /api/cost", a.cost)
	s.Handle("GET /api/setup", a.setup)
	s.Handle("POST /api/setup/done", a.setupDone)
}

type receipt struct {
	event.Event
	Action    host.ActionRecord `json:"action"`
	Undoable  bool              `json:"undoable"`
	UndoUntil time.Time         `json:"undo_until,omitzero"`
	Undone    bool              `json:"undone"`
}

func (a *App) receipts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := 200
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 1000 {
		limit = l
	}
	evs, err := a.Events.List(ctx, event.Query{Types: []string{host.ActionEvent}, Newest: true, Limit: limit, Search: r.URL.Query().Get("q")})
	if err != nil {
		server.WriteError(w, err)
		return
	}
	undone := map[int64]bool{}
	ups, _ := a.Events.List(ctx, event.Query{Types: []string{undo.EventUndone}})
	for _, u := range ups {
		var d struct {
			Action int64 `json:"action"`
		}
		u.Decode(&d)
		undone[d.Action] = true
	}
	out := make([]receipt, 0, len(evs))
	for _, e := range evs {
		var rec host.ActionRecord
		e.Decode(&rec)
		if !mine(ctx, rec.Person) {
			continue
		}
		var result map[string]any
		json.Unmarshal(rec.Result, &result)
		ok, until := undo.Plan(rec, result)
		if !until.IsZero() && time.Now().After(until) {
			ok = false
		}
		out = append(out, receipt{Event: e, Action: rec, Undoable: ok && !undone[e.ID], UndoUntil: until, Undone: undone[e.ID]})
	}
	server.WriteJSON(w, 200, out)
}

// myApprovals are the requests this person answers.
func (a *App) myApprovals(ctx context.Context) []approval.Request {
	out := []approval.Request{}
	for _, q := range a.Approvals.Open() {
		if mine(ctx, q.Responsible) {
			out = append(out, q)
		}
	}
	return out
}

func recordIsMine(ctx context.Context, e event.Event) bool {
	var rec host.ActionRecord
	e.Decode(&rec)
	return mine(ctx, rec.Person)
}

func (a *App) undoAction(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "bad id"})
		return
	}
	if ev, err := a.Events.ByID(r.Context(), id); err != nil || ev.Type != host.ActionEvent || !recordIsMine(r.Context(), ev) {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such action"})
		return
	}
	if err := a.Undo.Undo(r.Context(), id, actor(r.Context())); err != nil {
		server.WriteError(w, server.StatusError{Status: 409, Msg: err.Error()})
		return
	}
	server.WriteJSON(w, 200, map[string]string{"state": "undone"})
}

func (a *App) answerApproval(w http.ResponseWriter, r *http.Request) {
	ans := approval.Answer(r.PathValue("answer"))
	if ans != approval.Once && ans != approval.Always && ans != approval.Deny && ans != approval.Run {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "answer must be once, run, always or deny"})
		return
	}
	ctx := r.Context()
	if !slices.ContainsFunc(a.myApprovals(ctx), func(q approval.Request) bool { return q.ID == r.PathValue("id") }) {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such request"})
		return
	}
	// "Always" becomes a rule for the whole house, which is the owner's.
	if ans == approval.Always && people.From(ctx) != people.OwnerID {
		server.WriteError(w, server.StatusError{Status: 403, Msg: "a lasting permission is a rule for the whole house; ask the owner, or answer once"})
		return
	}
	if !a.Approvals.Resolve(ctx, r.PathValue("id"), ans, actor(ctx)) {
		server.WriteError(w, server.StatusError{Status: 410, Msg: "this request is no longer waiting"})
		return
	}
	server.WriteJSON(w, 200, map[string]string{"answer": string(ans)})
}

// putPreset replaces the rules with one of the setup presets, keeping the
// owner's own rules.
func (a *App) putPreset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Preset string `json:"preset"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	preset, ok := policy.Presets()[req.Preset]
	for i := range preset {
		if t, found := i18n.Lookup(i18n.Of(r.Context()), "rule."+preset[i].ID); found {
			preset[i].Text = t
		}
	}
	if !ok {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "preset must be conservative, balanced or liberal"})
		return
	}
	var keep []policy.Rule
	for _, rule := range a.Rules.Rules(r.Context()) {
		if !strings.HasPrefix(rule.ID, "preset-") {
			keep = append(keep, rule)
		}
	}
	rules := append(preset, keep...)
	if err := a.changeRules(r.Context(), func() error {
		if err := a.Rules.SaveRules(r.Context(), rules, "human:owner"); err != nil {
			return err
		}
		return a.Events.Put(r.Context(), "setup.preset", req.Preset)
	}); err != nil {
		server.WriteError(w, err)
		return
	}
	server.WriteJSON(w, 200, rules)
}

func (a *App) putRules(w http.ResponseWriter, r *http.Request) {
	var rules []policy.Rule
	if err := server.Decode(r, &rules); err != nil {
		server.WriteError(w, err)
		return
	}
	if err := a.changeRules(r.Context(), func() error { return a.Rules.SaveRules(r.Context(), rules, "human:owner") }); err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	server.WriteJSON(w, 200, rules)
}

var ruleSchema = json.RawMessage(`{"type":"object","required":["when","then","summary"],"additionalProperties":false,"properties":{
 "summary":{"type":"string","description":"one short sentence restating the rule in the owner's language"},
 "then":{"type":"string","enum":["allow","reversible","ask","block"]},
 "when":{"type":"object","additionalProperties":false,"properties":{
  "capabilities":{"type":"array","items":{"type":"string"}},
  "min_risk":{"type":"string","enum":["","read","notify","reversible","irreversible"]},
  "source":{"type":"string"},
  "args_contain":{"type":"array","items":{"type":"string"}},
  "hosts":{"type":"array","items":{"type":"string"}},
  "paths":{"type":"array","items":{"type":"string"},"description":"full folder paths; matches when every file the action names is inside one"},
  "people":{"type":"array","items":{"type":"string"},"description":"ids of the people the rule is about"},
  "roles":{"type":"array","items":{"type":"string","enum":["owner","member","guest"]}}}}}}`)

// compileRule turns the owner's sentence into a structured rule. Only the
// structured rule is ever enforced, and the owner confirms it first.
func (a *App) compileRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Text string `json:"text"`
	}
	if err := server.Decode(r, &req); err != nil || strings.TrimSpace(req.Text) == "" {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "write the rule in a sentence"})
		return
	}
	if err := a.Budget.Check(ctx); err != nil {
		server.WriteError(w, server.StatusError{Status: 402, Msg: err.Error()})
		return
	}
	var caps strings.Builder
	for _, n := range capability.Names() {
		fmt.Fprintf(&caps, "- %s (%s): %s\n", n, capability.Catalog[n].Risk, capability.Catalog[n].Signature)
	}
	routines, _ := a.Store.Routines(ctx)
	var names []string
	for _, rt := range routines {
		names = append(names, "routine:"+rt.ID+" ("+rt.Body.Name+")")
	}
	house, _ := a.People.List(ctx)
	var who []string
	for _, p := range house {
		who = append(who, fmt.Sprintf("%s (%s, %s)", p.ID, p.Name, p.Role))
	}
	resp, err := a.LLM.Generate(ctx, llm.Request{
		System: "You turn an owner's sentence into one rule for Pimpo's policy engine. Decisions: allow (just do it), reversible (do it but keep it undoable), ask (ask the owner first), block (never). Match as narrowly as the sentence says; leave fields empty to match everything. source is \"routine:<id>\" or \"exploration\"; risk levels are read < notify < reversible < irreversible. When the sentence is about some people of the house, fill people with their ids, or roles for a whole group.",
		Prompt: "Capabilities:\n" + caps.String() + "\nRoutines: " + strings.Join(names, ", ") + "\nPeople of the house: " + strings.Join(who, ", ") + "\n\nThe owner wrote: " + req.Text,
		Schema: ruleSchema, Model: a.Settings(ctx).JudgeModel, MaxCostUSD: 0.2,
	})
	a.Budget.Record(ctx, budgetCost(resp.CostUSD, "rule"))
	if err != nil {
		server.WriteError(w, err)
		return
	}
	var out struct {
		Summary string         `json:"summary"`
		Then    policy.Verdict `json:"then"`
		When    policy.When    `json:"when"`
	}
	json.Unmarshal(resp.Structured, &out)
	rule := policy.Rule{ID: "r-" + strconv.FormatInt(time.Now().UnixNano()%1e10, 36), Text: req.Text, When: out.When, Then: out.Then}
	for _, id := range rule.When.People {
		if _, err := a.People.Get(ctx, id); err != nil {
			server.WriteError(w, server.StatusError{Status: 422, Msg: "the rule names someone who is not in the house: " + id})
			return
		}
	}
	if err := rule.Validate(); err != nil {
		server.WriteError(w, server.StatusError{Status: 422, Msg: "I could not turn that into a rule I can enforce: " + err.Error()})
		return
	}
	server.WriteJSON(w, 200, map[string]any{"rule": rule, "summary": out.Summary})
}

// testRule shows what a rule would have changed over the last week.
func (a *App) testRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var rule policy.Rule
	if err := server.Decode(r, &rule); err != nil {
		server.WriteError(w, err)
		return
	}
	if err := rule.Validate(); err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	evs, _ := a.Events.List(ctx, event.Query{Types: []string{host.ActionEvent}, Newest: true, Limit: 1000})
	since := time.Now().Add(-7 * 24 * time.Hour)
	test := &policy.Engine{Events: a.Events}
	var hits []map[string]any
	for _, e := range evs {
		if e.Time.Before(since) {
			continue
		}
		var rec host.ActionRecord
		e.Decode(&rec)
		if !mine(ctx, rec.Person) {
			continue
		}
		act := policy.Action{Capability: rec.Capability, Scope: rec.Scope, Args: rec.Args, Risk: capability.Catalog[rec.Capability].Risk, Source: rec.Source}
		if test.Matches(rule, act) {
			hits = append(hits, map[string]any{"event": e.ID, "ts": e.Time, "source": rec.Source, "capability": rec.Capability, "was": rec.Verdict, "would_be": rule.Then})
		}
	}
	if hits == nil {
		hits = []map[string]any{}
	}
	server.WriteJSON(w, 200, map[string]any{"matches": hits})
}

func (a *App) cost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	zone := loadZone(a.Settings(ctx).Zone)
	now := time.Now().In(zone)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, zone)
	evs, _ := a.Events.List(ctx, event.Query{Types: []string{"cost.recorded"}})
	// The budget is the house's, so totals are shared; which routines of
	// other people spent it is not.
	mineIDs := map[string]bool{}
	if list, err := a.myRoutines(ctx); err == nil {
		for _, rt := range list {
			mineIDs["routine:"+rt.ID] = true
		}
	}
	byDay := map[string]float64{}
	bySource := map[string]float64{}
	month := 0.0
	for _, e := range evs {
		if e.Time.Before(monthStart) {
			continue
		}
		var c struct {
			USD    float64 `json:"usd"`
			Source string  `json:"source"`
			Ref    string  `json:"ref"`
		}
		e.Decode(&c)
		month += c.USD
		byDay[e.Time.In(zone).Format("2006-01-02")] += c.USD
		key := c.Source
		if strings.HasPrefix(c.Ref, "routine:") {
			key = strings.SplitN(c.Ref, "#", 2)[0]
			if !mineIDs[key] {
				key = "others"
			}
		}
		bySource[key] += c.USD
	}
	// By model and by job, from each answered call this month.
	byModel, byJob, subByModel := map[string]float64{}, map[string]float64{}, map[string]float64{}
	calls := map[string]int{}
	subToday, subMonth := 0.0, 0.0
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	used, _ := a.Events.List(ctx, event.Query{Types: []string{"model.used"}})
	for _, e := range used {
		if e.Time.Before(monthStart) {
			continue
		}
		var u struct {
			Job          string  `json:"job"`
			Model        string  `json:"model"`
			USD          float64 `json:"usd"`
			Subscription bool    `json:"subscription"`
		}
		e.Decode(&u)
		calls[u.Model]++
		// A subscription's calls cost no money: their API equivalent is
		// shown apart.
		if u.Subscription {
			subByModel[u.Model] += u.USD
			subMonth += u.USD
			if !e.Time.Before(dayStart) {
				subToday += u.USD
			}
			continue
		}
		byModel[u.Model] += u.USD
		byJob[u.Job] += u.USD
	}
	days := float64(now.Day())
	daysInMonth := float64(time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, zone).Day())
	spent, _ := a.Budget.Today(ctx)
	server.WriteJSON(w, 200, map[string]any{
		"today": spent, "limit": a.Budget.Limit(ctx), "month": month,
		"projected_month": month / days * daysInMonth,
		"by_day":          byDay, "by_source": bySource,
		"by_model": byModel, "by_job": byJob, "calls_by_model": calls,
		"subscription": map[string]any{"today": subToday, "month": subMonth, "by_model": subByModel},
	})
}

// setup reports what the first-run guide still needs.
func (a *App) setup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	has := func(name string) bool { _, err := a.Vault.Get(ctx, name); return err == nil }
	chat, _ := a.Channel.Chat(ctx)
	done, _ := a.Events.Get(ctx, "setup.done")
	preset, _ := a.Events.Get(ctx, "setup.preset")
	demo, _ := a.Events.Get(ctx, "demo")
	routines, _ := a.myRoutines(ctx)
	server.WriteJSON(w, 200, map[string]any{
		"done":     done == "true" || len(routines) > 0,
		"demo":     demo == "true",
		"telegram": chat != 0,
		"mail":     has("mail.password"),
		"calendar": has("calendar.feeds"),
		"preset":   preset,
		"claude":   claudeInstalled(),
	})
}

func (a *App) setupDone(w http.ResponseWriter, r *http.Request) {
	a.Events.Put(r.Context(), "setup.done", "true")
	a.Events.Append(r.Context(), "setup.finished", "human:owner", map[string]string{})
	server.WriteJSON(w, 200, map[string]bool{"done": true})
}
