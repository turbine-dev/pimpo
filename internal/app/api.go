package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/desktop"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/runtime"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/store"
	"github.com/turbine-dev/pimpo/internal/telegram"
	"github.com/turbine-dev/pimpo/internal/usage"
)

func (a *App) routes() {
	s := a.Server
	s.HandlePublic("POST /mcp/explore/{id}", a.Explore.MCP)
	s.Handle("GET /api/state", a.state)
	s.Handle("GET /api/routines", a.listRoutines)
	s.Handle("GET /api/routines/{id}", a.getRoutine)
	s.Handle("POST /api/routines/{id}/{action}", a.routineAction)
	s.Handle("GET /api/explorations", a.listExplorations)
	s.Handle("POST /api/explorations", a.startExploration)
	s.Handle("GET /api/explorations/{id}", a.getExploration)
	s.Handle("POST /api/explorations/{id}/{action}", a.explorationAction)
	s.Handle("GET /api/settings", func(w http.ResponseWriter, r *http.Request) { server.WriteJSON(w, 200, a.Settings(r.Context())) })
	s.Handle("PUT /api/settings", a.putSettings)
	s.Handle("PUT /api/budget", a.putBudget)
	s.Handle("GET /api/connections", a.connections)
	s.Handle("PUT /api/connections/{kind}", a.putConnection)
	s.Handle("DELETE /api/connections/{kind}", a.deleteConnection)
	s.Handle("POST /api/open", a.openLink)
	s.Handle("GET /api/runs", a.recentRuns)
	s.Handle("GET /api/report", a.usageReport)
}

// recentRuns is the run history of every routine.
func (a *App) recentRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	outcome := q.Get("outcome")
	if outcome != "" && outcome != store.RunOK && outcome != store.RunFailed && outcome != store.RunSkipped {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "unknown outcome"})
		return
	}
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	// Runs of other people's routines are left out, reading further back
	// so a page is still full.
	ctx := r.Context()
	mineIDs := map[string]bool{}
	if list, err := a.myRoutines(ctx); err == nil {
		for _, rt := range list {
			mineIDs[rt.ID] = true
		}
	}
	out := []store.RecentRun{}
	for len(out) < limit {
		runs, err := a.Store.RecentRuns(ctx, outcome, before, 200)
		if err != nil {
			server.WriteError(w, err)
			return
		}
		for _, run := range runs {
			if mineIDs[run.Routine] && len(out) < limit {
				out = append(out, run)
			}
		}
		if len(runs) < 200 {
			break
		}
		before = runs[len(runs)-1].ID
	}
	server.WriteJSON(w, 200, out)
}

// openLink opens a link in this computer's browser, for the desktop app,
// whose window cannot open one itself. Only the window on this machine may
// ask; a paired phone cannot open pages on the computer.
func (a *App) openLink(w http.ResponseWriter, r *http.Request) {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); !a.DesktopNotify || ip == nil || !ip.IsLoopback() {
		server.WriteError(w, server.StatusError{Status: 403, Msg: "links open only in the desktop app"})
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	if err := desktop.Open(r.Context(), req.URL); err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	server.WriteJSON(w, 200, map[string]bool{"opened": true})
}

type routineSummary struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	State        string   `json:"state"`
	Version      int      `json:"version"`
	NextRun      string   `json:"next_run,omitempty"`
	Runs         []string `json:"runs"`
	CostMonthUSD float64  `json:"cost_month_usd"`
	Capabilities []string `json:"capabilities"`
	Schedule     string   `json:"schedule"`
	// DefaultSchedule is the manifest's, to offer going back to it.
	DefaultSchedule string          `json:"default_schedule"`
	Params          []runtime.Param `json:"params"`
	// Values are the parameters as the routine sees them, defaults included.
	Values map[string]any `json:"values"`
	// Update is a newer version in the gallery the routine came from.
	Update *galleryUpdate `json:"gallery_update,omitempty"`
	// Watch is what wakes the routine instead of a clock, if anything.
	Watch *runtime.Watch `json:"watch,omitempty"`
	// Model is the routine's own model for judgments and texts.
	Model string `json:"model,omitempty"`
	// Effort is how hard that model thinks, "" for the default.
	Effort string `json:"effort,omitempty"`
	// Webhook says another service starts it by calling its address.
	Webhook bool `json:"webhook,omitempty"`
	// Thinks says whether the routine asks a model anything (judgments or
	// texts); only then does its model matter.
	Thinks bool `json:"thinks"`
}

