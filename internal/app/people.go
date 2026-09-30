package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/turbine-dev/pimpo/internal/memory"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
)

func (a *App) peopleRoutes() {
	s := a.Server
	s.Handle("GET /api/people", a.listPeople)
	s.Handle("POST /api/people", a.addPerson)
	s.Handle("PUT /api/people/{id}", a.updatePerson)
	s.Handle("DELETE /api/people/{id}", a.removePerson)
	s.Handle("PUT /api/people/{id}/connections/{kind}", a.personConnection)
}

type personView struct {
	people.Person
	Mail     bool `json:"mail"`
	Calendar bool `json:"calendar"`
}

func (a *App) listPeople(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// An invite that ran out is replaced, so the code shown always works.
	a.People.RenewInvites(ctx)
	list, err := a.People.List(ctx)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	out := make([]personView, 0, len(list))
	for _, p := range list {
		pctx := people.With(ctx, p.ID)
		addr, _ := a.Events.Get(pctx, personal(pctx, "mail.addr"))
		_, cal := a.Vault.Get(pctx, personal(pctx, "calendar.feeds"))
		out = append(out, personView{Person: p, Mail: addr != "", Calendar: cal == nil})
	}
	server.WriteJSON(w, 200, out)
}

type personRequest struct {
	Name        string      `json:"name"`
	Role        people.Role `json:"role"`
	Responsible string      `json:"responsible"`
}

func (a *App) addPerson(w http.ResponseWriter, r *http.Request) {
	var req personRequest
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	p, err := a.People.Add(r.Context(), req.Name, req.Role, req.Responsible)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	a.Events.Append(r.Context(), "person.added", "human:owner", map[string]any{"id": p.ID, "role": p.Role})
	server.WriteJSON(w, 200, p)
}

func (a *App) updatePerson(w http.ResponseWriter, r *http.Request) {
	var req personRequest
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	p, err := a.People.Update(r.Context(), r.PathValue("id"), req.Role, req.Responsible)
	if err != nil {
		server.WriteError(w, peopleError(err))
		return
	}
	a.Events.Append(r.Context(), "person.changed", "human:owner", map[string]any{"id": p.ID, "role": p.Role, "responsible": p.Responsible})
	// The person changed learns of it in their own activity: the owner
	// administers roles, but never quietly.
	a.Events.Append(r.Context(), "person.role_changed", "system", map[string]any{"to": p.ID, "role": p.Role, "responsible": p.Responsible})
	server.WriteJSON(w, 200, p)
}

func (a *App) removePerson(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	if err := a.People.Remove(ctx, id); err != nil {
		server.WriteError(w, peopleError(err))
		return
	}
	a.forgetDevicesOf(ctx, id)
	a.forgetPerson(ctx, id)
	a.Events.Append(ctx, "person.removed", "human:owner", map[string]any{"id": id})
	server.WriteJSON(w, 200, map[string]string{"removed": id})
}

// forgetPerson deletes what a removed person kept: their accounts, memory
// (and the house facts they shared), chats, explorations, routines, jobs,
// reminders, places and queued emails. Their id is never given again, so
// anything missed here stays unreachable.
func (a *App) forgetPerson(ctx context.Context, id string) {
	prefix := "person." + id + "."
	if names, err := a.Vault.Names(ctx); err == nil {
		for _, n := range names {
			if strings.HasPrefix(n, prefix) {
				a.Vault.Delete(ctx, n)
			}
		}
	}
	// Ids are letters and digits only, so they need no escaping in LIKE.
	a.Events.DB().ExecContext(ctx, `DELETE FROM kv WHERE key LIKE ? OR key = ? OR key = ? OR key LIKE ?`,
		prefix+"%", placesKey+"."+id, organizedKeyFor(id), "conv.%."+id)
	if a.Memory != nil {
		facts, _ := a.Memory.List()
		var drop []string
		for _, f := range facts {
			if f.Person == id || memory.SharedBy(f) == id {
				drop = append(drop, f.ID)
			}
		}
		a.Memory.Drop(drop, "remove: "+memory.ForPrefix+id)
	}
	if chats, err := a.Store.Chats(ctx, id); err == nil {
		for _, c := range chats {
			a.Store.DeleteChat(ctx, c.ID)
		}
	}
	routines, _ := a.Store.ForgetPerson(ctx, id)
	for _, rid := range routines {
		a.Scheduler.Changed(ctx, rid)
	}
	a.forgetJobsOf(ctx, id)
	remindersMu.Lock()
	kept := []reminder{}
	for _, r := range a.reminders(ctx) {
		if people.Norm(r.Person) != id {
			kept = append(kept, r)
		}
	}
	a.saveReminders(ctx, kept)
	remindersMu.Unlock()
	if a.Outbox != nil {
		a.Outbox.CancelFor(ctx, id)
	}
}

