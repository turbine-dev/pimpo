package app

import (
	"context"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/denerFernandes/pimpo/internal/connector/services"
	"github.com/denerFernandes/pimpo/internal/llm"
	"github.com/denerFernandes/pimpo/internal/models"
	"github.com/denerFernandes/pimpo/internal/server"
	"github.com/denerFernandes/pimpo/internal/snapshot"
	"github.com/denerFernandes/pimpo/internal/store"
	"github.com/denerFernandes/pimpo/internal/sysinfo"
)

// The doctor checks every part for real, now: each channel answers, the
// accounts can be read, the models answer one word, the backups are
// recent, and says what to do about each part that fails. It only reads,
// except the model tests, capped at a cent each.

type finding struct {
	ID     string `json:"id"`
	Group  string `json:"group"`
	Name   string `json:"name"`
	State  string `json:"state"` // ok, warn, fail
	Detail string `json:"detail,omitempty"`
	// Fix names what to do (a doc.fix.* text in the app) and Link where.
	Fix  string `json:"fix,omitempty"`
	Link string `json:"link,omitempty"`
}

func (a *App) doctorRoutes() {
	a.Server.Handle("POST /api/doctor", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		list := a.doctor(ctx)
		a.Events.Append(r.Context(), "doctor.ran", actor(r.Context()), map[string]int{"checked": len(list), "failed": countState(list, "fail"), "warned": countState(list, "warn")})
		server.WriteJSON(w, 200, list)
	})
}

func countState(list []finding, state string) int {
	n := 0
	for _, f := range list {
		if f.State == state {
			n++
		}
	}
	return n
}

func (a *App) doctor(ctx context.Context) []finding {
	var mu sync.Mutex
	var out []finding
	var wg sync.WaitGroup
	add := func(f finding) {
		mu.Lock()
		out = append(out, f)
		mu.Unlock()
	}
	check := func(fn func()) {
		wg.Add(1)
		go func() { defer wg.Done(); fn() }()
	}
	has := func(name string) bool { v, _ := a.Vault.Get(ctx, name); return v != "" }

	// Channels.
	if has("telegram.token") {
		check(func() {
			f := finding{ID: "telegram", Group: "channel", Name: "Telegram", State: "ok", Link: "/connections"}
			if bot, ok := a.bot(ctx).(interface {
				Me(context.Context) (string, error)
			}); ok {
				if name, err := bot.Me(ctx); err != nil {
					f.State, f.Detail, f.Fix = "fail", err.Error(), "doc.fix.telegramToken"
				} else {
					f.Detail = "@" + name
				}
			}
			if chat, _ := a.Channel.Chat(ctx); f.State == "ok" && chat == 0 {
				f.State, f.Fix = "warn", "doc.fix.pair"
			}
			add(f)
		})
	}
	linksMu.Lock()
	links := map[string]*linkRun{}
	for k, v := range a.links {
		links[k] = v
	}
	linksMu.Unlock()
	for kind, run := range links {
		kind, run := kind, run
		check(func() {
			k, _ := services.Get(kind)
			f := finding{ID: kind, Group: "channel", Name: k.Title, State: "ok", Link: "/connections"}
			if err := run.link.Check(ctx); err != nil {
				f.State, f.Detail, f.Fix = "fail", err.Error(), "doc.fix.linkCheck"
			} else if owner, _ := a.Events.Get(ctx, linkOwnerKey(kind)); owner == "" {
				f.State, f.Fix = "warn", "doc.fix.pair"
			}
			add(f)
		})
	}

	// Accounts: one small read each, as a routine would do it.
	if mailAuth, _ := a.Events.Get(ctx, "mail.auth"); has("mail.password") || mailAuth == "oauth" {
		check(func() {
			f := finding{ID: "mail", Group: "account", Name: "E-mail", State: "ok", Link: "/connections"}
			if _, err := a.Router.Call(ctx, "gmail.search", "", map[string]any{"query": "", "days": 1, "max": 1}); err != nil {
				f.State, f.Detail, f.Fix = "fail", err.Error(), "doc.fix.mail"
			}
			add(f)
		})
	}
	if cal, _ := a.Events.Get(ctx, "calendar.source"); cal == "google" || has("calendar.feeds") {
		check(func() {
			f := finding{ID: "calendar", Group: "account", Name: "Agenda", State: "ok", Link: "/connections"}
			now := time.Now()
			if _, err := a.Router.Call(ctx, "calendar.events", "", map[string]any{"from": now.Format(time.RFC3339), "to": now.Add(24 * time.Hour).Format(time.RFC3339)}); err != nil {
				f.State, f.Detail, f.Fix = "fail", err.Error(), "doc.fix.calendar"
			}
			add(f)
		})
	}

	// Models: every model a job may use.
	s := a.Settings(ctx)
	seen := map[string]bool{}
	for _, job := range []string{"explore", "compile", "judge"} {
		primary := map[string]string{"explore": s.ExploreModel, "compile": s.CompileModel, "judge": s.JudgeModel}[job]
		for _, m := range append([]string{primary}, s.fallbacks(job)...) {
			if m == "" || seen[m] {
				continue
			}
			seen[m] = true
			m := m
			check(func() { add(a.checkModel(ctx, m)) })
		}
	}

	// Services with a check of their own.
	for _, k := range services.All() {
		if slices.Contains(services.LinkKinds, k.ID) || k.Probe == nil {
			continue
		}
		if ok, _ := a.catalogConfigured(ctx, k); !ok {
			continue
		}
		k := k
		check(func() {
			f := finding{ID: k.ID, Group: "service", Name: k.Title, State: "ok", Link: "/connections"}
			if err := k.Probe(ctx, a.catalogConfig(k.ID)); err != nil {
				f.State, f.Detail, f.Fix = "fail", err.Error(), "doc.fix.service"
			}
			add(f)
		})
	}
	a.mu.Lock()
	for _, e := range a.externalErrs {
		out = append(out, finding{ID: "mcp", Group: "service", Name: "MCP", State: "fail", Detail: e, Fix: "doc.fix.mcp", Link: "/connections"})
	}
	a.mu.Unlock()

	wg.Wait()
	out = append(out, a.localFindings(ctx)...)
	order := map[string]int{"fail": 0, "warn": 1, "ok": 2}
	sort.SliceStable(out, func(i, j int) bool {
		if order[out[i].State] != order[out[j].State] {
			return order[out[i].State] < order[out[j].State]
		}
		return out[i].Group+out[i].Name < out[j].Group+out[j].Name
	})
	return out
}

