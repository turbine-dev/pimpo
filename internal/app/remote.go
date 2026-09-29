package app

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/turbine-dev/pimpo/internal/remote"
	"github.com/turbine-dev/pimpo/internal/server"
)

// Two ways for the phone to reach this Pimpo, both remembered across
// restarts: Tailscale inside the binary (a stable https link from
// anywhere) and the home network (no account, same Wi-Fi only).

// defaultLANPort is used when the main server's address is unknown.
const defaultLANPort = 7788

// AttachRemote prepares both; newNode is nil for the real Tailscale.
func (a *App) AttachRemote(home string, newNode func() remote.Node) {
	if newNode == nil {
		newNode = func() remote.Node {
			dir := filepath.Join(home, "tailscale")
			return &remote.Tailscale{Dir: dir, Hostname: a.tailscaleName(context.Background(), dir)}
		}
	}
	a.Remote = &remote.Remote{NewNode: newNode, Handler: a.Server, OnURL: func(url string) {
		a.Events.Put(context.Background(), "public_url", url)
	}}
	port, served := defaultLANPort, false
	if host, p, err := net.SplitHostPort(a.ListenAddr); err == nil {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			port = n
		}
		ip := net.ParseIP(host)
		served = host == "" || ip != nil && ip.IsUnspecified()
	}
	a.LAN = &remote.LAN{Port: port, Handler: a.Server, Served: served}
}

// tailscaleName is the machine's name on the tailnet, which is part of the
// phone's link. An install that joined before the rename keeps its name, so
// paired phones keep working; new ones are "pimpo". The choice is kept.
func (a *App) tailscaleName(ctx context.Context, dir string) string {
	if name, _ := a.Events.Get(ctx, "remote.hostname"); name != "" {
		return name
	}
	name := "pimpo"
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		name = "zodim"
	}
	a.Events.Put(ctx, "remote.hostname", name)
	return name
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
