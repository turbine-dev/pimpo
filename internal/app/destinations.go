package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/turbine-dev/pimpo/internal/llm"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/turbine-dev/pimpo/internal/connector"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/store"
	"github.com/turbine-dev/pimpo/internal/telegram"
)

// Destinations are the places a routine can deliver to with notify.send:
// the owner's Telegram, extra Telegram bots, WhatsApp, Slack, Discord and
// email. Each routine picks one or more in its settings.

type Destination struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	Ready bool   `json:"ready"`
}

// Bot is an extra Telegram bot that only sends, for example a family group
// bot next to the owner's own.
type Bot struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Chat     int64  `json:"chat,omitempty"`
	ChatName string `json:"chat_name,omitempty"`
}

var botsMu sync.Mutex

func (a *App) bots(ctx context.Context) []Bot {
	raw, _ := a.Events.Get(ctx, "telegram.bots")
	var list []Bot
	json.Unmarshal([]byte(raw), &list)
	return list
}

func (a *App) saveBots(ctx context.Context, list []Bot) error {
	b, _ := json.Marshal(list)
	return a.Events.Put(ctx, "telegram.bots", string(b))
}

func (a *App) botClient(ctx context.Context, id string) (telegram.Bot, error) {
	tok, err := a.Vault.Get(ctx, "telegram.bot."+id)
	if err != nil {
		return telegram.Bot{}, errors.New("that bot is not set up")
	}
	return telegram.Bot{Token: tok, BaseURL: a.TelegramAPI}, nil
}

func (a *App) destinations(ctx context.Context) []Destination {
	chat, _ := a.personChat(ctx)
	tgName := "Telegram"
	if b, ok := a.bot(ctx).(telegram.Bot); ok {
		if u, err := b.Me(ctx); err == nil && u != "" {
			tgName = "Telegram @" + u
		}
	}
	out := []Destination{{ID: "telegram", Label: tgName, Kind: "telegram", Ready: a.bot(ctx) != nil && chat != 0}}
	for _, b := range a.bots(ctx) {
		label := "Telegram @" + b.Username
		if b.ChatName != "" {
			label += " → " + b.ChatName
		}
		out = append(out, Destination{ID: "bot:" + b.ID, Label: label, Kind: "telegram", Ready: b.Chat != 0})
	}
	p, _ := a.People.Get(ctx, people.From(ctx))
	out = append(out, Destination{ID: "whatsapp", Label: "WhatsApp", Kind: "whatsapp", Ready: a.wa(ctx) != nil && p.WhatsApp != ""})
	for _, k := range []string{"slack", "discord"} {
		v, _ := a.catalogConfig(k)(ctx, "webhook")
		out = append(out, Destination{ID: k, Label: map[string]string{"slack": "Slack", "discord": "Discord"}[k], Kind: k, Ready: v != ""})
	}
	user, _ := a.Events.Get(ctx, personal(ctx, "mail.user"))
	out = append(out, Destination{ID: "email", Label: "E-mail " + user, Kind: "email", Ready: user != ""})
	return out
}

// notifyCap is notify.send.
type notifyCap struct{ a *App }

func (notifyCap) Capabilities() []string { return []string{"notify.send"} }

func (n notifyCap) Call(ctx context.Context, _, _ string, args any) (any, error) {
	var in struct {
		Text string `json:"text"`
	}
	if err := connector.Args(args, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Text) == "" {
		return nil, errors.New("text is empty")
	}
	targets := host.DestinationsFrom(ctx)
	if len(targets) == 0 {
		// No choice made: the usual channel, and the inbox, where every
		// notice is kept anyway.
		for _, d := range n.a.destinations(ctx) {
			if d.Ready && (d.ID == "telegram" || d.ID == "whatsapp") {
				targets = []string{d.ID}
				break
			}
		}
	}
	var delivered, failed []string
	for _, t := range targets {
		if err := n.a.deliver(ctx, t, in.Text); err != nil {
			failed = append(failed, t+": "+err.Error())
			continue
		}
		delivered = append(delivered, t)
	}
	if len(targets) == 0 {
		n.a.Channel.Notify(ctx, explore.Notice{Text: in.Text, To: people.From(ctx)})
		delivered = []string{"inbox"}
	}
	if len(delivered) == 0 {
		return nil, fmt.Errorf("nothing delivered: %s", strings.Join(failed, "; "))
	}
	return map[string]any{"ok": true, "delivered": delivered, "failed": failed}, nil
}