func (a *App) summary(ctx context.Context, r store.Routine) routineSummary {
	sum := routineSummary{ID: r.ID, Name: r.Body.Name, Description: r.Body.Description, State: r.State, Version: r.Version,
		Capabilities: r.Body.Manifest.Capabilities, Schedule: r.Schedule(), DefaultSchedule: r.Body.Manifest.Schedule, Runs: []string{},
		Params: r.Body.Manifest.Params, Values: map[string]any{}, Model: r.Settings.Model, Effort: r.Settings.Effort, Webhook: r.Body.Manifest.Webhook,
		Thinks: len(r.Body.Manifest.Judgments)+len(r.Body.Manifest.Writes) > 0}
	if sum.Params == nil {
		sum.Params = []runtime.Param{}
	}
	if v, err := r.Body.Manifest.ResolveParams(r.Settings.Params); err == nil {
		sum.Values = v
	} else {
		sum.Values = r.Settings.Params
	}
	if n := a.Scheduler.Next(r.ID); !n.IsZero() {
		sum.NextRun = n.Format(time.RFC3339)
	}
	sum.Update = a.pendingUpdate(ctx, r)
	sum.Watch = r.Watch()
	runs, _ := a.Store.Runs(ctx, r.ID, 14)
	for i := len(runs) - 1; i >= 0; i-- {
		o := runs[i].Outcome
		if o == store.RunRunning {
			continue
		}
		sum.Runs = append(sum.Runs, o)
	}
	now := time.Now().In(loadZone(a.Settings(ctx).Zone))
	sum.CostMonthUSD, _ = a.Store.RunCostSince(ctx, r.ID, time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()))
	return sum
}

func (a *App) state(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	spent, _ := a.Budget.Today(ctx)
	routines, _ := a.myRoutines(ctx)
	broken := 0
	for _, rt := range routines {
		if rt.State == store.RoutineBroken {
			broken++
		}
	}
	ready, _ := a.myExplorations(ctx, store.ExplorationReady)
	chat, _ := a.Channel.Chat(ctx)
	intact, _ := a.Events.Verify(ctx)
	server.WriteJSON(w, 200, map[string]any{
		"budget":          map[string]float64{"spent": spent, "limit": a.Budget.Limit(ctx)},
		"healthy":         broken == 0,
		"broken":          broken,
		"awaiting":        len(ready),
		"approvals":       len(a.myApprovals(ctx)),
		"person":          people.From(ctx),
		"role":            a.roleOf(ctx),
		"name":            a.nameOf(ctx),
		"admin_account":   a.hasAdmin(ctx),
		"telegram_paired": chat != 0,
		"log_intact":      intact == 0,
		"claude":          claudeInstalled(),
	})
}

func (a *App) listRoutines(w http.ResponseWriter, r *http.Request) {
	routines, err := a.myRoutines(r.Context())
	if err != nil {
		server.WriteError(w, err)
		return
	}
	out := make([]routineSummary, 0, len(routines))
	for _, rt := range routines {
		out = append(out, a.summary(r.Context(), rt))
	}
	server.WriteJSON(w, 200, out)
}

func (a *App) getRoutine(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	rt, err := a.myRoutine(ctx, id)
	if err != nil {
		server.WriteError(w, notFound(err))
		return
	}
	versions, _ := a.Store.Versions(ctx, id)
	runs, _ := a.Store.Runs(ctx, id, 50)
	server.WriteJSON(w, 200, map[string]any{"summary": a.summary(ctx, rt), "routine": rt.Body, "versions": versions, "runs": runs,
		"state": a.Scheduler.State(ctx, id), "used_by": a.usedBy(ctx, id)})
}

