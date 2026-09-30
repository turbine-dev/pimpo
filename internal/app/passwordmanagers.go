package app

import (
	"context"
	"net/http"
	"strings"

	"github.com/turbine-dev/pimpo/internal/connector/external"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/vault"
)

// Outside password managers (1Password, HashiCorp Vault) hold secrets that
// Pimpo keeps only a reference to. The owner sets up the house's; each
// person may set up their own, which only their own connections use.
// Nothing here ever answers with a token or with a value a reference
// points to.

func (a *App) passwordManagerRoutes() {
	a.Server.Handle("GET /api/password-managers", a.listPasswordManagers)
	a.Server.Handle("PUT /api/password-managers/{kind}", a.putPasswordManager)
	a.Server.Handle("DELETE /api/password-managers/{kind}", a.deletePasswordManager)
	a.Server.Handle("POST /api/password-managers/{kind}/test", a.testPasswordManager)
	a.Server.Handle("POST /api/secrets/check", a.checkReference)
}

// vaultScope is whose password managers a request uses: "" for the owner,
// who administers the house's, or the person's own id.
func vaultScope(ctx context.Context) string {
	if p := people.From(ctx); p != people.OwnerID {
		return p
	}
	return ""
}

type passwordManagersView struct {
	House       bool `json:"house"`
	OPInstalled bool `json:"op_installed"`
	OnePassword struct {
		Mode       string `json:"mode"`
		ConnectURL string `json:"connect_url,omitempty"`
	} `json:"onepassword"`
	HashiCorp struct {
		Auth      string `json:"auth"`
		Addr      string `json:"addr,omitempty"`
		Namespace string `json:"namespace,omitempty"`
		AuthMount string `json:"auth_mount,omitempty"`
	} `json:"hashicorp"`
}

func (a *App) passwordManagersView(ctx context.Context) (passwordManagersView, error) {
	scope := vaultScope(ctx)
	m, err := a.Vault.Managers(ctx, scope)
	var v passwordManagersView
	v.House, v.OPInstalled = scope == "", a.Vault.OPInstalled()
	if op := m.OnePassword; op != nil {
		v.OnePassword.Mode, v.OnePassword.ConnectURL = op.Mode(), op.ConnectURL
	}
	if hv := m.HashiCorp; hv != nil {
		v.HashiCorp.Auth, v.HashiCorp.Addr, v.HashiCorp.Namespace, v.HashiCorp.AuthMount = hv.Auth(), hv.Addr, hv.Namespace, hv.AuthMount
	}
	return v, err
}

func (a *App) listPasswordManagers(w http.ResponseWriter, r *http.Request) {
	v, err := a.passwordManagersView(r.Context())
	if err != nil {
		server.WriteError(w, err)
		return
	}
	server.WriteJSON(w, 200, v)
}

