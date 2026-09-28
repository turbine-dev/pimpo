package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/denerFernandes/pimpo/internal/capability"
	"github.com/denerFernandes/pimpo/internal/connector"
	"github.com/denerFernandes/pimpo/internal/i18n"
	"github.com/denerFernandes/pimpo/internal/people"
	"github.com/denerFernandes/pimpo/internal/server"
)

// Reminders are one-shot messages at a time: "in 30 minutes, remind me to
// check the deploy". Unlike a routine they run once and are gone. Setting
// one only schedules a message to the person who asked, so it is a notify,
// like telling them now; cancelling one is a change and asks first.

type reminder struct {
	ID      string    `json:"id"`
	At      time.Time `json:"at"`
	Text    string    `json:"text"`
	Person  string    `json:"person,omitempty"`
	Created time.Time `json:"created"`
}

const remindersKey = "reminders"

func init() {
	capability.Register(capability.Spec{Name: "reminder.set", Risk: capability.Notify, Signature: "reminder.set({at, text}) or reminder.set({in, text})",
		Returns: "{id, at}; at is an ISO 8601 time with its offset, in is a delay such as 30m, 2h or 1d; the message goes to the person who asked, once",
		Schema:  `{"type":"object","required":["text"],"properties":{"at":{"type":"string","description":"ISO 8601 time with offset, e.g. 2026-09-29T09:00:00-03:00"},"in":{"type":"string","description":"delay from now: 30m, 2h, 1d"},"text":{"type":"string","description":"what to remind, as it should be read"}}}`})
	capability.Register(capability.Spec{Name: "reminder.list", Risk: capability.Read, Signature: "reminder.list()", Returns: "[{id, at, text}] the pending reminders, soonest first"})
	capability.Register(capability.Spec{Name: "reminder.cancel", Risk: capability.Reversible, Signature: "reminder.cancel({id})", Returns: "{ok}",
		Schema: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`})
}

var remindersMu sync.Mutex

func (a *App) reminders(ctx context.Context) []reminder {
	raw, _ := a.Events.Get(ctx, remindersKey)
	var list []reminder
	json.Unmarshal([]byte(raw), &list)
	return list
}

func (a *App) saveReminders(ctx context.Context, list []reminder) error {
	sort.Slice(list, func(i, j int) bool { return list[i].At.Before(list[j].At) })
	if list == nil {
		list = []reminder{}
	}
	b, _ := json.Marshal(list)
	return a.Events.Put(ctx, remindersKey, string(b))
}

// parseDelay reads 30m, 2h, 1d, 1h30m.
func parseDelay(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if strings.HasSuffix(s, "d") {
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err == nil && n > 0 {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%q is not a delay like 30m, 2h or 1d", s)
	}
	return d, nil
}

// reminderCap is reminder.set, reminder.list and reminder.cancel.
type reminderCap struct{ a *App }

func (reminderCap) Capabilities() []string {
	return []string{"reminder.set", "reminder.list", "reminder.cancel"}
}

func (c reminderCap) Call(ctx context.Context, name, _ string, args any) (any, error) {
	remindersMu.Lock()
	defer remindersMu.Unlock()
	person := people.Norm(people.From(ctx))
	list := c.a.reminders(ctx)
	switch name {
	case "reminder.list":
		out := []map[string]any{}
		for _, r := range list {
			if people.Norm(r.Person) == person {
				out = append(out, map[string]any{"id": r.ID, "at": r.At.Format(time.RFC3339), "text": r.Text})
			}
		}
		return out, nil
	case "reminder.cancel":
		var in struct {
			ID string `json:"id"`
		}
		if err := connector.Args(args, &in); err != nil {
			return nil, err
		}
		for i, r := range list {
			if r.ID == in.ID && people.Norm(r.Person) == person {
				list = append(list[:i], list[i+1:]...)
				c.a.Events.Append(ctx, "reminder.cancelled", "system", map[string]string{"id": r.ID})
				return map[string]any{"ok": true}, c.a.saveReminders(ctx, list)
			}
		}
		return nil, fmt.Errorf("no reminder %q", in.ID)
	}
	var in struct {
		At   string `json:"at"`
		In   string `json:"in"`
		Text string `json:"text"`
	}
	if err := connector.Args(args, &in); err != nil {
		return nil, err
	}
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" {
		return nil, errors.New("text is empty")
	}
	now := time.Now()
	var at time.Time
	switch {
	case in.In != "":
		d, err := parseDelay(in.In)
		if err != nil {
			return nil, err
		}
		at = now.Add(d)
	case in.At != "":
		t, err := time.Parse(time.RFC3339, in.At)
		if err != nil {
			return nil, fmt.Errorf("at must be ISO 8601 with its offset, e.g. 2026-09-29T09:00:00-03:00: %w", err)
		}
		at = t
	default:
		return nil, errors.New("say when: at (a time) or in (a delay)")
	}
	if at.Before(now.Add(-time.Minute)) {
		return nil, fmt.Errorf("%s has already passed", at.Format(time.RFC3339))
	}
	if at.After(now.AddDate(1, 0, 0)) {
		return nil, errors.New("reminders go up to a year ahead; use a routine for what repeats")
	}
	if len(list) >= 500 {
		return nil, errors.New("too many pending reminders")
	}
	b := make([]byte, 6)
	rand.Read(b)
	r := reminder{ID: "rem-" + hex.EncodeToString(b), At: at, Text: in.Text, Person: person, Created: now}
	if err := c.a.saveReminders(ctx, append(list, r)); err != nil {
		return nil, err
	}
	c.a.Events.Append(ctx, "reminder.set", "system", map[string]any{"id": r.ID, "at": r.At})
	return map[string]any{"id": r.ID, "at": r.At.In(loadZone(c.a.Settings(ctx).Zone)).Format(time.RFC3339)}, nil
}

// reminderLoop sends reminders when they are due. One missed while Pimpo
// was closed goes out when it starts again, saying it is late.
func (a *App) reminderLoop(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		a.sendDue(ctx, time.Now())
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
}

func (a *App) sendDue(ctx context.Context, now time.Time) {
	remindersMu.Lock()
	list := a.reminders(ctx)
	var due, keep []reminder
	for _, r := range list {
		if r.At.After(now) {
			keep = append(keep, r)
		} else {
			due = append(due, r)
		}
	}
	if len(due) > 0 {
		a.saveReminders(ctx, keep)
	}
	remindersMu.Unlock()
	for _, r := range due {
		pctx := people.With(ctx, r.Person)
		text := i18n.T(pctx, "msg.reminder", "text", r.Text)
		if now.Sub(r.At) > 5*time.Minute {
			text = i18n.T(pctx, "msg.reminder.late", "text", r.Text, "when", r.At.In(loadZone(a.Settings(ctx).Zone)).Format("02/01 15:04"))
		}
		_, err := notifyCap{a}.Call(pctx, "notify.send", "", map[string]any{"text": text})
		a.Events.Append(ctx, "reminder.sent", "system", map[string]any{"id": r.ID, "error": errText(err)})
	}
}

func (a *App) reminderRoutes() {
	a.Server.Handle("GET /api/reminders", func(w http.ResponseWriter, r *http.Request) {
		out, err := reminderCap{a}.Call(r.Context(), "reminder.list", "", map[string]any{})
		if err != nil {
			server.WriteError(w, err)
			return
		}
		server.WriteJSON(w, 200, out)
	})
	a.Server.Handle("DELETE /api/reminders/{id}", func(w http.ResponseWriter, r *http.Request) {
		if _, err := (reminderCap{a}).Call(r.Context(), "reminder.cancel", "", map[string]any{"id": r.PathValue("id")}); err != nil {
			server.WriteError(w, server.StatusError{Status: 404, Msg: err.Error()})
			return
		}
		server.WriteJSON(w, 200, map[string]bool{"ok": true})
	})
}
