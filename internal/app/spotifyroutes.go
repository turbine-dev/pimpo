package app

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/denerFernandes/pimpo/internal/connector/spotify"
	"github.com/denerFernandes/pimpo/internal/server"
)

func (a *App) spotify() *spotify.Spotify {
	if a.Spotify == nil {
		a.Spotify = &spotify.Spotify{Store: vaultStore{a.Vault}}
	}
	return a.Spotify
}

// spotifyRedirect is where Spotify sends the owner back; it accepts
// loopback addresses by number only.
func spotifyRedirect(r *http.Request) string {
	host := strings.Replace(r.Host, "localhost", "127.0.0.1", 1)
	return "http://" + host + "/oauth/spotify"
}

func (a *App) spotifyRoutes() {
	a.Server.Handle("GET /api/spotify", func(w http.ResponseWriter, r *http.Request) {
		id, _ := vaultStore{a.Vault}.Get(r.Context(), "spotify.client_id")
		server.WriteJSON(w, 200, map[string]any{"connected": a.spotify().Connected(r.Context()), "client_id": id != "", "redirect": spotifyRedirect(r)})
	})
	a.Server.Handle("POST /api/oauth/spotify/start", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ClientID string `json:"client_id"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		if strings.TrimSpace(req.ClientID) == "" {
			req.ClientID, _ = vaultStore{a.Vault}.Get(r.Context(), "spotify.client_id")
		}
		link, err := a.spotify().Begin(r.Context(), req.ClientID, spotifyRedirect(r))
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
			return
		}
		server.WriteJSON(w, 200, map[string]string{"url": link, "redirect": spotifyRedirect(r)})
	})
	a.Server.Handle("DELETE /api/spotify", func(w http.ResponseWriter, r *http.Request) {
		a.Vault.Delete(r.Context(), "spotify.refresh")
		a.Events.Append(r.Context(), "connection.changed", "human:owner", map[string]string{"kind": "spotify", "state": "off"})
		server.WriteJSON(w, 200, map[string]bool{"ok": true})
	})
	// The state value, checked by Finish, is the credential.
	a.Server.HandlePublic("GET /oauth/spotify", func(w http.ResponseWriter, r *http.Request) {
		if e := r.URL.Query().Get("error"); e != "" {
			http.Redirect(w, r, "/connections?spotify="+url.QueryEscape(e), http.StatusSeeOther)
			return
		}
		if err := a.spotify().Finish(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code")); err != nil {
			http.Redirect(w, r, "/connections?spotify="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		a.Events.Append(r.Context(), "connection.changed", "human:owner", map[string]string{"kind": "spotify", "state": "on"})
		http.Redirect(w, r, "/connections?spotify=ok", http.StatusSeeOther)
	})
}
