package app

import (
	"context"
	"net/http"
	"net/url"

	"github.com/denerFernandes/zodim/internal/oauth"
	"github.com/denerFernandes/zodim/internal/server"
	"github.com/denerFernandes/zodim/internal/vault"
)

// vaultStore lets OAuth keep its tokens in the vault.
type vaultStore struct{ v *vault.Vault }

func (s vaultStore) Get(ctx context.Context, name string) (string, error) {
	v, err := s.v.Get(ctx, name)
	if err == vault.ErrNotFound {
		return "", nil
	}
	return v, err
}
func (s vaultStore) Set(ctx context.Context, name, value string) error {
	return s.v.Set(ctx, name, value)
}

func (a *App) googleRoutes() {
	a.Google = &oauth.Google{Store: vaultStore{a.Vault}}
	a.Server.Handle("POST /api/oauth/google/start", a.googleStart)
	a.Server.HandlePublic("GET /oauth/google", a.googleCallback)
}

func (a *App) googleStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	redirect := "http://" + r.Host + "/oauth/google"
	link, err := a.Google.Begin(r.Context(), req.ClientID, req.ClientSecret, redirect)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	server.WriteJSON(w, 200, map[string]string{"url": link, "redirect": redirect})
}

// googleCallback is where Google sends the owner back. The state value,
// checked by Finish, is the credential.
func (a *App) googleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if e := r.URL.Query().Get("error"); e != "" {
		http.Redirect(w, r, "/connections?google="+url.QueryEscape(e), http.StatusSeeOther)
		return
	}
	email, err := a.Google.Finish(ctx, r.URL.Query().Get("state"), r.URL.Query().Get("code"))
	if err != nil {
		http.Redirect(w, r, "/connections?google="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	a.Events.Put(ctx, "mail.auth", "oauth")
	a.Events.Put(ctx, "mail.addr", "imap.gmail.com:993")
	a.Events.Put(ctx, "mail.smtp", "smtp.gmail.com:465")
	a.Events.Put(ctx, "calendar.source", "google")
	if email != "" {
		a.Events.Put(ctx, "mail.user", email)
	}
	a.Events.Append(ctx, "connection.changed", "human:owner", map[string]string{"kind": "google", "email": email})
	http.Redirect(w, r, "/connections?google=ok", http.StatusSeeOther)
}
