// Command zodim runs the personal agent: the local server, the web UI and
// the Telegram channel.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/denerFernandes/zodim/internal/app"
	"github.com/denerFernandes/zodim/internal/desktop"
	"github.com/denerFernandes/zodim/internal/event"
	"github.com/denerFernandes/zodim/internal/snapshot"
	"github.com/denerFernandes/zodim/internal/vault"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "zodim:", err)
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
	case "snapshot", "snapshots", "restore":
		return snapshots(cmd, args)
	case "migrate":
		return migrateCmd(args, os.Stdout)
	case "gallery":
		return galleryCmd(args, os.Stdout)
	case "connector":
		return connectorCmd(args, os.Stdout)
	case "protect":
		return protectCmd(args, os.Stdout)
	case "export":
		return exportCmd(args, os.Stdout)
	case "import":
		return importCmd(args, os.Stdout)
	case "version":
		fmt.Println(version)
		return nil
	}
	return fmt.Errorf("unknown command %q (try: serve, export, import, protect, migrate, gallery, connector, snapshot, snapshots, restore, version)", cmd)
}

func dataDir(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if d := os.Getenv("ZODIM_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".zodim")
	if err := adoptLegacy(filepath.Join(home, ".vigia"), dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return dir
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:7788", "listen address (loopback only unless you know why)")
	dir := fs.String("data", "", "data directory (default ~/.zodim)")
	demoMode := fs.Bool("demo", false, "try Zodim with a demo mailbox and calendar, no accounts and no model costs")
	if err := fs.Parse(args); err != nil {
		return err
	}
	home := dataDir(*dir)
	if *demoMode && *dir == "" {
		home += "-demo"
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	if pendingImport(home) {
		keep, err := applyImport(home)
		if err != nil {
			return fmt.Errorf("finishing the import: %w", err)
		}
		fmt.Printf("Imported the backup. What was here before is in %s.\n", keep)
	}
	if name := snapshot.Staged(home); name != "" {
		if err := applyRestore(home, name); err != nil {
			return fmt.Errorf("restoring %s: %w", name, err)
		}
		fmt.Printf("Restored %s. What was here before is a snapshot too.\n", name)
	}
	store, err := event.Open(filepath.Join(home, "zodim.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if os.Getenv("ZODIM_DESKTOP_NOTIFY") != "" {
		desktop.KnownPath()
		go desktop.ShellPath()
	}
	if os.Getenv("ZODIM_EXIT_WITH_PARENT") != "" {
		go exitWithParent(ctx, stop)
	}
	if err := guardVersion(ctx, store, home); err != nil {
		return err
	}
	os.WriteFile(filepath.Join(home, "zodim.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600)
	defer os.Remove(filepath.Join(home, "zodim.pid"))
	go dailySnapshots(ctx, store, home)

	token, err := sessionToken(ctx, store)
	if err != nil {
		return err
	}
	v, err := vault.Open(store.DB(), vault.OSKey(home))
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	a, err := app.New(ctx, store, v, token, "http://"+ln.Addr().String())
	if err != nil {
		return err
	}
	if err := a.AttachMemory(filepath.Join(home, "memory")); err != nil {
		return err
	}
	a.AttachConnectors(filepath.Join(home, "connectors"))
	a.VoiceModel = filepath.Join(home, "models", "ggml-base.bin")
	a.Home, a.Version = home, version
	if !*demoMode {
		a.AttachRemote(home, nil)
	}
	a.DesktopNotify = os.Getenv("ZODIM_DESKTOP_NOTIFY") != ""
	a.TelegramAPI = os.Getenv("ZODIM_TELEGRAM_API")
	if *demoMode {
		a.EnableDemo(ctx, 700*time.Millisecond)
		fmt.Println("Demo mode: a sample mailbox and calendar, a scripted agent, no model costs.")
	}
	if err := a.Start(ctx); err != nil {
		return err
	}
	srv := a.Server
	fmt.Printf("Zodim %s is running.\n\n  Open: http://%s/auth?token=%s\n\nData: %s\n", version, ln.Addr(), token, home)
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

// exitWithParent stops the server when the desktop app that started it
// dies, however it dies: the process is then adopted by another parent.
func exitWithParent(ctx context.Context, stop func()) {
	parent := os.Getppid()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if os.Getppid() != parent {
				stop()
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// guardVersion snapshots the data before a new version touches it.
func guardVersion(ctx context.Context, s *event.Store, home string) error {
	last, _ := s.Get(ctx, "last_version")
	if last != "" && last != version {
		snap, err := snapshot.Create(s.DB(), home, "before-"+version)
		if err != nil {
			return fmt.Errorf("could not snapshot before updating from %s: %w", last, err)
		}
		fmt.Printf("Updated from %s to %s. Snapshot %s keeps the old data; `zodim restore %s` brings it back.\n", last, version, snap.Name, snap.Name)
		b, _ := json.Marshal(map[string]string{"from": last, "to": version, "snapshot": snap.Name})
		s.Put(ctx, app.UpgradeKey, string(b))
	}
	return s.Put(ctx, "last_version", version)
}

// applyRestore puts back a snapshot the app staged, before anything opens
// the database.
func applyRestore(home, name string) error {
	store, err := event.Open(filepath.Join(home, "zodim.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	defer snapshot.Unstage(home)
	return snapshot.Restore(home, name, store.DB())
}

func dailySnapshots(ctx context.Context, s *event.Store, home string) {
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			snapshot.Create(s.DB(), home, "daily")
		case <-ctx.Done():
			return
		}
	}
}

func running(home string) bool { return alive(filepath.Join(home, "zodim.pid")) }

func alive(pidFile string) bool {
	b, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return false
	}
	p, err := os.FindProcess(pid)
	return err == nil && p.Signal(syscall.Signal(0)) == nil
}

func snapshots(cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	dir := fs.String("data", "", "data directory (default ~/.zodim)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	home := dataDir(*dir)
	switch cmd {
	case "snapshots":
		list, err := snapshot.List(home)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Println("No snapshots yet.")
		}
		for _, s := range list {
			fmt.Printf("%s  %s  %.1f MB\n", s.Name, s.When.Local().Format("02/01/2006 15:04"), float64(s.Bytes)/1e6)
		}
		return nil
	case "snapshot":
		store, err := event.Open(filepath.Join(home, "zodim.db"))
		if err != nil {
			return err
		}
		defer store.Close()
		s, err := snapshot.Create(store.DB(), home, strings.Join(fs.Args(), " "))
		if err != nil {
			return err
		}
		fmt.Println("Saved", s.Name)
		return nil
	case "restore":
		if fs.NArg() != 1 {
			return errors.New("usage: zodim restore NAME (see zodim snapshots)")
		}
		if running(home) {
			return errors.New("stop Zodim before restoring")
		}
		store, err := event.Open(filepath.Join(home, "zodim.db"))
		if err != nil {
			return err
		}
		if err := snapshot.Restore(home, fs.Arg(0), store.DB()); err != nil {
			return err
		}
		fmt.Println("Restored", fs.Arg(0), "- your previous state was saved as a snapshot too.")
		return nil
	}
	return nil
}

// sessionToken is created once and kept, so the login link stays valid
// across restarts until the user rotates it.
func sessionToken(ctx context.Context, s *event.Store) (string, error) {
	// Browser tests pin the token so they can log in.
	if t := os.Getenv("ZODIM_TOKEN"); t != "" {
		return t, s.Put(ctx, "session_token", t)
	}
	if t, err := s.Get(ctx, "session_token"); err != nil || t != "" {
		return t, err
	}
	b := make([]byte, 24)
	rand.Read(b)
	t := hex.EncodeToString(b)
	return t, s.Put(ctx, "session_token", t)
}
