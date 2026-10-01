package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/store"
)

// Who may use each route. Every person signs in as themselves and every
// route acts only for them: the owner administers the house (people,
// connections, models, rules, backups) but sees nothing of anyone else's
// routines, memory, conversations or activity, and nobody sees the
// owner's. A route not listed here is the owner's alone; a test fails if
// a route is added without deciding.

// guestRoutes are what a guest may use: ask, and see their own answers.
// What a guest's request would change waits for their responsible person.
var guestRoutes = routeSet(
	"GET /api/state",
	"GET /api/capabilities",
	"GET /api/chats", "GET /api/chats/search", "GET /api/chats/{id}", "POST /api/chats", "POST /api/chats/{id}/messages", "DELETE /api/chats/{id}",
	"GET /api/explorations", "GET /api/explorations/{id}", "POST /api/explorations",
	"GET /api/questions", "POST /api/questions/{id}/answer",
	"GET /api/needs",
	"GET /api/events", "GET /api/events/verify", "GET /api/ws",
	"POST /api/voice/transcribe", "POST /api/speak",
	"GET /api/media", "GET /api/media/{id}",
	"GET /api/assistants",
	"GET /api/models/retired",
	"GET /api/memory", "POST /api/memory", "DELETE /api/memory/{id}", "POST /api/memory/{id}/confirm", "GET /api/memory/search",
	"GET /api/memory/sources", "POST /api/memory/sources/forget",
	"GET /api/passkeys", "POST /api/passkeys/begin", "POST /api/passkeys/finish", "DELETE /api/passkeys/{id}",
	"GET /api/me/devices", "DELETE /api/me/devices/{id}",
	"GET /api/me/limits",
	"GET /api/dashboards", "GET /api/dashboards/{id}/widgets", "GET /api/widgets", "GET /api/widgets/{id}",
)

// memberRoutes add to a guest's what a member manages for themselves.
var memberRoutes = routeSet(
	"POST /api/explorations/{id}/{action}", "POST /api/chats/{id}/turns/{exp}/do",
	"GET /api/routines", "GET /api/routines/{id}", "POST /api/routines/{id}/{action}", "PUT /api/routines/{id}/settings",
	"GET /api/routines/{id}/webhook", "POST /api/routines/{id}/webhook/{action}",
	"GET /api/routines/{id}/push", "POST /api/routines/{id}/push/{action}", "GET /api/routines/{id}/github", "POST /api/routines/{id}/github/{action}",
	"GET /api/runs",
	"GET /api/receipts", "POST /api/actions/{id}/undo",
	"GET /api/approvals", "POST /api/approvals/{id}/{answer}",
	"GET /api/grants", "DELETE /api/grants/{id}",
	"GET /api/reminders", "DELETE /api/reminders/{id}",
	"GET /api/cost",
	"GET /api/jobs", "GET /api/jobs/{id}", "POST /api/jobs", "POST /api/jobs/{id}/start", "POST /api/jobs/{id}/stop", "POST /api/jobs/{id}/follow",
	"GET /api/companies", "POST /api/companies", "POST /api/companies/import", "GET /api/companies/{id}", "PUT /api/companies/{id}", "DELETE /api/companies/{id}", "GET /api/companies/{id}/export",
	"PUT /api/companies/{id}/departments/{part}", "DELETE /api/companies/{id}/departments/{part}", "PUT /api/companies/{id}/roles/{part}", "DELETE /api/companies/{id}/roles/{part}",
	"PUT /api/companies/{id}/members/{part}", "DELETE /api/companies/{id}/members/{part}", "GET /api/companies/{id}/members/{part}/preview",
	"GET /api/companies/{id}/tasks", "POST /api/companies/{id}/tasks", "POST /api/companies/{id}/tasks/{part}/drop",
	"GET /api/companies/{id}/questions", "POST /api/companies/{id}/questions/{part}/answer",
	"GET /api/companies/{id}/work", "POST /api/companies/{id}/members/{part}/work", "POST /api/companies/{id}/work/{part}/stop",
	"PUT /api/companies/{id}/agent-routines/{part}", "DELETE /api/companies/{id}/agent-routines/{part}", "POST /api/companies/{id}/agent-routines/{part}/run",
	"PUT /api/companies/{id}/members/{part}/routines/{routine}", "DELETE /api/companies/{id}/members/{part}/routines/{routine}",
	"PUT /api/companies/{id}/contexts/{part}", "DELETE /api/companies/{id}/contexts/{part}", "PUT /api/companies/{id}/rules/{part}", "DELETE /api/companies/{id}/rules/{part}",
	"GET /api/progress",
	"GET /api/phone", "POST /api/phone/shares", "POST /api/phone/key", "POST /api/phone/widgets", "POST /api/phone/places", "DELETE /api/phone/places/{name}", "GET /api/phone/photos/{id}",
	"GET /api/gallery", "POST /api/gallery/{id}/install",
	"GET /api/memory/organized",
	"GET /api/lessons", "POST /api/lessons/{id}/{action}",
	"GET /api/destinations", "GET /api/geocode",
	"POST /api/open",
	"PUT /api/people/{id}/connections/{kind}",
	"GET /api/history", "POST /api/history/{id}/undo",
	"GET /api/credentials", "GET /api/credentials/{id}", "POST /api/credentials/{id}", "DELETE /api/credentials/{id}",
	"GET /api/password-managers", "PUT /api/password-managers/{kind}", "DELETE /api/password-managers/{kind}", "POST /api/password-managers/{kind}/test", "POST /api/secrets/check",
	"POST /api/dashboards", "PUT /api/dashboards/{id}", "DELETE /api/dashboards/{id}",
	"PUT /api/widgets/{id}", "DELETE /api/widgets/{id}", "POST /api/widgets/{id}/refresh",
)