// checkModel tests an API model with one word; Claude Code is only looked
// for, so the check does not spend the owner's subscription limit.
func (a *App) checkModel(ctx context.Context, id string) finding {
	f := finding{ID: "model:" + id, Group: "brain", Name: id, State: "ok", Link: "/settings#modelos"}
	api, isAPI, err := a.apiModel(ctx, id)
	if !isAPI && llm.IsOpencode(id) {
		f.Name = "opencode · " + strings.TrimPrefix(id, "opencode:")
		if llm.OpencodeBinary() == "" {
			f.State, f.Fix = "fail", "doc.fix.opencode"
		}
		return f
	}
	if !isAPI && isCodex(id) {
		f.Name = "Codex · ChatGPT"
		if llm.CodexBinary() == "" {
			f.State, f.Fix = "fail", "doc.fix.codex"
		}
		return f
	}
	if !isAPI {
		f.Name = "Claude Code · " + id
		if !claudeInstalled() {
			f.State, f.Fix = "fail", "doc.fix.claude"
		}
		return f
	}
	if err == nil {
		var resp llm.Response
		resp, err = api.Generate(ctx, llm.Request{Prompt: "Answer with the single word: ok", MaxCostUSD: 0.01})
		if err == nil {
			a.Budget.Record(ctx, budgetCost(resp.CostUSD, "model test"))
			return f
		}
	}
	f.State, f.Detail, f.Fix = "fail", err.Error(), "doc.fix.model."+models.Problem(err)
	return f
}

// localFindings are the checks that need no network.
func (a *App) localFindings(ctx context.Context) []finding {
	var out []finding
	if a.Home != "" {
		h := sysinfo.ReadHost(a.Home)
		if h.DiskSize > 0 && h.DiskFree < 5e9 {
			out = append(out, finding{ID: "disk", Group: "system", Name: "Disco", State: "warn", Detail: formatGB(h.DiskFree), Fix: "doc.fix.disk"})
		}
		list, _ := snapshot.List(a.Home)
		if len(list) == 0 || time.Since(list[0].When) > 48*time.Hour {
			out = append(out, finding{ID: "snapshots", Group: "backup", Name: "Cópias locais", State: "warn", Fix: "doc.fix.snapshots", Link: "/settings#backup"})
		} else {
			out = append(out, finding{ID: "snapshots", Group: "backup", Name: "Cópias locais", State: "ok", Detail: list[0].When.Format(time.RFC3339)})
		}
	}
	if cfg := a.cloudConfig(ctx); cfg.Kind != "" {
		f := finding{ID: "cloud", Group: "backup", Name: "Backup na nuvem", State: "ok", Link: "/settings#backup"}
		last, ok := a.lastCloudRun(ctx)
		switch {
		case !ok:
			f.State, f.Fix = "warn", "doc.fix.cloudNever"
		case !last.OK:
			f.State, f.Detail, f.Fix = "fail", last.Error, "doc.fix.cloud"
		case time.Since(last.At) > 8*24*time.Hour:
			f.State, f.Fix = "warn", "doc.fix.cloudOld"
		default:
			f.Detail = last.At.Format(time.RFC3339)
		}
		out = append(out, f)
	}
	if routines, err := a.Store.Routines(ctx); err == nil {
		broken := 0
		for _, r := range routines {
			if r.State == store.RoutineBroken {
				broken++
			}
		}
		if broken > 0 {
			out = append(out, finding{ID: "routines", Group: "system", Name: "Rotinas", State: "warn", Detail: formatCount(broken), Fix: "doc.fix.routines", Link: "/routines"})
		}
	}
	if spent, err := a.Budget.Today(ctx); err == nil {
		if limit := a.Budget.Limit(ctx); limit > 0 && spent >= limit {
			out = append(out, finding{ID: "budget", Group: "system", Name: "Limite de gasto", State: "warn", Fix: "doc.fix.budget", Link: "/settings"})
		}
	}
	return out
}

func formatGB(b uint64) string { return strconv.FormatFloat(float64(b)/1e9, 'f', 1, 64) + " GB" }

func formatCount(n int) string { return strconv.Itoa(n) }
