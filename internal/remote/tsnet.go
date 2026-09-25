package remote

import (
	"context"
	"net"

	"tailscale.com/tailcfg"
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

// Funnel reports whether the tailnet lets this node use Funnel (HTTPS
// certificates and the funnel attribute) and, when not, the link where the
// owner turns both on with one click.
func (t *Tailscale) Funnel(ctx context.Context) (bool, string, error) {
	lc, err := t.srv.LocalClient()
	if err != nil {
		return false, "", err
	}
	st, err := lc.StatusWithoutPeers(ctx)
	if err != nil {
		return false, "", err
	}
	if st.Self != nil && st.Self.HasCap(tailcfg.CapabilityHTTPS) && st.Self.HasCap(tailcfg.NodeAttrFunnel) {
		return true, "", nil
	}
	info, err := lc.QueryFeature(ctx, "funnel")
	if err != nil {
		return false, "", err
	}
	return info.Complete, info.URL, nil
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
