// Command vigia runs the personal agent: the local server, the web UI and
// the Telegram channel.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/denerFernandes/vigia/internal/event"
	"github.com/denerFernandes/vigia/internal/server"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "vigia:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		return serve(args)
	case "version":
		fmt.Println(version)
		return nil
	}
	return fmt.Errorf("unknown command %q (try: serve, version)", cmd)
}

func dataDir(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if d := os.Getenv("VIGIA_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".vigia")
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:7788", "listen address (loopback only unless you know why)")
	dir := fs.String("data", "", "data directory (default ~/.vigia)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	home := dataDir(*dir)
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	store, err := event.Open(filepath.Join(home, "vigia.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	token, err := sessionToken(ctx, store)
	if err != nil {
		return err
	}
	srv := server.New(store, token)
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	fmt.Printf("Vigia %s is running.\n\n  Open: http://%s/auth?token=%s\n\nData: %s\n", version, ln.Addr(), token, home)
	httpSrv := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdown)
	}()
	if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// sessionToken is created once and kept, so the login link stays valid
// across restarts until the user rotates it.
func sessionToken(ctx context.Context, s *event.Store) (string, error) {
	if t, err := s.Get(ctx, "session_token"); err != nil || t != "" {
		return t, err
	}
	b := make([]byte, 24)
	rand.Read(b)
	t := hex.EncodeToString(b)
	return t, s.Put(ctx, "session_token", t)
}
