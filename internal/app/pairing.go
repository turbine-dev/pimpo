package app

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/denerFernandes/vigia/internal/server"
)

// The phone app opens Vigia through an address the owner exposes, such as
// `tailscale serve`. Pairing hands it that address with the session token.

func (a *App) pairingRoutes() {
	a.Server.Handle("GET /api/pairing", a.getPairing)
	a.Server.Handle("POST /api/pairing", a.setPairing)
}

func (a *App) pairingLink(base string) string {
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/auth?token=" + url.QueryEscape(a.Server.Token)
}

func (a *App) getPairing(w http.ResponseWriter, r *http.Request) {
	base, _ := a.Events.Get(r.Context(), "public_url")
	server.WriteJSON(w, 200, map[string]string{"base": base, "link": a.pairingLink(base)})
}

func (a *App) setPairing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Base string `json:"base"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	base := strings.TrimSpace(req.Base)
	if base != "" {
		u, err := url.Parse(base)
		if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && private(u.Hostname()))) {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "use an https address; the token must not travel in the clear"})
			return
		}
		base = u.Scheme + "://" + u.Host
	}
	if err := a.Events.Put(r.Context(), "public_url", base); err != nil {
		server.WriteError(w, err)
		return
	}
	server.WriteJSON(w, 200, map[string]string{"base": base, "link": a.pairingLink(base)})
}

func private(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()))
}
