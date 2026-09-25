package app

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/denerFernandes/zodim/internal/event"
	"github.com/denerFernandes/zodim/internal/runtime"
	"github.com/denerFernandes/zodim/internal/server"
	"github.com/denerFernandes/zodim/internal/store"
	"github.com/denerFernandes/zodim/internal/telegram"
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
}

func (a *App) summary(ctx context.Context, r store.Routine) routineSummary {
	sum := routineSummary{ID: r.ID, Name: r.Body.Name, Description: r.Body.Description, State: r.State, Version: r.Version,
		Capabilities: r.Body.Manifest.Capabilities, Schedule: r.Schedule(), DefaultSchedule: r.Body.Manifest.Schedule, Runs: []string{},
		Params: r.Body.Manifest.Params, Values: map[string]any{}}
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
	routines, _ := a.Store.Routines(ctx)
	broken := 0
	for _, rt := range routines {
		if rt.State == store.RoutineBroken {
			broken++
		}
	}
	ready, _ := a.Store.Explorations(ctx, store.ExplorationReady)
	chat, _ := a.Channel.Chat(ctx)
	intact, _ := a.Events.Verify(ctx)
	server.WriteJSON(w, 200, map[string]any{
		"budget":          map[string]float64{"spent": spent, "limit": a.Budget.Limit(ctx)},
		"healthy":         broken == 0,
		"broken":          broken,
		"awaiting":        len(ready),
		"telegram_paired": chat != 0,
		"log_intact":      intact == 0,
		"claude":          claudeInstalled(),
	})
}

func (a *App) listRoutines(w http.ResponseWriter, r *http.Request) {
	routines, err := a.Store.Routines(r.Context())
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
	rt, err := a.Store.Routine(ctx, id)
	if err != nil {
		server.WriteError(w, notFound(err))
		return
	}
	versions, _ := a.Store.Versions(ctx, id)
	runs, _ := a.Store.Runs(ctx, id, 50)
	server.WriteJSON(w, 200, map[string]any{"summary": a.summary(ctx, rt), "routine": rt.Body, "versions": versions, "runs": runs})
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
	if _, err := a.Store.Routine(ctx, id); err != nil {
		server.WriteError(w, notFound(err))
		return
	}
	var err error
	switch r.PathValue("action") {
	case "run":
		a.Store.SetRoutineState(ctx, id, store.RoutineActive)
		a.Scheduler.Changed(ctx, id)
		run, runErr := a.Scheduler.RunNow(context.WithoutCancel(ctx), id, "owner")
		server.WriteJSON(w, 200, map[string]any{"run": run, "error": errText(runErr)})
		return
	case "pause":
		err = a.Store.SetRoutineState(ctx, id, store.RoutinePaused)
	case "resume":
		err = a.Store.SetRoutineState(ctx, id, store.RoutineActive)
	case "repair":
		var eid string
		eid, err = a.Explore.Repair(ctx, id, "", "human:owner")
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
	a.Events.Append(ctx, "routine."+r.PathValue("action")+"d", "human:owner", map[string]string{"routine": id})
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
	list, err := a.Store.Explorations(r.Context(), states...)
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
	e, err := a.Store.Exploration(r.Context(), r.PathValue("id"))
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
	id, err := a.Explore.Start(r.Context(), req.Request, "human:owner")
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	server.WriteJSON(w, 202, map[string]string{"id": id})
}

func (a *App) explorationAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	switch r.PathValue("action") {
	case "compile":
		rt, err := a.Explore.Approve(context.WithoutCancel(ctx), id, "human:owner")
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 422, Msg: err.Error()})
			return
		}
		server.WriteJSON(w, 200, a.summary(ctx, rt))
	case "explore":
		e, err := a.Store.Exploration(ctx, id)
		if err != nil || e.State != store.ExplorationImported {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no imported task with that id"})
			return
		}
		started, err := a.Explore.Start(context.WithoutCancel(ctx), e.Request, "human:owner")
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
			return
		}
		a.Explore.Discard(ctx, id, "human:owner")
		server.WriteJSON(w, 202, map[string]string{"id": started})
	case "discard":
		if err := a.Explore.Discard(ctx, id, "human:owner"); err != nil {
			server.WriteError(w, notFound(err))
			return
		}
		server.WriteJSON(w, 200, map[string]string{"state": "discarded"})
	default:
		server.WriteError(w, server.StatusError{Status: 404, Msg: "unknown action"})
	}
}

func (a *App) putSettings(w http.ResponseWriter, r *http.Request) {
	var s Settings
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
	has := func(name string) bool { _, err := a.Vault.Get(ctx, name); return err == nil }
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
		if _, merr := (telegram.Bot{Token: tok}).Me(ctx); merr != nil {
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
		err = a.Vault.Set(ctx, "mail.password", strings.ReplaceAll(req["password"], " ", ""))
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
