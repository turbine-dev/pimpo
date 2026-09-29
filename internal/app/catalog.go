package app

import (
	"context"
	"errors"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/turbine-dev/pimpo/internal/connector/external"
	"github.com/turbine-dev/pimpo/internal/connector/services"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/vault"
)

// The catalog: connectors set up from a form in Connections. Secret fields
// live in the vault, the rest in settings; both are per person, like mail.

func catalogKey(kind, field string) string { return "conn." + kind + "." + field }

func (a *App) catalogConfig(kind string) services.Config {
	k, _ := services.Get(kind)
	secret := map[string]bool{}
	for _, f := range k.Fields {
		secret[f.Name] = f.Secret
	}
	return func(ctx context.Context, field string) (string, error) {
		name := personal(ctx, catalogKey(kind, field))
		if secret[field] {
			return a.Vault.Get(ctx, name)
		}
		return a.Events.Get(ctx, name)
	}
}

func (a *App) catalogRoutes() {
	a.Server.Handle("GET /api/catalog", a.listCatalog)
	a.Server.Handle("PUT /api/catalog/{kind}", a.putCatalog)
	a.Server.Handle("DELETE /api/catalog/{kind}", a.deleteCatalog)
	a.Server.Handle("POST /api/catalog/{kind}/check", a.checkCatalog)
}

type catalogCap struct {
	Name      string `json:"name"`
	Risk      string `json:"risk"`
	Signature string `json:"signature"`
	Returns   string `json:"returns"`
}

type catalogView struct {
	services.Kind
	Capabilities []catalogCap `json:"capabilities"`
	Configured   bool         `json:"configured"`
	// Values holds the non-secret fields already set, to show them.
	Values map[string]string `json:"values"`
	// External connectors run as their own process, from connector.json.
	External bool   `json:"external,omitempty"`
	Source   string `json:"source,omitempty"`
}

func (a *App) catalogConfigured(ctx context.Context, k services.Kind) (bool, map[string]string) {
	cfg := a.catalogConfig(k.ID)
	values := map[string]string{}
	ok, required, filled := true, false, false
	for _, f := range k.Fields {
		v, err := cfg(ctx, f.Name)
		set := err == nil && v != ""
		required = required || !f.Optional
		filled = filled || set
		if !set && !f.Optional {
			ok = false
		}
		if !f.Secret && v != "" {
			values[f.Name] = v
		}
	}
	// When every field is optional, as with web search, one must be set.
	if len(k.Fields) > 0 && !required {
		ok = filled
	}
	return ok, values
}

// localKind puts a built-in connector's texts in the owner's language.
func localKind(ctx context.Context, k services.Kind) services.Kind {
	lang := i18n.Of(ctx)
	pick := func(key, fallback string) string {
		if t, ok := i18n.Lookup(lang, "svc."+k.ID+"."+key); ok {
			return t
		}
		return fallback
	}
	k.Title, k.Description, k.Help = pick("title", k.Title), pick("description", k.Description), pick("help", k.Help)
	fields := make([]services.Field, len(k.Fields))
	for i, f := range k.Fields {
		f.Label = pick("field."+f.Name+".label", f.Label)
		if f.Placeholder != "" {
			f.Placeholder = pick("field."+f.Name+".placeholder", f.Placeholder)
		}
		fields[i] = f
	}
	if k.Fields != nil {
		k.Fields = fields
	}
	return k
}

func (a *App) listCatalog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := []catalogView{}
	for _, k := range services.All() {
		v := catalogView{Kind: localKind(ctx, k), Capabilities: []catalogCap{}}
		if k.Fields == nil {
			v.Fields = []services.Field{}
		}
		for _, s := range k.Specs {
			v.Capabilities = append(v.Capabilities, catalogCap{s.Name, s.Risk.String(), s.Signature, s.Returns})
		}
		v.Configured, v.Values = a.catalogConfigured(ctx, k)
		out = append(out, v)
	}
	out = append(out, a.externalKinds(ctx)...)
	a.mu.Lock()
	broken := append([]string{}, a.externalErrs...)
	a.mu.Unlock()
	server.WriteJSON(w, 200, map[string]any{"connectors": out, "broken": broken})
}

