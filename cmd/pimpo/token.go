package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/snapshot"
)

// The owner's login link carries the master token. It is printed when it
// is first made and whenever asked for; `pimpo token rotate` replaces it,
// which signs out every browser that used the old link.

const (
	tokenKey  = "session_token"
	listenKey = "listen_addr"
)

func tokenCmd(args []string, out io.Writer) error {
	rotate := len(args) > 0 && args[0] == "rotate"
	if rotate {
		args = args[1:]
	}
	fs := flag.NewFlagSet("token", flag.ContinueOnError)
	dir := fs.String("data", "", "data directory (default ~/.pimpo)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("usage: pimpo token [rotate]")
	}
	if rec, ok := snapshot.Recovering(dataDir(*dir)); ok {
		fmt.Fprintf(out, "Pimpo is in recovery. Open: %s\n", loginLink("", rec.Token))
		return nil
	}
	s, err := event.Open(filepath.Join(dataDir(*dir), "pimpo.db"))
	if err != nil {
		return err
	}
	defer s.Close()
	ctx := context.Background()
	t, err := s.Get(ctx, tokenKey)
	if err != nil {
		return err
	}
	if rotate || t == "" {
		t = newMasterToken()
		if err := s.Put(ctx, tokenKey, t); err != nil {
			return err
		}
	}
	addr, _ := s.Get(ctx, listenKey)
	fmt.Fprintf(out, "Open: %s\n", loginLink(addr, t))
	if rotate {
		fmt.Fprintln(out, "\nThe old link and the browsers signed in with it no longer work. A running Pimpo picks up the new one within seconds, unless PIMPO_TOKEN fixes it.")
	}
	return nil
}

func newMasterToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// loginLink is the owner's link on this computer; an address on every
// interface is shown as loopback.
func loginLink(addr, token string) string {
	if addr == "" {
		addr = "127.0.0.1:7788"
	}
	if host, port, err := net.SplitHostPort(addr); err == nil {
		if ip := net.ParseIP(host); host == "" || ip != nil && ip.IsUnspecified() {
			addr = net.JoinHostPort("127.0.0.1", port)
		}
	}
	return "http://" + addr + "/auth?token=" + token
}

// sessionToken is created once and kept, so the login link stays valid
// across restarts until the owner rotates it. created says it was just
// made, when the link is worth printing.
func sessionToken(ctx context.Context, s *event.Store) (token string, created bool, err error) {
	// Browser tests and the desktop app pin the token so they can log in.
	if t := os.Getenv("PIMPO_TOKEN"); t != "" {
		return t, false, s.Put(ctx, tokenKey, t)
	}
	if t, err := s.Get(ctx, tokenKey); err != nil || t != "" {
		return t, false, err
	}
	t := newMasterToken()
	return t, true, s.Put(ctx, tokenKey, t)
}

// followToken applies a token rotated from the command line while Pimpo
// runs.
func followToken(ctx context.Context, s *event.Store, current string, set func(string)) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			if t, err := s.Get(ctx, tokenKey); err == nil && t != "" && t != current {
				current = t
				set(t)
			}
		case <-ctx.Done():
			return
		}
	}
}
