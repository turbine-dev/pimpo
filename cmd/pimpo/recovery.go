package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/turbine-dev/pimpo/internal/app"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/snapshot"
)

// checkDatabase checks the database before anything opens it for
// writing. A damaged one is moved aside untouched and Pimpo goes into
// recovery.
func checkDatabase(home string) error {
	if _, ok := snapshot.Recovering(home); ok {
		return nil
	}
	err := event.CheckFile(filepath.Join(home, "pimpo.db"))
	if err == nil || !errors.Is(err, event.ErrDamaged) {
		return err
	}
	if running(home) {
		return fmt.Errorf("%w, and another Pimpo is using it: stop it first", err)
	}
	rec, qerr := snapshot.Quarantine(home, err.Error())
	if qerr != nil {
		return fmt.Errorf("%v; %w", err, qerr)
	}
	fmt.Printf("The database failed its check (%s).\nIt was moved, untouched, to %s.\n\n", err, filepath.Dir(rec.Quarantined(home)))
	return nil
}

// recoveryMode serves only the recovery page until the owner puts a
// database back or starts fresh; then Pimpo starts as usual.
func recoveryMode(ctx context.Context, home, addr string, rec snapshot.Recovery) error {
	done, finish := context.WithCancel(ctx)
	defer finish()
	srv, err := app.RecoveryServer(home, rec, os.Getenv("PIMPO_TOKEN"), finish)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	pid := filepath.Join(home, "pimpo.pid")
	os.WriteFile(pid, []byte(strconv.Itoa(os.Getpid())), 0o600)
	defer os.Remove(pid)
	fmt.Printf("Pimpo %s is in recovery: choose a copy to go back to, or start fresh.\n\n  Open: %s\n\nData: %s\n", version, loginLink(ln.Addr().String(), rec.Token), home)
	httpSrv := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-done.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdown)
	}()
	if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	if ctx.Err() == nil {
		fmt.Println("Recovered. Starting Pimpo.")
	}
	return nil
}

// errRecovering stops commands that would open, and so recreate, the
// database that was set aside.
var errRecovering = errors.New("Pimpo is in recovery: its database was damaged and set aside. Open Pimpo to choose what to do, or `pimpo restore NAME` (see pimpo snapshots)")
