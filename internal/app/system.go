package app

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/denerFernandes/pimpo/internal/connector/services"
	"github.com/denerFernandes/pimpo/internal/event"
	"github.com/denerFernandes/pimpo/internal/server"
	"github.com/denerFernandes/pimpo/internal/store"
	"github.com/denerFernandes/pimpo/internal/sysinfo"
)

// The system panel: how busy this computer and Pimpo are, what is running
// now, and the state of every part that talks to the outside.

type component struct {
	ID     string `json:"id"`
	Group  string `json:"group"` // channel, account, service, access, backup, brain
	Name   string `json:"name"`
	State  string `json:"state"` // ok, off, error, waiting
	Detail string `json:"detail,omitempty"`
}

func (a *App) systemRoutes() {
	a.Server.Handle("GET /api/system", a.system)
}

func (a *App) system(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	home := a.Home
	if home == "" {
		home = "."
	}
	running, _ := a.Store.Explorations(ctx, store.ExplorationRunning)
	runs, _ := a.Store.RecentRuns(ctx, store.RunRunning, 0, 50)
	today, _ := a.Store.RecentRuns(ctx, "", 0, 200)
	now := time.Now().In(loadZone(a.Settings(ctx).Zone))
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var ok, failed int
	for _, run := range today {
		if run.StartedAt.Before(midnight) {
			break
		}
		switch run.Outcome {
		case store.RunOK:
			ok++
		case store.RunFailed:
			failed++
		}
	}
	server.WriteJSON(w, 200, map[string]any{
		"process":    sysinfo.ReadProcess(),
		"host":       sysinfo.ReadHost(home),
		"activity":   map[string]int{"explorations": len(running), "runs": len(runs), "approvals": len(a.Approvals.Open()), "runs_ok_today": ok, "runs_failed_today": failed},
		"components": a.components(ctx),
		"version":    a.Version,
	})
}

func (a *App) components(ctx context.Context) []component {
	has := func(name string) bool { v, _ := a.Vault.Get(ctx, name); return v != "" }
	state := func(on bool) string {
		if on {
			return "ok"
		}
		return "off"
	}
	var out []component

	chat, _ := a.Channel.Chat(ctx)
	tg := component{ID: "telegram", Group: "channel", Name: "Telegram", State: state(chat != 0)}
	if has("telegram.token") && chat == 0 {
		tg.State, tg.Detail = "waiting", "waiting for pairing"
	}
	if down, why := a.health.down("telegram"); down {
		tg.State, tg.Detail = "error", why
	}
	out = append(out, tg)
	wa, _ := a.Events.Get(ctx, "whatsapp.owner")
	out = append(out, component{ID: "whatsapp", Group: "channel", Name: "WhatsApp", State: state(has("whatsapp.token") && wa != "")})
	linksMu.Lock()
	live := map[string]bool{}
	for k := range a.links {
		live[k] = true
	}
	linksMu.Unlock()
	for _, k := range services.LinkKinds {
		kind, _ := services.Get(k)
		c := component{ID: k, Group: "channel", Name: kind.Title, State: "off"}
		if live[k] {
			c.State = "ok"
			if owner, _ := a.Events.Get(ctx, linkOwnerKey(k)); owner == "" {
				c.State, c.Detail = "waiting", "waiting for pairing"
			}
			if down, why := a.health.down(k); down {
				c.State, c.Detail = "error", why
			}
		}
		out = append(out, c)
	}

	mailAuth, _ := a.Events.Get(ctx, "mail.auth")
	out = append(out, component{ID: "mail", Group: "account", Name: "E-mail", State: state(has("mail.password") || mailAuth == "oauth")})
	cal, _ := a.Events.Get(ctx, "calendar.source")
	out = append(out, component{ID: "calendar", Group: "account", Name: "Agenda", State: state(cal == "google" || has("calendar.feeds"))})

	out = append(out, component{ID: "claude", Group: "brain", Name: "Claude Code", State: state(claudeInstalled())})
	out = append(out, component{ID: "jev", Group: "brain", Name: "Jev", State: state(has("typesafe.key"))})

	for _, k := range services.All() {
		if slices.Contains(services.LinkKinds, k.ID) || len(k.Fields) == 0 {
			continue
		}
		if ok, _ := a.catalogConfigured(ctx, k); ok {
			out = append(out, component{ID: k.ID, Group: "service", Name: k.Title, State: "ok"})
		}
	}
	a.mu.Lock()
	for name := range a.external {
		out = append(out, component{ID: "mcp:" + name, Group: "service", Name: name, State: "ok", Detail: "MCP"})
	}
	for _, e := range a.externalErrs {
		out = append(out, component{ID: "mcp-broken", Group: "service", Name: "MCP", State: "error", Detail: e})
	}
	a.mu.Unlock()

	if a.Remote != nil {
		st := a.Remote.Status()
		c := component{ID: "tailscale", Group: "access", Name: "Tailscale", State: "off", Detail: st.URL}
		switch st.State {
		case "running":
			c.State = "ok"
		case "starting", "needs_login", "needs_funnel":
			c.State, c.Detail = "waiting", st.State
		case "error":
			c.State, c.Detail = "error", st.Error
		}
		out = append(out, c)
		out = append(out, component{ID: "lan", Group: "access", Name: "Em casa", State: state(a.LAN.URL() != ""), Detail: a.LAN.URL()})
	}

	cfg := a.cloudConfig(ctx)
	if cfg.Kind != "" {
		c := component{ID: "cloud", Group: "backup", Name: "Backup na nuvem", State: "waiting", Detail: "no backup yet"}
		if last, ok := a.lastCloudRun(ctx); ok {
			if last.OK {
				c.State, c.Detail = "ok", last.At.Format(time.RFC3339)
			} else {
				c.State, c.Detail = "error", last.Error
			}
		}
		out = append(out, c)
	}
	return out
}

// lastEvent returns the error of the latest event of this type for a
// channel, if it happened in the last ten minutes.
func (a *App) lastEvent(ctx context.Context, typ, channel string) string {
	evs, err := a.Events.List(ctx, event.Query{Types: []string{typ}, Limit: 20, Newest: true})
	if err != nil {
		return ""
	}
	for _, e := range evs {
		var d struct {
			Channel string `json:"channel"`
			Error   string `json:"error"`
		}
		json.Unmarshal(e.Data, &d)
		if d.Channel == channel && time.Since(e.Time) < 10*time.Minute {
			return d.Error
		}
	}
	return ""
}