// ownerOnlyRoutes are the owner's and say so; listing them keeps the
// decision explicit for every route.
var ownerOnlyRoutes = routeSet(
	"GET /api/system", "GET /api/rules", "GET /api/setup", "POST /api/rules/compile", "POST /api/rules/test", "POST /api/setup/done", "PUT /api/rules", "PUT /api/rules/preset",
	"DELETE /api/people/{id}", "GET /api/people", "POST /api/people", "PUT /api/people/{id}",
	"GET /api/protection", "POST /api/guard/check", "POST /api/protection/{id}/ignore",
	"DELETE /api/skills/{id}", "GET /api/skills", "POST /api/skills/install", "POST /api/skills/preview", "PUT /api/skills/{id}",
	"POST /api/suggestions", "GET /api/suggestions", "POST /api/suggestions/{id}/{action}",
	"DELETE /api/spotify", "GET /api/spotify", "POST /api/oauth/spotify/start",
	"DELETE /api/devices/{id}", "GET /api/pairing", "POST /api/pairing",
	"DELETE /api/catalog/{kind}", "GET /api/catalog", "POST /api/catalog/{kind}/check", "PUT /api/catalog/{kind}",
	"DELETE /api/connections/{kind}", "GET /api/connections", "GET /api/report", "GET /api/settings", "PUT /api/budget", "PUT /api/connections/{kind}", "PUT /api/settings",
	"POST /api/doctor", "POST /api/setup/model",
	"POST /api/backup/export", "POST /api/backup/import", "POST /api/connectors/install", "POST /api/connectors/reload",
	"GET /api/repo", "POST /api/repo/apply/{id}", "POST /api/repo/export", "POST /api/repo/pull", "POST /api/repo/push", "PUT /api/repo",
	"DELETE /api/snapshots/restore", "GET /api/snapshots", "POST /api/snapshots", "POST /api/snapshots/restore",
	"POST /api/migrate/apply", "POST /api/migrate/preview",
	"POST /api/channel/button", "POST /api/channel/message", "PUT /api/channel/webhook",
	"GET /api/models", "GET /api/models/catalog/{provider}", "GET /api/models/detect", "GET /api/models/opencode", "POST /api/models/test", "PUT /api/models/keys/{provider}",
	"GET /api/remote", "POST /api/remote/{kind}/{switch}",
	"POST /api/memory/organize", "POST /api/memory-versions/{hash}/restore",
	"DELETE /api/backup/cloud", "GET /api/backup/cloud", "GET /api/backup/cloud/files", "POST /api/backup/cloud/restore", "POST /api/backup/cloud/run", "PUT /api/backup/cloud",
	"DELETE /api/connectors/{name}", "GET /api/connectors/registry", "POST /api/connectors/add", "POST /api/connectors/probe",
	"DELETE /api/local/ollama/{model...}", "DELETE /api/local/{id}", "GET /api/local", "GET /api/voice", "POST /api/local/install/{id}", "POST /api/local/jobs/{id}/cancel", "POST /api/local/ollama/pull", "POST /api/local/sample", "PUT /api/voice/elevenlabs-key",
	"DELETE /api/telegram/bots/{id}", "GET /api/telegram/bots", "POST /api/telegram/bots", "POST /api/telegram/bots/{id}/detect",
	"POST /api/connectors/openapi/add", "POST /api/connectors/openapi/preview",
	"POST /api/learn",
	"POST /api/routines/{id}/publish", "POST /api/routines/{id}/update",
	"POST /api/oauth/google/start",
	"GET /api/push/gmail", "PUT /api/push/gmail",
	"PUT /api/assistants/{id}", "DELETE /api/assistants/{id}",
	"POST /api/browser/login",
	"PUT /api/account",
	"PUT /api/people/{id}/limits",
)

