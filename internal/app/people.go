package app

import (
	"errors"
	"net/http"
	"strings"

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
	server.WriteJSON(w, 200, p)
}

func (a *App) removePerson(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	if err := a.People.Remove(ctx, id); err != nil {
		server.WriteError(w, peopleError(err))
		return
	}
	// Their accounts go with them.
	pctx := people.With(ctx, id)
	for _, name := range []string{"mail.password", "calendar.feeds"} {
		a.Vault.Delete(ctx, personal(pctx, name))
	}
	a.Events.Append(ctx, "person.removed", "human:owner", map[string]any{"id": id})
	server.WriteJSON(w, 200, map[string]string{"removed": id})
}

// personConnection sets up a member's own mail or calendar; runs for them
// use these and never the owner's.
func (a *App) personConnection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, err := a.People.Get(ctx, r.PathValue("id"))
	if err != nil || p.ID == people.OwnerID {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such person; set up your own accounts in Connections"})
		return
	}
	var req map[string]string
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	pctx := people.With(ctx, p.ID)
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
		err = a.Vault.Set(ctx, personal(pctx, "mail.password"), strings.ReplaceAll(req["password"], " ", ""))
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
	a.Events.Append(ctx, "person.connected", "human:owner", map[string]string{"id": p.ID, "kind": r.PathValue("kind")})
	server.WriteJSON(w, 200, map[string]string{"ok": "true"})
}

func peopleError(err error) error {
	if errors.Is(err, people.ErrUnknown) {
		return server.StatusError{Status: 404, Msg: err.Error()}
	}
	return server.StatusError{Status: 400, Msg: err.Error()}
}