func (a *App) deliver(ctx context.Context, target, text string) error {
	switch {
	case target == "telegram":
		_, err := a.Router.Call(ctx, "telegram.send", "", map[string]any{"text": text})
		return err
	case target == "whatsapp":
		_, err := a.Router.Call(ctx, "whatsapp.send", "", map[string]any{"text": text})
		return err
	case target == "slack" || target == "discord":
		_, err := a.Router.Call(ctx, target+".send", "", map[string]any{"text": text})
		return err
	case target == "email":
		me, _ := a.Events.Get(ctx, personal(ctx, "mail.user"))
		if me == "" {
			return errors.New("email is not set up")
		}
		subject, _, _ := strings.Cut(text, "\n")
		if r := []rune(subject); len(r) > 80 {
			subject = string(r[:79]) + "…"
		}
		_, err := a.Router.Call(ctx, "gmail.send", "", map[string]any{"to": me, "subject": "Pimpo: " + subject, "body": text})
		return err
	case strings.HasPrefix(target, "bot:"):
		id := strings.TrimPrefix(target, "bot:")
		for _, b := range a.bots(ctx) {
			if b.ID == id {
				if b.Chat == 0 {
					return errors.New("the bot has no chat yet; open Connections")
				}
				c, err := a.botClient(ctx, id)
				if err != nil {
					return err
				}
				_, err = c.Send(ctx, b.Chat, text)
				return err
			}
		}
		return errors.New("that bot was removed")
	}
	return fmt.Errorf("unknown destination %q", target)
}

func (a *App) destinationRoutes() {
	s := a.Server
	s.Handle("GET /api/destinations", func(w http.ResponseWriter, r *http.Request) { server.WriteJSON(w, 200, a.destinations(r.Context())) })
	s.Handle("GET /api/telegram/bots", func(w http.ResponseWriter, r *http.Request) {
		list := a.bots(r.Context())
		if list == nil {
			list = []Bot{}
		}
		server.WriteJSON(w, 200, list)
	})
	s.Handle("POST /api/telegram/bots", a.addBot)
	s.Handle("POST /api/telegram/bots/{id}/detect", a.detectBotChat)
	s.Handle("DELETE /api/telegram/bots/{id}", a.removeBot)
	s.Handle("PUT /api/routines/{id}/settings", a.putRoutineSettings)
	s.Handle("GET /api/geocode", a.geocode)
}