// usedBy lists the routines that run this one as a helper.
func (a *App) usedBy(ctx context.Context, id string) []string {
	out := []string{}
	all, _ := a.myRoutines(ctx)
	for _, r := range all {
		for _, u := range r.Body.Manifest.Uses {
			if u == id {
				out = append(out, r.ID)
			}
		}
	}
	return out
}

func notFound(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return server.StatusError{Status: 404, Msg: "not found"}
	}
	return err
}

func (a *App) routineAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	if _, err := a.myRoutine(ctx, id); err != nil {
		server.WriteError(w, notFound(err))
		return
	}
	var err error
	switch r.PathValue("action") {
	case "run":
		a.unstop(ctx, id)
		run, runErr := a.Scheduler.RunNow(context.WithoutCancel(ctx), id, "owner")
		server.WriteJSON(w, 200, map[string]any{"run": run, "error": errText(runErr)})
		return
	case "forget":
		err = a.Scheduler.SaveState(ctx, id, nil)
		if err == nil {
			a.Events.Append(ctx, "routine.state.forgotten", actor(ctx), map[string]string{"routine": id})
		}
	case "pause":
		err = a.Store.SetRoutineState(ctx, id, store.RoutinePaused)
	case "resume":
		err = a.Store.SetRoutineState(ctx, id, store.RoutineActive)
	case "widget":
		// Turn into a widget: re-explore the task so it also ends with
		// widget.show; approving the result saves a new version.
		var body struct {
			Kind string `json:"kind"`
		}
		_ = server.Decode(r, &body)
		change := "Also show the result at a glance on the owner's dashboard with widget.show, keeping everything else the routine does."
		if slices.Contains(widgetKinds, body.Kind) {
			change += " Use the " + body.Kind + " kind."
		}
		var eid string
		if eid, err = a.Explore.Improve(ctx, id, change, actor(ctx)); err == nil {
			server.WriteJSON(w, 202, map[string]string{"exploration": eid})
			return
		}
	case "repair":
		var eid string
		eid, err = a.Explore.Repair(ctx, id, "", actor(ctx))
		if err == nil {
			server.WriteJSON(w, 202, map[string]string{"exploration": eid})
			return
		}
	default:
		err = server.StatusError{Status: 404, Msg: "unknown action"}
	}
	if err != nil {
		server.WriteError(w, err)
		return
	}
	a.Scheduler.Changed(ctx, id)
	a.Events.Append(ctx, "routine."+r.PathValue("action")+"d", actor(ctx), map[string]string{"routine": id})
	server.WriteJSON(w, 200, map[string]string{"state": "ok"})
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (a *App) listExplorations(w http.ResponseWriter, r *http.Request) {
	var states []string
	if s := r.URL.Query().Get("state"); s != "" {
		states = strings.Split(s, ",")
	}
	list, err := a.myExplorations(r.Context(), states...)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	for i := range list {
		list[i].Trace = nil
	}
	server.WriteJSON(w, 200, list)
}

func (a *App) getExploration(w http.ResponseWriter, r *http.Request) {
	e, err := a.myExploration(r.Context(), r.PathValue("id"))
	if err != nil {
		server.WriteError(w, notFound(err))
		return
	}
	actions, _ := a.Events.List(r.Context(), event.Query{Types: []string{"action.done"}, Search: `"exploration:` + e.ID + `"`})
	if actions == nil {
		actions = []event.Event{}
	}
	server.WriteJSON(w, 200, map[string]any{"exploration": e, "actions": actions})
}