func (a *App) putCatalog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req map[string]string
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	if c := a.externalConnector(r.PathValue("kind")); c != nil {
		for _, e := range append(append([]string{}, c.Env...), c.Headers...) {
			if v := strings.TrimSpace(req[e]); v != "" {
				if err := a.Vault.Set(ctx, "connector."+c.Name+"."+e, v); err != nil {
					server.WriteError(w, err)
					return
				}
			}
		}
		// Restart so the process sees the new values.
		c.Close()
		server.WriteJSON(w, 200, map[string]string{"kind": c.Name})
		return
	}
	k, ok := services.Get(r.PathValue("kind"))
	if !ok {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "unknown connector"})
		return
	}
	for _, f := range k.Fields {
		v := strings.TrimSpace(req[f.Name])
		if v == "" {
			if f.Optional {
				continue
			}
			// Leaving a secret empty keeps the one already saved.
			if cur, _ := a.catalogConfig(k.ID)(ctx, f.Name); f.Secret && cur != "" {
				continue
			}
			server.WriteError(w, server.StatusError{Status: 400, Msg: f.Label + " is required"})
			return
		}
		name := personal(ctx, catalogKey(k.ID, f.Name))
		var err error
		if f.Secret {
			err = a.Vault.Set(ctx, name, v)
		} else {
			err = a.Events.Put(ctx, name, v)
		}
		if err != nil {
			server.WriteError(w, err)
			return
		}
	}
	a.Events.Append(ctx, "connection.set", actor(ctx), map[string]string{"kind": k.ID})
	if slices.Contains(services.LinkKinds, k.ID) {
		a.restartLink(ctx, k.ID)
	}
	server.WriteJSON(w, 200, map[string]string{"kind": k.ID})
}

func (a *App) deleteCatalog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	k, ok := services.Get(r.PathValue("kind"))
	if !ok {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "unknown connector"})
		return
	}
	for _, f := range k.Fields {
		name := personal(ctx, catalogKey(k.ID, f.Name))
		if f.Secret {
			a.Vault.Delete(ctx, name)
		} else {
			a.Events.Put(ctx, name, "")
		}
	}
	a.Events.Append(ctx, "connection.removed", actor(ctx), map[string]string{"kind": k.ID})
	if slices.Contains(services.LinkKinds, k.ID) {
		a.restartLink(ctx, k.ID)
		a.Events.Put(ctx, linkOwnerKey(k.ID), "")
	}
	server.WriteJSON(w, 200, map[string]string{"kind": k.ID, "state": "removed"})
}

func (a *App) externalConnector(name string) *external.Connector {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.external[name]
}

func (a *App) checkCatalog(w http.ResponseWriter, r *http.Request) {
	if c := a.externalConnector(r.PathValue("kind")); c != nil {
		problems := external.Check(r.Context(), c.Manifest, c.Secrets)
		server.WriteJSON(w, 200, map[string]any{"ok": len(problems) == 0, "detail": strings.Join(problems, "; ")})
		return
	}
	k, ok := services.Get(r.PathValue("kind"))
	if !ok {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "unknown connector"})
		return
	}
	if k.Probe == nil {
		server.WriteJSON(w, 200, map[string]any{"ok": true, "detail": "nothing to check without sending a message"})
		return
	}
	if err := k.Probe(r.Context(), a.catalogConfig(k.ID)); err != nil {
		if errors.Is(err, vault.ErrNotFound) {
			err = errors.New(k.Title + " is not set up")
		}
		server.WriteJSON(w, 200, map[string]any{"ok": false, "detail": err.Error()})
		return
	}
	server.WriteJSON(w, 200, map[string]any{"ok": true})
}

// AttachConnectors loads the external connectors installed under dir.
// Broken ones are reported in the catalog and not loaded.
func (a *App) AttachConnectors(dir string) {
	found, errs := external.Discover(dir)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.externalErrs = nil
	for _, err := range errs {
		a.externalErrs = append(a.externalErrs, err.Error())
	}
	if a.external == nil {
		a.external = map[string]*external.Connector{}
	}
	for _, m := range found {
		m.Register()
		name := m.Name
		c := &external.Connector{Manifest: m, Secrets: func(ctx context.Context, v string) (string, error) {
			return a.Vault.Get(ctx, "connector."+name+"."+v)
		}}
		a.external[name] = c
		a.Router.Add(c)
	}
}

func (a *App) externalKinds(ctx context.Context) []catalogView {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []catalogView
	for _, c := range a.external {
		help := i18n.T(ctx, "catalog.externalHelp", "dir", c.Dir)
		if c.URL != "" {
			help = i18n.T(ctx, "catalog.remoteHelp", "url", c.URL)
		}
		if c.HTTP != nil {
			help = i18n.T(ctx, "catalog.httpHelp", "url", c.HTTP.Base)
		}
		v := catalogView{Kind: services.Kind{ID: c.Name, Title: c.Name, Description: c.Description, Help: help, Fields: []services.Field{}},
			Capabilities: []catalogCap{}, Values: map[string]string{}, External: true, Configured: true, Source: c.Source}
		for _, e := range append(append([]string{}, c.Env...), c.Headers...) {
			v.Fields = append(v.Fields, services.Field{Name: e, Label: e, Secret: true})
		}
		for _, cp := range c.Manifest.Capabilities {
			v.Capabilities = append(v.Capabilities, catalogCap{cp.Name, cp.Risk, cp.Signature, cp.Returns})
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