// forgetJobsOf stops and deletes a removed person's jobs.
func (a *App) forgetJobsOf(ctx context.Context, person string) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	ids := a.jobIDs(ctx)
	kept := []string{}
	for _, jid := range ids {
		if j, ok := a.job(ctx, jid); ok && people.Norm(j.Person) == person {
			// A running part finds its job gone and stops.
			a.Events.DB().ExecContext(ctx, `DELETE FROM kv WHERE key = ?`, jobKey(jid))
			continue
		}
		kept = append(kept, jid)
	}
	b, _ := json.Marshal(kept)
	a.Events.Put(ctx, jobsKey, string(b))
}

// personConnection sets up a member's own mail or calendar; runs for them
// use these and never the owner's.
func (a *App) personConnection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// A person sets up their own accounts; the owner may set them up for
	// someone who has never signed in, but never reads them back, and
	// never replaces what a person keeps for themselves.
	me := people.From(ctx)
	if me != people.OwnerID && me != r.PathValue("id") {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such person"})
		return
	}
	p, err := a.People.Get(ctx, r.PathValue("id"))
	if err != nil || p.ID == people.OwnerID {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such person; set up your own accounts in Connections"})
		return
	}
	pctx := people.With(ctx, p.ID)
	if me == people.OwnerID && a.signedIn(ctx, p.ID) {
		server.WriteError(w, server.StatusError{Status: 403, Msg: p.Name + " signs in on their own now; only they can change their accounts"})
		return
	}
	var req map[string]string
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	switch r.PathValue("kind") {
	case "mail":
		if req["user"] == "" || req["password"] == "" {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "email address and app password are required"})
			return
		}
		addr := req["addr"]
		if addr == "" {
			addr = "imap.gmail.com:993"
		}
		a.Events.Put(ctx, personal(pctx, "mail.addr"), addr)
		a.Events.Put(ctx, personal(pctx, "mail.user"), strings.TrimSpace(req["user"]))
		err = a.Vault.Set(ctx, personal(pctx, "mail.password"), appPassword(req["password"]))
	case "calendar":
		var urls []string
		for _, l := range strings.Split(req["feeds"], "\n") {
			if l = strings.TrimSpace(l); l == "" {
				continue
			}
			if !strings.HasPrefix(l, "https://") {
				server.WriteError(w, server.StatusError{Status: 400, Msg: "calendar links must start with https://"})
				return
			}
			urls = append(urls, l)
		}
		err = a.Vault.Set(ctx, personal(pctx, "calendar.feeds"), strings.Join(urls, "\n"))
	default:
		server.WriteError(w, server.StatusError{Status: 404, Msg: "unknown connection"})
		return
	}
	if err != nil {
		server.WriteError(w, err)
		return
	}
	if me != people.OwnerID {
		a.Events.Put(ctx, personal(pctx, "accounts.own"), "1")
	}
	a.Events.Append(ctx, "person.connected", actor(ctx), map[string]string{"id": p.ID, "kind": r.PathValue("kind")})
	server.WriteJSON(w, 200, map[string]string{"ok": "true"})
}

// signedIn says whether a person has used Pimpo on their own: a device
// they used, a passkey, or accounts they set up themselves. From then on
// their accounts are theirs alone to change.
func (a *App) signedIn(ctx context.Context, person string) bool {
	for _, d := range a.devices(ctx) {
		if people.Norm(d.Person) == person && !d.LastSeen.IsZero() {
			return true
		}
	}
	for _, k := range a.passkeys(ctx) {
		if people.Norm(k.Person) == person {
			return true
		}
	}
	own, _ := a.Events.Get(ctx, personal(people.With(ctx, person), "accounts.own"))
	return own != ""
}

func peopleError(err error) error {
	if errors.Is(err, people.ErrUnknown) {
		return server.StatusError{Status: 404, Msg: err.Error()}
	}
	return server.StatusError{Status: 400, Msg: err.Error()}
}
