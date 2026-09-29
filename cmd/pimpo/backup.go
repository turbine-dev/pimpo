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
	fmt.Fprint(out, "Passphrase for the backup's secrets: ")
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
	fmt.Fprintf(out, "Exported everything to %s (%d secrets, encrypted with your passphrase).\n", fs.Arg(0), m.Secrets)
	return nil
}

func importCmd(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	dir := fs.String("data", "", "data directory (default ~/.pimpo)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: pimpo import FILE.pimpo")
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
	m, secrets, err := backup.Unpack(f, stage, passphrase(os.Stdin, out))
	if err != nil {
		os.RemoveAll(stage)
		return err
	}
	if err := saveStagedSecrets(stage, secrets); err != nil {
		return err
	}
	keep, err := applyImport(home)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Imported a backup from %s (%d secrets). What was here before is in %s.\n", m.Created.Local().Format("02/01/2006 15:04"), m.Secrets, keep)
	return nil
}