// putPasswordManager sets up one manager. A credential left empty keeps
// the one already stored, so changing an address needs no token again.
func (a *App) putPasswordManager(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	scope := vaultScope(ctx)
	var req struct {
		Mode         string `json:"mode"` // service, desktop or connect
		Token        string `json:"token"`
		ConnectURL   string `json:"connect_url"`
		ConnectToken string `json:"connect_token"`
		Addr         string `json:"addr"`
		Auth         string `json:"auth"` // token or approle
		RoleID       string `json:"role_id"`
		SecretID     string `json:"secret_id"`
		Namespace    string `json:"namespace"`
		AuthMount    string `json:"auth_mount"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	m, err := a.Vault.Managers(ctx, scope)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	keep := func(s, old string) string {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
		return old
	}
	kind := r.PathValue("kind")
	switch kind {
	case "onepassword":
		old := m.OnePassword
		if old == nil {
			old = &vault.OnePassword{}
		}
		op := &vault.OnePassword{}
		switch req.Mode {
		case "service":
			op.Token = keep(req.Token, old.Token)
		case "desktop":
			op.Desktop = true
		case "connect":
			op.ConnectURL = strings.TrimRight(strings.TrimSpace(req.ConnectURL), "/")
			op.ConnectToken = keep(req.ConnectToken, old.ConnectToken)
		default:
			server.WriteError(w, server.StatusError{Status: 400, Msg: "choose a service account, the 1Password app or a Connect server"})
			return
		}
		if op.Mode() == "" {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "the 1Password token is required"})
			return
		}
		m.OnePassword = op
	case "hashicorp":
		old := m.HashiCorp
		if old == nil {
			old = &vault.HashiCorp{}
		}
		hv := &vault.HashiCorp{Addr: strings.TrimRight(strings.TrimSpace(req.Addr), "/"), Namespace: strings.TrimSpace(req.Namespace), AuthMount: strings.TrimSpace(req.AuthMount)}
		switch req.Auth {
		case "token":
			hv.Token = keep(req.Token, old.Token)
		case "approle":
			hv.RoleID, hv.SecretID = keep(req.RoleID, old.RoleID), keep(req.SecretID, old.SecretID)
		default:
			server.WriteError(w, server.StatusError{Status: 400, Msg: "choose a token or AppRole"})
			return
		}
		if hv.Auth() == "" {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "the HashiCorp Vault credentials are required"})
			return
		}
		m.HashiCorp = hv
	default:
		server.WriteError(w, server.StatusError{Status: 404, Msg: "unknown password manager"})
		return
	}
	if err := a.Vault.SetManagers(ctx, scope, m); err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	a.Events.Append(ctx, "password_manager.changed", actor(ctx), map[string]any{"kind": kind, "set": true})
	a.listPasswordManagers(w, r)
}

func (a *App) deletePasswordManager(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	scope := vaultScope(ctx)
	m, err := a.Vault.Managers(ctx, scope)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	kind := r.PathValue("kind")
	switch kind {
	case "onepassword":
		m.OnePassword = nil
	case "hashicorp":
		m.HashiCorp = nil
	default:
		server.WriteError(w, server.StatusError{Status: 404, Msg: "unknown password manager"})
		return
	}
	if err := a.Vault.SetManagers(ctx, scope, m); err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(ctx, "password_manager.changed", actor(ctx), map[string]any{"kind": kind, "set": false})
	a.listPasswordManagers(w, r)
}

func (a *App) testPasswordManager(w http.ResponseWriter, r *http.Request) {
	if err := a.Vault.TestManager(r.Context(), vaultScope(r.Context()), r.PathValue("kind")); err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	server.WriteJSON(w, 200, map[string]bool{"ok": true})
}

// checkReference resolves a reference once with the caller's own
// credentials and says only whether it was found.
func (a *App) checkReference(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reference string `json:"reference"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	if err := a.Vault.Check(r.Context(), vaultScope(r.Context()), strings.TrimSpace(req.Reference)); err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	server.WriteJSON(w, 200, map[string]bool{"found": true})
}

// plainSecret is what a secret typed in a form is, for the checks that
// need it before it is kept (a Telegram token asked of Telegram): a
// reference is read with the credentials of whoever the secret belongs to.
func (a *App) plainSecret(ctx context.Context, name, value string) (string, error) {
	if !vault.IsReference(value) {
		return value, nil
	}
	return a.Vault.Resolve(ctx, vault.ScopeOf(name), value)
}

// appPassword drops the spaces Google shows in app passwords; a reference
// is kept as typed, since item names may have spaces.
func appPassword(s string) string {
	if s = strings.TrimSpace(s); vault.IsReference(s) {
		return s
	}
	return strings.ReplaceAll(s, " ", "")
}

// probe asks a server for its tools with the values its references point
// to; what is kept afterwards is still the references.
func (a *App) probe(ctx context.Context, e external.Endpoint) ([]external.Tool, error) {
	for _, m := range []*map[string]string{&e.Env, &e.Headers} {
		plain := map[string]string{}
		for k, v := range *m {
			p, err := a.plainSecret(ctx, "connector", v)
			if err != nil {
				return nil, err
			}
			plain[k] = p
		}
		*m = plain
	}
	return external.Probe(ctx, e)
}
