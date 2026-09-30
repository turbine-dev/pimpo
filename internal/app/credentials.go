package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/connector"
	"github.com/turbine-dev/pimpo/internal/connector/services"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/secretscan"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/store"
)

// When a run needs a password or key its person has not given, or one the
// service refused, Pimpo asks for it privately: a notice with a link to a
// one-time form in the app, which writes straight to the vault under that
// person's name. The value never passes through a chat, a channel, a
// model, an event or an answer of the API. Requests are made only here,
// from a connector's own error, never from a model's text.

type credentialRequest struct {
	ID          string    `json:"id"`
	Person      string    `json:"person"`
	Connector   string    `json:"connector"`
	Field       string    `json:"field"`
	Invalid     bool      `json:"invalid,omitempty"`
	Routine     string    `json:"routine,omitempty"`
	Exploration string    `json:"exploration,omitempty"`
	Asked       time.Time `json:"asked"`
	Expires     time.Time `json:"expires"`
}

// credentialView is a request as its person sees it, in their language.
type credentialView struct {
	credentialRequest
	Title       string `json:"title"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

const credentialsKey = "credential.requests"

// credentialTTL is how long a request's link works.
const credentialTTL = 7 * 24 * time.Hour

var credentialsMu sync.Mutex

func (a *App) credentialRequests(ctx context.Context) []credentialRequest {
	raw, _ := a.Events.Get(ctx, credentialsKey)
	var list []credentialRequest
	json.Unmarshal([]byte(raw), &list)
	now := time.Now()
	live := list[:0]
	for _, c := range list {
		if c.Expires.After(now) {
			live = append(live, c)
		}
	}
	return live
}

func (a *App) saveCredentialRequests(ctx context.Context, list []credentialRequest) {
	if list == nil {
		list = []credentialRequest{}
	}
	b, _ := json.Marshal(list)
	a.Events.Put(ctx, credentialsKey, string(b))
}

// credentialTarget is where a connector's field lives in the vault for
// whoever ctx acts for, and how to name it. house says the key is the
// administrator's for everyone, as external connectors' are.
type credentialTarget struct {
	secret, title, label string
	house                bool
}

func (a *App) credentialTarget(ctx context.Context, conn, field string) (credentialTarget, bool) {
	if k, ok := services.Get(conn); ok {
		k = localKind(ctx, k)
		for _, f := range k.Fields {
			if f.Name == field && f.Secret {
				return credentialTarget{secret: personal(ctx, catalogKey(conn, field)), title: k.Title, label: f.Label}, true
			}
		}
		return credentialTarget{}, false
	}
	if conn == "mail" && field == "password" {
		return credentialTarget{secret: personal(ctx, "mail.password"), title: i18n.T(ctx, "credential.mail.title"), label: i18n.T(ctx, "credential.mail.password")}, true
	}
	if c := a.externalConnector(conn); c != nil && (slices.Contains(c.Env, field) || slices.Contains(c.Headers, field)) {
		return credentialTarget{secret: "connector." + c.Name + "." + field, title: c.Name, label: field, house: true}, true
	}
	return credentialTarget{}, false
}

// missingCredential asks the person a run acts for for the key it lacks,
// and gives the run an error that says so and holds no value.
func (a *App) missingCredential(ctx context.Context, source string, m *connector.MissingCredential) error {
	person := people.Norm(people.From(ctx))
	t, ok := a.credentialTarget(ctx, m.Connector, m.Field)
	// A house key is the administrator's to give, and a guest keeps no
	// accounts: their runs only say what is missing.
	if !ok || t.house && person != people.OwnerID || a.People.Role(ctx, person) == people.Guest {
		return m
	}
	if _, err := a.askCredential(ctx, person, m, source); err != nil {
		return m
	}
	return errors.New(m.Error() + ". Pimpo asked the person privately, in a form of its own, for the " + t.label + "; do not ask for it in the conversation and never accept it as a message")
}

// askCredential opens a request, or finds the one already open for the
// same key, and sends its person the link.
func (a *App) askCredential(ctx context.Context, person string, m *connector.MissingCredential, source string) (credentialRequest, error) {
	credentialsMu.Lock()
	defer credentialsMu.Unlock()
	list := a.credentialRequests(ctx)
	routine, exploration := "", ""
	if id, ok := strings.CutPrefix(source, "routine:"); ok {
		routine, _, _ = strings.Cut(id, "#")
	} else if id, ok := strings.CutPrefix(source, "exploration:"); ok {
		exploration = id
	}
	for i, c := range list {
		if c.Person == person && c.Connector == m.Connector && c.Field == m.Field {
			// Asked already: the same link keeps working, without a new
			// notice every time a routine runs into it.
			c.Invalid = c.Invalid || m.Invalid
			if routine != "" || exploration != "" {
				c.Routine, c.Exploration = routine, exploration
			}
			list[i] = c
			a.saveCredentialRequests(ctx, list)
			return c, nil
		}
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return credentialRequest{}, err
	}
	now := time.Now().UTC()
	c := credentialRequest{ID: hex.EncodeToString(b), Person: person, Connector: m.Connector, Field: m.Field, Invalid: m.Invalid,
		Routine: routine, Exploration: exploration, Asked: now, Expires: now.Add(credentialTTL)}
	a.saveCredentialRequests(ctx, append(list, c))
	a.Events.Append(ctx, "credential.requested", "system", map[string]any{"id": c.ID, "person": person, "connector": c.Connector, "field": c.Field, "invalid": c.Invalid})
	v := a.credentialView(people.With(ctx, person), c)
	key := "msg.credential.ask"
	if c.Invalid {
		key = "msg.credential.refused"
	}
	a.Channel.Notify(ctx, explore.Notice{Text: i18n.T(ctx, key, "title", v.Title, "field", v.Label, "link", a.credentialLink(ctx, c.ID)), To: person})
	return c, nil
}

func (a *App) credentialView(ctx context.Context, c credentialRequest) credentialView {
	v := credentialView{credentialRequest: c}
	if t, ok := a.credentialTarget(ctx, c.Connector, c.Field); ok {
		v.Title, v.Label = t.title, t.label
	}
	key := "credential.needs"
	if c.Invalid {
		key = "credential.refused"
	}
	v.Description = i18n.T(ctx, key, "title", v.Title, "field", v.Label)
	return v
}

// credentialLink is the form's address, on the address the person can
// reach from where they read the notice when there is one.
func (a *App) credentialLink(ctx context.Context, id string) string {
	base, _ := a.Events.Get(ctx, "public_url")
	if base == "" {
		base = a.lanURL()
	}
	if base == "" && a.Explore != nil {
		base = a.Explore.BaseURL
	}
	return strings.TrimRight(base, "/") + "/credentials/" + id
}

// myCredentialRequest is an open request of the person asking; anyone
// else's does not exist for them.
func (a *App) myCredentialRequest(ctx context.Context, id string) (credentialRequest, bool) {
	for _, c := range a.credentialRequests(ctx) {
		if c.ID == id && mine(ctx, c.Person) {
			return c, true
		}
	}
	return credentialRequest{}, false
}

// myCredentialRequests are the open requests of the person asking, the
// newest first, in their language.
func (a *App) myCredentialRequests(ctx context.Context) []credentialView {
	out := []credentialView{}
	for _, c := range a.credentialRequests(ctx) {
		if mine(ctx, c.Person) {
			out = append(out, a.credentialView(ctx, c))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Asked.After(out[j].Asked) })
	return out
}

func (a *App) credentialRoutes() {
	a.Server.Handle("GET /api/credentials", func(w http.ResponseWriter, r *http.Request) {
		server.WriteJSON(w, 200, a.myCredentialRequests(r.Context()))
	})
	a.Server.Handle("GET /api/credentials/{id}", func(w http.ResponseWriter, r *http.Request) {
		c, ok := a.myCredentialRequest(r.Context(), r.PathValue("id"))
		if !ok {
			server.WriteError(w, server.StatusError{Status: 404, Msg: i18n.T(r.Context(), "credential.gone")})
			return
		}
		server.WriteJSON(w, 200, a.credentialView(r.Context(), c))
	})
	a.Server.Handle("POST /api/credentials/{id}", a.saveCredential)
	a.Server.Handle("DELETE /api/credentials/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if _, ok := a.myCredentialRequest(ctx, r.PathValue("id")); !ok {
			server.WriteError(w, server.StatusError{Status: 404, Msg: i18n.T(ctx, "credential.gone")})
			return
		}
		a.closeCredentialRequest(ctx, r.PathValue("id"))
		a.Events.Append(ctx, "credential.dismissed", actor(ctx), map[string]string{"id": r.PathValue("id"), "person": people.Norm(people.From(ctx))})
		server.WriteJSON(w, 200, map[string]string{"state": "dismissed"})
	})
}

func (a *App) closeCredentialRequest(ctx context.Context, id string) {
	credentialsMu.Lock()
	defer credentialsMu.Unlock()
	list := a.credentialRequests(ctx)
	kept := list[:0]
	for _, c := range list {
		if c.ID != id {
			kept = append(kept, c)
		}
	}
	a.saveCredentialRequests(ctx, kept)
}

// saveCredential writes the value straight to the vault, under the name
// Pimpo's code chose for that person's connector, and closes the request.
// The answer, and the event, say only that it was saved.
func (a *App) saveCredential(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, ok := a.myCredentialRequest(ctx, r.PathValue("id"))
	if !ok {
		server.WriteError(w, server.StatusError{Status: 404, Msg: i18n.T(ctx, "credential.gone")})
		return
	}
	var req struct {
		Value string `json:"value"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: i18n.T(ctx, "credential.unreadable")})
		return
	}
	value := strings.TrimSpace(req.Value)
	if c.Connector == "mail" {
		// App passwords are shown in groups; the spaces are not part of it.
		value = strings.ReplaceAll(value, " ", "")
	}
	if value == "" || len(value) > 16<<10 {
		server.WriteError(w, server.StatusError{Status: 400, Msg: i18n.T(ctx, "credential.empty")})
		return
	}
	pctx := people.With(ctx, c.Person)
	t, ok := a.credentialTarget(pctx, c.Connector, c.Field)
	if !ok {
		a.closeCredentialRequest(ctx, c.ID)
		server.WriteError(w, server.StatusError{Status: 404, Msg: i18n.T(ctx, "credential.gone")})
		return
	}
	if err := a.Vault.Set(ctx, t.secret, value); err != nil {
		server.WriteError(w, server.StatusError{Status: 500, Msg: i18n.T(ctx, "credential.failed")})
		return
	}
	a.closeCredentialRequest(ctx, c.ID)
	if c.Person != people.OwnerID {
		a.Events.Put(ctx, personal(pctx, "accounts.own"), "1")
	}
	if slices.Contains(services.LinkKinds, c.Connector) {
		a.restartLink(ctx, c.Connector)
	}
	if ext := a.externalConnector(c.Connector); ext != nil && t.house {
		// Restart so the process sees the new value.
		ext.Close()
	}
	a.Events.Append(ctx, "credential.saved", actor(ctx), map[string]string{"id": c.ID, "person": c.Person, "connector": c.Connector, "field": c.Field})
	out := map[string]any{"saved": true}
	if c.Routine != "" {
		if rt, err := a.myRoutine(ctx, c.Routine); err == nil && rt.State != store.RoutinePaused {
			out["routine"] = c.Routine
		}
	}
	if c.Exploration != "" {
		if _, err := a.myExploration(ctx, c.Exploration); err == nil {
			out["exploration"] = c.Exploration
		}
	}
	server.WriteJSON(w, 200, out)
}

// guardPasted takes a pasted key out of a message before it goes any
// further, and says what to tell the person: that it was removed and not
// kept, and where keys go instead.
func (a *App) guardPasted(ctx context.Context, text string) (clean, warning string) {
	clean, found := secretscan.Redact(text)
	if !found {
		return text, ""
	}
	a.Events.Append(ctx, "secret.redacted", actor(ctx), map[string]string{"person": people.Norm(people.From(ctx))})
	warning = i18n.T(ctx, "msg.secret.removed")
	for _, c := range a.credentialRequests(ctx) {
		if mine(ctx, c.Person) {
			v := a.credentialView(ctx, c)
			warning += "\n" + i18n.T(ctx, "msg.secret.form", "title", v.Title, "link", a.credentialLink(ctx, c.ID))
			break
		}
	}
	return clean, warning
}
