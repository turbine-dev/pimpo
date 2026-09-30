package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/vault"
)

// Staged secrets are encrypted at rest and survive a start that fails
// before they reach the vault.
func TestStagedImportKeepsSecretsUntilTheyAreInTheVault(t *testing.T) {
	home := t.TempDir()
	stage := filepath.Join(home, "import-pending")
	ev, err := event.Open(filepath.Join(stage, "pimpo.db"))
	if err != nil {
		t.Fatal(err)
	}
	ev.Put(context.Background(), "marker", "imported")
	ev.Close()
	key := vault.FileKey(filepath.Join(home, "vault.key"))
	if err := stageSecrets(stage, key, map[string]string{"telegram.token": "123:secret"}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(stage)
	for _, e := range entries {
		if b, _ := os.ReadFile(filepath.Join(stage, e.Name())); bytes.Contains(b, []byte("123:secret")) {
			t.Fatalf("%s holds a secret in plain text", e.Name())
		}
	}
	if !pendingImport(home) {
		t.Fatal("not pending")
	}

	locked := func(bool) ([]byte, error) { return nil, errors.New("keychain locked") }
	if _, err := applyImport(home, locked); err == nil {
		t.Fatal("finished without a vault")
	}
	if !pendingImport(home) {
		t.Fatal("the secrets were dropped before reaching the vault")
	}

	if _, err := applyImport(home, key); err != nil {
		t.Fatal(err)
	}
	if pendingImport(home) {
		t.Fatal("still pending")
	}
	if _, err := os.Stat(stage); err == nil {
		t.Fatal("the stage was left behind")
	}
	store, _ := event.Open(filepath.Join(home, "pimpo.db"))
	defer store.Close()
	if v, _ := store.Get(context.Background(), "marker"); v != "imported" {
		t.Fatal("the database was not placed")
	}
	v, _ := vault.Open(store.DB(), key)
	if tok, _ := v.Get(context.Background(), "telegram.token"); tok != "123:secret" {
		t.Fatal("secret lost")
	}
}