func (a *App) startExploration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Request string `json:"request"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	id, err := a.Explore.Start(r.Context(), req.Request, actor(r.Context()))
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	server.WriteJSON(w, 202, map[string]string{"id": id})
}

func (a *App) explorationAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	if _, err := a.myExploration(ctx, id); err != nil {
		server.WriteError(w, notFound(err))
		return
	}
	switch r.PathValue("action") {
	case "compile":
		rt, err := a.Explore.Approve(context.WithoutCancel(ctx), id, actor(ctx))
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 422, Msg: err.Error()})
			return
		}
		server.WriteJSON(w, 200, a.summary(ctx, rt))
	case "explore":
		e, err := a.myExploration(ctx, id)
		if err != nil || e.State != store.ExplorationImported {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no imported task with that id"})
			return
		}
		started, err := a.Explore.Start(context.WithoutCancel(ctx), e.Request, actor(ctx))
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
			return
		}
		a.Explore.Discard(ctx, id, actor(ctx))
		server.WriteJSON(w, 202, map[string]string{"id": started})
	case "discard":
		if err := a.Explore.Discard(ctx, id, actor(ctx)); err != nil {
			server.WriteError(w, notFound(err))
			return
		}
		server.WriteJSON(w, 200, map[string]string{"state": "discarded"})
	default:
		server.WriteError(w, server.StatusError{Status: 404, Msg: "unknown action"})
	}
}

func (a *App) putSettings(w http.ResponseWriter, r *http.Request) {
	// Fields left out keep their value: a client may send only what changed.
	s := a.Settings(r.Context())
	if err := server.Decode(r, &s); err != nil {
		server.WriteError(w, err)
		return
	}
	if err := a.SaveSettings(r.Context(), s, "human:owner"); err != nil {
		server.WriteError(w, err)
		return
	}
	server.WriteJSON(w, 200, s)
}

func (a *App) putBudget(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DailyUSD float64 `json:"daily_usd"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	if err := a.Budget.SetLimit(r.Context(), req.DailyUSD, "human:owner"); err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	server.WriteJSON(w, 200, map[string]float64{"daily_usd": req.DailyUSD})
}

type connection struct {
	Kind       string `json:"kind"`
	Configured bool   `json:"configured"`
	Detail     string `json:"detail,omitempty"`
	Paired     bool   `json:"paired,omitempty"`
	Code       string `json:"pairing_code,omitempty"`
	Bot        string `json:"bot,omitempty"`
	Webhook    string `json:"webhook,omitempty"`
	Verify     string `json:"verify_token,omitempty"`
}

func (a *App) connections(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	has := func(name string) bool { return a.Vault.Has(ctx, name) }
	out := []connection{}
	tg := connection{Kind: "telegram", Configured: has("telegram.token")}
	if tg.Configured {
		if b, ok := a.bot(ctx).(telegram.Bot); ok {
			tg.Bot, _ = b.Me(ctx)
		}
		chat, _ := a.Channel.Chat(ctx)
		tg.Paired = chat != 0
		if !tg.Paired {
			tg.Code = a.Channel.PairingCode()
		}
	}
	out = append(out, tg)
	user, _ := a.Events.Get(ctx, "mail.user")
	auth, _ := a.Events.Get(ctx, "mail.auth")
	mailDetail := user
	if auth == "oauth" {
		mailDetail = user + " (Google)"
	}
	out = append(out, connection{Kind: "mail", Configured: user != "" && (has("mail.password") || (auth == "oauth" && has("google.refresh"))), Detail: mailDetail})
	feeds, _ := a.Vault.Get(ctx, "calendar.feeds")
	n := 0
	for _, l := range strings.Split(feeds, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	calDetail := plural(n, "agenda", "agendas")
	if src, _ := a.Events.Get(ctx, "calendar.source"); src == "google" && has("google.refresh") {
		n, calDetail = 1, "Google Agenda"
	}
	out = append(out, connection{Kind: "calendar", Configured: n > 0, Detail: calDetail})
	wa := connection{Kind: "whatsapp", Configured: a.wa(ctx) != nil}
	if wa.Configured {
		wa.Paired = a.ownerWhatsApp(ctx) != ""
		if !wa.Paired {
			wa.Code = a.Channel.PairingCode()
		}
		base, _ := a.Events.Get(ctx, "public_url")
		verify, _ := a.Vault.Get(ctx, "whatsapp.verify_token")
		wa.Webhook, wa.Verify = strings.TrimRight(base, "/")+"/webhook/whatsapp", verify
	}
	out = append(out, wa)
	out = append(out, connection{Kind: "jev", Configured: has("typesafe.key")})
	out = append(out, connection{Kind: "claude", Configured: claudeInstalled(), Detail: "Claude Code"})
	server.WriteJSON(w, 200, out)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

func (a *App) putConnection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req map[string]string
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	var err error
	kind := r.PathValue("kind")
	switch kind {
	case "telegram":
		tok := strings.TrimSpace(req["token"])
		plain, perr := a.plainSecret(ctx, "telegram.token", tok)
		if perr != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: perr.Error()})
			return
		}
		if _, merr := (telegram.Bot{Token: plain}).Me(ctx); merr != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "Telegram did not accept this token"})
			return
		}
		err = a.Vault.Set(ctx, "telegram.token", tok)
		if err == nil {
			a.restartListener(ctx)
		}
	case "whatsapp":
		err = a.putWhatsApp(ctx, req)
		if se, ok := err.(server.StatusError); ok {
			server.WriteError(w, se)
			return
		}
	case "mail":
		if req["user"] == "" || req["password"] == "" {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "email address and app password are required"})
			return
		}
		addr := req["addr"]
		if addr == "" {
			addr = "imap.gmail.com:993"
		}
		a.Events.Put(ctx, "mail.addr", addr)
		a.Events.Put(ctx, "mail.user", strings.TrimSpace(req["user"]))
		err = a.Vault.Set(ctx, "mail.password", appPassword(req["password"]))
	case "calendar":
		var urls []string
		for _, l := range strings.Split(req["feeds"], "\n") {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			if !strings.HasPrefix(l, "https://") {
				server.WriteError(w, server.StatusError{Status: 400, Msg: "calendar links must start with https://"})
				return
			}
			urls = append(urls, l)
		}
		sort.Strings(urls)
		err = a.Vault.Set(ctx, "calendar.feeds", strings.Join(urls, "\n"))
	case "jev":
		err = a.Vault.Set(ctx, "typesafe.key", strings.TrimSpace(req["key"]))
	default:
		err = server.StatusError{Status: 404, Msg: "unknown connection"}
	}
	if err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(ctx, "connection.changed", "human:owner", map[string]string{"kind": kind})
	server.WriteJSON(w, 200, map[string]string{"kind": kind, "state": "configured"})
}

