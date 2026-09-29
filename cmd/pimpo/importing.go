package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/turbine-dev/pimpo/internal/backup"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/vault"
)

// A staged import is a backup already unpacked (and its passphrase already
// checked) into home/import-pending, waiting for a moment when nothing has
// the database open: now, from the CLI, or at the next start after an
// import from the web app. Its secrets wait there only until then.

func saveStagedSecrets(stage string, secrets map[string]string) error {
	b, _ := json.Marshal(secrets)
	return os.WriteFile(filepath.Join(stage, "secrets.json"), b, 0o600)
}

// applyImport swaps the staged backup in and fills the local vault.
func applyImport(home string) (string, error) {
	stage := filepath.Join(home, "import-pending")
	raw, err := os.ReadFile(filepath.Join(stage, "secrets.json"))
	if err != nil {
		return "", err
	}
	var secrets map[string]string
	json.Unmarshal(raw, &secrets)
	os.Remove(filepath.Join(stage, "secrets.json"))
	keep, err := backup.Place(stage, home)
	if err != nil {
		return keep, err
	}
	os.RemoveAll(stage)
	store, err := event.Open(filepath.Join(home, "pimpo.db"))
	if err != nil {
		return keep, err
	}
	defer store.Close()
	v, err := vault.Open(store.DB(), vault.OSKey(home))
	if err != nil {
		return keep, err
	}
	ctx := context.Background()
	for k, val := range secrets {
		if err := v.Set(ctx, k, val); err != nil {
			return keep, err
		}
	}
	store.Append(ctx, "backup.imported", "human:owner", map[string]any{"secrets": len(secrets)})
	return keep, nil
}

func pendingImport(home string) bool {
	_, err := os.Stat(filepath.Join(home, "import-pending", "secrets.json"))
	return err == nil
}