func (a *App) addBot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Name  string `json:"name"`
		Token string `json:"token"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	tok := strings.TrimSpace(req.Token)
	user, err := (telegram.Bot{Token: tok, BaseURL: a.TelegramAPI}).Me(ctx)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "Telegram did not accept this token"})
		return
	}
	b := Bot{ID: importID()[:8], Name: strings.TrimSpace(req.Name), Username: user}
	if b.Name == "" {
		b.Name = user
	}
	if err := a.Vault.Set(ctx, "telegram.bot."+b.ID, tok); err != nil {
		server.WriteError(w, err)
		return
	}
	botsMu.Lock()
	err = a.saveBots(ctx, append(a.bots(ctx), b))
	botsMu.Unlock()
	if err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(ctx, "telegram.bot.added", "human:owner", map[string]string{"id": b.ID, "username": user})
	server.WriteJSON(w, 200, b)
}

// detectBotChat finds the chat of the last message someone sent the bot,
// so the owner only has to write to it (or add it to a group) once.
func (a *App) detectBotChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	c, err := a.botClient(ctx, id)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 404, Msg: err.Error()})
		return
	}
	var found *telegram.Message
	pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	c.Poll(pctx, 0, func(u telegram.Update) {
		if u.Message != nil {
			found = u.Message
		}
		cancel()
	})
	cancel()
	if found == nil {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "send any message to the bot (or add it to the group) and try again"})
		return
	}
	botsMu.Lock()
	defer botsMu.Unlock()
	list := a.bots(ctx)
	for i := range list {
		if list[i].ID == id {
			list[i].Chat = found.Chat.ID
			list[i].ChatName = found.From.FirstName
			if found.Chat.Title != "" {
				list[i].ChatName = found.Chat.Title
			}
			a.saveBots(ctx, list)
			a.Events.Append(ctx, "telegram.bot.chat", "human:owner", map[string]any{"id": id, "chat": found.Chat.ID})
			server.WriteJSON(w, 200, list[i])
			return
		}
	}
	server.WriteError(w, server.StatusError{Status: 404, Msg: "no such bot"})
}

func (a *App) removeBot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	botsMu.Lock()
	list := a.bots(ctx)
	kept := list[:0]
	for _, b := range list {
		if b.ID != id {
			kept = append(kept, b)
		}
	}
	a.saveBots(ctx, kept)
	botsMu.Unlock()
	a.Vault.Delete(ctx, "telegram.bot."+id)
	server.WriteJSON(w, 200, map[string]string{"removed": id})
}

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// putRoutineSettings saves when a routine runs and the values of its
// parameters, checked against the manifest before anything is stored.
// tooOften reports whether a schedule fires twice within five minutes
// anywhere in the next day.
func tooOften(s cron.Schedule) bool {
	t := s.Next(time.Now())
	for range 300 {
		n := s.Next(t)
		if n.Sub(t) < 5*time.Minute {
			return true
		}
		if n.Sub(time.Now()) > 24*time.Hour {
			return false
		}
		t = n
	}
	return false
}

func (a *App) putRoutineSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rt, err := a.Store.Routine(ctx, r.PathValue("id"))
	if err != nil {
		server.WriteError(w, notFound(err))
		return
	}
	var req struct {
		Schedule   string         `json:"schedule"`
		Params     map[string]any `json:"params"`
		WatchEvery string         `json:"watch_every"`
		Model      string         `json:"model"`
		Effort     string         `json:"effort"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	req.Schedule = strings.Join(strings.Fields(req.Schedule), " ")
	if req.Schedule != "" {
		sched, err := cronParser.Parse(req.Schedule)
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "that schedule is not valid: " + err.Error()})
			return
		}
		if tooOften(sched) {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "a routine can run at most every 5 minutes"})
			return
		}
	}
	resolved, err := rt.Body.Manifest.ResolveParams(req.Params)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	known := map[string]bool{}
	for _, d := range a.destinations(ctx) {
		known[d.ID] = true
	}
	for _, d := range rt.Body.Manifest.Destinations(resolved) {
		if !known[d] {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "unknown destination " + d})
			return
		}
	}
	if req.Schedule == rt.Body.Manifest.Schedule {
		req.Schedule = ""
	}
	if req.WatchEvery != "" {
		d, err := time.ParseDuration(req.WatchEvery)
		if err != nil || d < 5*time.Minute || d > 24*time.Hour || rt.Body.Manifest.Watch == nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "checks happen every 5 minutes to 24 hours, and only for routines that watch"})
			return
		}
	}
	if req.Model == Auto {
		req.Model = ""
	}
	if !a.usableModel(ctx, req.Model) {
		server.WriteError(w, server.StatusError{Status: 400, Msg: req.Model + " is not among your models"})
		return
	}
	if req.Effort == Auto {
		req.Effort = ""
	}
	if !llm.ValidEffort(req.Effort) {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "effort is low, medium, high or max"})
		return
	}
	settings := store.Settings{Schedule: req.Schedule, Params: req.Params, WatchEvery: req.WatchEvery, Model: req.Model, Effort: req.Effort}
	if err := a.Store.SetRoutineSettings(ctx, rt.ID, settings); err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(ctx, "routine.settings", "human:owner", map[string]any{"routine": rt.ID, "schedule": req.Schedule, "params": req.Params})
	a.Scheduler.Changed(ctx, rt.ID)
	rt, _ = a.Store.Routine(ctx, rt.ID)
	server.WriteJSON(w, 200, a.summary(ctx, rt))
}

// geocode finds places by name for location parameters, through
// Open-Meteo's free geocoding (only the typed name leaves the machine).
func (a *App) geocode(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 {
		server.WriteJSON(w, 200, []any{})
		return
	}
	lang := strings.SplitN(a.Settings(r.Context()).Locale, "-", 2)[0]
	base := a.GeocodeAPI
	if base == "" {
		base = "https://geocoding-api.open-meteo.com"
	}
	u := base + "/v1/search?count=6&format=json&language=" + url.QueryEscape(lang) + "&name=" + url.QueryEscape(q)
	req, _ := http.NewRequestWithContext(r.Context(), "GET", u, nil)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 502, Msg: "could not search places right now"})
		return
	}
	defer resp.Body.Close()
	var res struct {
		Results []struct {
			Name      string  `json:"name"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Timezone  string  `json:"timezone"`
			Country   string  `json:"country"`
			Admin1    string  `json:"admin1"`
		} `json:"results"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&res)
	out := []map[string]any{}
	for _, p := range res.Results {
		name := p.Name
		if p.Admin1 != "" && p.Admin1 != p.Name {
			name += ", " + p.Admin1
		}
		out = append(out, map[string]any{"name": name, "latitude": p.Latitude, "longitude": p.Longitude, "timezone": p.Timezone, "country": p.Country})
	}
	server.WriteJSON(w, 200, out)
}
