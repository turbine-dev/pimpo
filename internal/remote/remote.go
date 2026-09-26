// Package remote gives Pimpo a link the owner's phone can open from
// anywhere, with nothing to install: Tailscale runs inside the binary
// (tsnet) and Funnel publishes the web app over HTTPS, with TLS ending on
// this machine. The first time, the owner signs in to Tailscale in the
// browser; after that the link comes back by itself.
package remote

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Node is the Tailscale node; tsnet in production, a fake in tests.
type Node interface {
	Start() error
	// State reports the backend state ("NeedsLogin", "Running", …), the
	// login URL when there is one, and the node's DNS name.
	State(ctx context.Context) (state, authURL, dnsName string, err error)
	Login(ctx context.Context) error
	// Funnel reports whether Funnel can be used, or the link that turns it
	// on for the tailnet.
	Funnel(ctx context.Context) (ready bool, enableURL string, err error)
	ListenFunnel() (net.Listener, error)
	Close() error
}

type Status struct {
	// State is off, starting, needs_login, needs_funnel, running or error.
	// AuthURL is the sign-in link, or with needs_funnel the link that turns
	// on HTTPS and Funnel.
	State   string `json:"state"`
	AuthURL string `json:"auth_url,omitempty"`
	URL     string `json:"url,omitempty"`
	Error   string `json:"error,omitempty"`
}

type Remote struct {
	// NewNode makes the node; it is called on every start.
	NewNode func() Node
	Handler http.Handler
	// OnURL is told the public link once it works.
	OnURL func(url string)

	mu     sync.Mutex
	status Status
	node   Node
	cancel context.CancelFunc
	srv    *http.Server
}

func (r *Remote) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.status.State == "" {
		return Status{State: "off"}
	}
	return r.status
}

func (r *Remote) set(s Status) {
	r.mu.Lock()
	r.status = s
	r.mu.Unlock()
}

// Start brings the node up in the background: it asks for a login when
// needed and publishes the handler once the node runs.
func (r *Remote) Start(ctx context.Context) {
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r.cancel = cancel
	r.status = Status{State: "starting"}
	r.mu.Unlock()
	go r.run(ctx)
}

func (r *Remote) fail(err error) {
	r.set(Status{State: "error", Error: friendly(err)})
}

func (r *Remote) run(ctx context.Context) {
	node := r.NewNode()
	r.mu.Lock()
	r.node = node
	r.mu.Unlock()
	if err := node.Start(); err != nil {
		r.fail(err)
		return
	}
	asked := false
	for ctx.Err() == nil {
		state, auth, dns, err := node.State(ctx)
		switch {
		case err != nil:
			r.fail(err)
		case state == "Running":
			ready, enable, err := node.Funnel(ctx)
			switch {
			case err != nil:
				r.fail(err)
			case ready:
				r.publish(ctx, node, dns)
				return
			case enable != "":
				r.set(Status{State: "needs_funnel", AuthURL: enable})
			default:
				r.fail(errors.New("Funnel not available; HTTPS must be enabled"))
			}
		case state == "NeedsLogin" && auth == "" && !asked:
			asked = true
			if err := node.Login(ctx); err != nil {
				r.fail(err)
			}
		case auth != "":
			r.set(Status{State: "needs_login", AuthURL: auth})
		}
		select {
		case <-time.After(700 * time.Millisecond):
		case <-ctx.Done():
		}
	}
}

func (r *Remote) publish(ctx context.Context, node Node, dns string) {
	ln, err := node.ListenFunnel()
	if err != nil {
		r.fail(err)
		return
	}
	url := "https://" + strings.TrimSuffix(dns, ".")
	srv := &http.Server{Handler: r.Handler, ReadHeaderTimeout: 10 * time.Second}
	if r.OnURL != nil {
		r.OnURL(url)
	}
	r.mu.Lock()
	r.srv = srv
	r.status = Status{State: "running", URL: url}
	r.mu.Unlock()
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	srv.Serve(ln)
}

// Stop takes the link down; the login is kept for next time.
func (r *Remote) Stop() {
	r.mu.Lock()
	cancel, node := r.cancel, r.node
	r.cancel, r.node, r.status = nil, nil, Status{State: "off"}
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if node != nil {
		node.Close()
	}
}

// friendly turns the usual Tailscale refusals into something actionable.
func friendly(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "HTTPS") || strings.Contains(s, "cert"):
		return "HTTPS certificates are off in your Tailscale account: turn on HTTPS in the admin console (login.tailscale.com/admin/dns), then turn this off and on again."
	case strings.Contains(s, "Funnel not available") || strings.Contains(s, "funnel"):
		return "Funnel is off in your Tailscale account: allow it in the admin console (Access controls › Funnel), then turn this off and on again."
	}
	return s
}

var ErrOff = errors.New("remote access is off")