func routeSet(patterns ...string) map[string]bool {
	m := map[string]bool{}
	for _, p := range patterns {
		m[p] = true
	}
	return m
}

func (a *App) allow(pattern, person string) bool {
	switch a.People.Role(context.Background(), person) {
	case people.Member:
		return memberRoutes[pattern] || guestRoutes[pattern]
	case people.Guest:
		return guestRoutes[pattern]
	}
	return false
}

// eventVisible says whether an event is that person's to see whole: what
// they did, what was sent to them, what was done for them. Everyone else
// only learns that something changed.
func (a *App) eventVisible(person string, e event.Event) bool {
	person = people.Norm(person)
	if who, ok := strings.CutPrefix(e.Actor, "human:"); ok {
		return people.Norm(who) == person
	}
	if dev, ok := strings.CutPrefix(e.Actor, "device:"); ok {
		for _, d := range a.devices(context.Background()) {
			if d.ID == dev {
				return people.Norm(d.Person) == person
			}
		}
		return false
	}
	var d struct {
		To     *string `json:"to"`
		Person *string `json:"person"`
	}
	json.Unmarshal(e.Data, &d)
	switch {
	case d.To != nil:
		return people.Norm(*d.To) == person
	case d.Person != nil:
		return people.Norm(*d.Person) == person
	}
	return false
}

// mine says whether something kept for person belongs to whoever ctx acts
// for. Empty is the owner, as it was before people had logins.
func mine(ctx context.Context, person string) bool {
	return people.Norm(person) == people.Norm(people.From(ctx))
}

// myRoutine is a routine of the person asking; anyone else's does not
// exist for them.
func (a *App) myRoutine(ctx context.Context, id string) (store.Routine, error) {
	rt, err := a.Store.Routine(ctx, id)
	if err != nil {
		return rt, err
	}
	if !mine(ctx, rt.Person) {
		return store.Routine{}, store.ErrNotFound
	}
	return rt, nil
}

func (a *App) myRoutines(ctx context.Context) ([]store.Routine, error) {
	all, err := a.Store.Routines(ctx)
	out := all[:0]
	for _, r := range all {
		if mine(ctx, r.Person) {
			out = append(out, r)
		}
	}
	return out, err
}

func (a *App) myExploration(ctx context.Context, id string) (store.Exploration, error) {
	e, err := a.Store.Exploration(ctx, id)
	if err != nil {
		return e, err
	}
	if !mine(ctx, e.Person) {
		return store.Exploration{}, store.ErrNotFound
	}
	return e, nil
}

func (a *App) myExplorations(ctx context.Context, states ...string) ([]store.Exploration, error) {
	all, err := a.Store.Explorations(ctx, states...)
	out := all[:0]
	for _, e := range all {
		if mine(ctx, e.Person) {
			out = append(out, e)
		}
	}
	return out, err
}

// roleOf is what the person asking may do, for the interface to show.
func (a *App) roleOf(ctx context.Context) people.Role {
	if p := people.From(ctx); p != people.OwnerID {
		return a.People.Role(ctx, p)
	}
	return people.Owner
}

// The administrator's account: made on the first visit, with their name,
// before anything else. Their passkeys carry it.
const adminKey = "admin.account"

type adminAccount struct {
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
}

func (a *App) admin(ctx context.Context) (adminAccount, bool) {
	raw, _ := a.Events.Get(ctx, adminKey)
	var acc adminAccount
	return acc, json.Unmarshal([]byte(raw), &acc) == nil && acc.Name != ""
}

func (a *App) accountRoutes() {
	a.Server.Handle("PUT /api/account", func(w http.ResponseWriter, r *http.Request) {
		if !ownerOnly(w, r) {
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		name := clip(strings.Join(strings.Fields(req.Name), " "), 60)
		if name == "" {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "say your name"})
			return
		}
		acc, existed := a.admin(r.Context())
		acc.Name = name
		if !existed {
			acc.Created = time.Now().UTC()
		}
		b, _ := json.Marshal(acc)
		a.Events.Put(r.Context(), adminKey, string(b))
		kind := "admin.renamed"
		if !existed {
			kind = "admin.created"
		}
		a.Events.Append(r.Context(), kind, "human:owner", map[string]string{"person": people.OwnerID})
		server.WriteJSON(w, 200, acc)
	})
}

// nameOf is how the person asking is called.
func (a *App) nameOf(ctx context.Context) string {
	p := people.From(ctx)
	if p == people.OwnerID {
		acc, _ := a.admin(ctx)
		return acc.Name
	}
	if q, err := a.People.Get(ctx, p); err == nil {
		return q.Name
	}
	return ""
}

// hasAdmin says whether the administrator's account exists; the interface
// asks for it on the first visit.
func (a *App) hasAdmin(ctx context.Context) bool {
	_, ok := a.admin(ctx)
	return ok
}
