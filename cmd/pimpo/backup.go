package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/turbine-dev/pimpo/internal/backup"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/vault"
)

// passphrase comes from PIMPO_BACKUP_PASSPHRASE or is asked on the terminal.
func passphrase(in io.Reader, out io.Writer) string {
	if p := os.Getenv("PIMPO_BACKUP_PASSPHRASE"); p != "" {
		return p
	}
	fmt.Fprint(out, "Passphrase for the backup: ")
	line, _ := bufio.NewReader(in).ReadString('\n')
	return strings.TrimSpace(line)
}

func exportCmd(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	dir := fs.String("data", "", "data directory (default ~/.pimpo)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: pimpo export FILE.pimpo")
	}
	home := dataDir(*dir)
	store, err := event.Open(filepath.Join(home, "pimpo.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	v, err := vault.Open(store.DB(), vault.OSKey(home))
	if err != nil {
		return err
	}
	f, err := os.OpenFile(fs.Arg(0), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	m, err := backup.Export(context.Background(), store.DB(), home, v, passphrase(os.Stdin, out), version, f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(fs.Arg(0))
		return err
	}
	fmt.Fprintf(out, "Exported everything to %s (%d secrets), encrypted with your passphrase.\n", fs.Arg(0), m.Secrets)
	return nil
}

func importCmd(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	dir := fs.String("data", "", "data directory (default ~/.pimpo)")
	unsealed := fs.Bool("unsealed", false, "also take a backup that is not sealed as a whole (made by export before version 2 of the format); its database, memory and connectors cannot be checked")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: pimpo import [--unsealed] FILE.pimpo")
	}
	home := dataDir(*dir)
	if running(home) {
		return errors.New("Pimpo is running; import from Settings in the web app, or stop it first")
	}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	defer f.Close()
	stage := filepath.Join(home, "import-pending")
	os.RemoveAll(stage)
	unpack := backup.Unpack
	if *unsealed {
		fmt.Fprintln(out, "Warning: an unsealed backup cannot be checked. Anyone who had the file could have changed its database (people, paired devices), memory or connectors. Import it only if you are sure where it came from.")
		unpack = backup.UnpackUnsealed
	}
	m, secrets, err := unpack(f, stage, passphrase(os.Stdin, out))
	if err != nil {
		return err
	}
	key := vault.OSKey(home)
	if err := stageSecrets(stage, key, secrets); err != nil {
		os.RemoveAll(stage)
		return err
	}
	keep, err := applyImport(home, key)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Imported a backup from %s (%d secrets). What was here before is in %s.\n", m.Created.Local().Format("02/01/2006 15:04"), m.Secrets, keep)
	return nil
}