func (a *App) deleteConnection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	kind := r.PathValue("kind")
	names := map[string][]string{"telegram": {"telegram.token"}, "mail": {"mail.password"}, "calendar": {"calendar.feeds"}, "jev": {"typesafe.key"}, "whatsapp": {"whatsapp.token", "whatsapp.app_secret"}}[kind]
	if names == nil {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "unknown connection"})
		return
	}
	for _, n := range names {
		a.Vault.Delete(ctx, n)
	}
	if kind == "telegram" {
		a.Events.Put(ctx, "telegram.chat", "")
	}
	if kind == "whatsapp" {
		a.Events.Put(ctx, "whatsapp.owner", "")
	}
	a.Events.Append(ctx, "connection.removed", "human:owner", map[string]string{"kind": kind})
	server.WriteJSON(w, 200, map[string]string{"kind": kind, "state": "removed"})
}

// respond writes v, or err as a 400 with its message.
func respond(w http.ResponseWriter) func(v any, err error) {
	return func(v any, err error) {
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
			return
		}
		server.WriteJSON(w, 200, v)
	}
}

// unstop puts a routine stopped by a failure back on its schedule before
// running it by hand; a paused one runs once and stays paused.
func (a *App) unstop(ctx context.Context, id string) {
	if r, err := a.myRoutine(ctx, id); err == nil && r.State == store.RoutineBroken {
		a.Store.SetRoutineState(ctx, id, store.RoutineActive)
		a.Scheduler.Changed(ctx, id)
	}
}

// usageReport is how routines did in real use over the last days.
func (a *App) usageReport(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 365 {
		days = 21
	}
	zone, err := time.LoadLocation(a.Settings(r.Context()).Zone)
	if err != nil {
		zone = time.Local
	}
	rep, err := usage.BuildFor(r.Context(), a.Events, a.Store, time.Now(), days, zone, people.From(r.Context()))
	if err != nil {
		server.WriteError(w, err)
		return
	}
	server.WriteJSON(w, 200, map[string]any{"report": rep, "markdown": rep.Markdown(zone)})
}
