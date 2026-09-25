package remote

import (
	"context"
	"net"

	"tailscale.com/tsnet"
)

// Tailscale is the real node: Tailscale inside this process, keeping its
// state (the login) in Dir.
type Tailscale struct {
	Dir      string
	Hostname string

	srv *tsnet.Server
}

func (t *Tailscale) Start() error {
	t.srv = &tsnet.Server{Dir: t.Dir, Hostname: t.Hostname, Logf: func(string, ...any) {}}
	return t.srv.Start()
}

func (t *Tailscale) State(ctx context.Context) (string, string, string, error) {
	lc, err := t.srv.LocalClient()
	if err != nil {
		return "", "", "", err
	}
	st, err := lc.StatusWithoutPeers(ctx)
	if err != nil {
		return "", "", "", err
	}
	dns := ""
	if st.Self != nil {
		dns = st.Self.DNSName
	}
	return st.BackendState, st.AuthURL, dns, nil
}

func (t *Tailscale) Login(ctx context.Context) error {
	lc, err := t.srv.LocalClient()
	if err != nil {
		return err
	}
	return lc.StartLoginInteractive(ctx)
}

func (t *Tailscale) ListenFunnel() (net.Listener, error) {
	return t.srv.ListenFunnel("tcp", ":443", tsnet.FunnelOnly())
}

func (t *Tailscale) Close() error {
	if t.srv == nil {
		return nil
	}
	return t.srv.Close()
}
