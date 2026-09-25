package app

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/denerFernandes/zodim/internal/remote"
	"github.com/denerFernandes/zodim/internal/server"
)

// Two ways for the phone to reach this Zodim, both remembered across
// restarts: Tailscale inside the binary (a stable https link from
// anywhere) and the home network (no account, same Wi-Fi only).

const lanPort = 7788

// AttachRemote prepares both; newNode is nil for the real Tailscale.
func (a *App) AttachRemote(home string, newNode func() remote.Node) {
	if newNode == nil {
		newNode = func() remote.Node {
			return &remote.Tailscale{Dir: filepath.Join(home, "tailscale"), Hostname: "zodim"}
		}
	}
	a.Remote = &remote.Remote{NewNode: newNode, Handler: a.Server, OnURL: func(url string) {
		a.Events.Put(context.Background(), "public_url", url)
	}}
	a.LAN = &remote.LAN{Port: lanPort, Handler: a.Server}
}

// startRemote resumes what the owner turned on before.
func (a *App) startRemote(ctx context.Context) {
	if a.Remote == nil {
		return
	}
	if on, _ := a.Events.Get(ctx, "remote.tailscale"); on == "on" {
		a.Remote.Start(ctx)
	}
	if on, _ := a.Events.Get(ctx, "remote.lan"); on == "on" {
		a.LAN.Start()
	}
}

type remoteView struct {
	Tailscale remote.Status `json:"tailscale"`
	LAN       struct {
		On    bool   `json:"on"`
		URL   string `json:"url,omitempty"`
		Error string `json:"error,omitempty"`
	} `json:"lan"`
}

func (a *App) remoteRoutes() {
	a.Server.Handle("GET /api/remote", a.getRemote)
	a.Server.Handle("POST /api/remote/{kind}/{switch}", a.switchRemote)
}

func (a *App) remoteStatus(ctx context.Context) remoteView {
	var v remoteView
	if a.Remote == nil {
		v.Tailscale = remote.Status{State: "off"}
		return v
	}
	v.Tailscale = a.Remote.Status()
	v.LAN.URL = a.LAN.URL()
	v.LAN.On = v.LAN.URL != ""
	return v
}

func (a *App) getRemote(w http.ResponseWriter, r *http.Request) {
	server.WriteJSON(w, 200, a.remoteStatus(r.Context()))
}

func (a *App) switchRemote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if a.Remote == nil {
		server.WriteError(w, server.StatusError{Status: 503, Msg: "remote access is not available here"})
		return
	}
	on := r.PathValue("switch") == "on"
	state := map[bool]string{true: "on", false: "off"}[on]
	v := a.remoteStatus(ctx)
	switch r.PathValue("kind") {
	case "tailscale":
		if on {
			a.Remote.Start(ctx)
		} else {
			a.Remote.Stop()
			if base, _ := a.Events.Get(ctx, "public_url"); base == v.Tailscale.URL && base != "" {
				a.Events.Put(ctx, "public_url", "")
			}
		}
	case "lan":
		if on {
			if _, err := a.LAN.Start(); err != nil {
				v.LAN.Error = err.Error()
				server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
				return
			}
		} else {
			a.LAN.Stop()
		}
	default:
		server.WriteError(w, server.StatusError{Status: 404, Msg: "unknown remote access"})
		return
	}
	a.Events.Put(ctx, "remote."+r.PathValue("kind"), state)
	a.Events.Append(ctx, "remote.changed", "human:owner", map[string]string{"kind": r.PathValue("kind"), "state": state})
	server.WriteJSON(w, 200, a.remoteStatus(ctx))
}
