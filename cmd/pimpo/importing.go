package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/turbine-dev/pimpo/internal/backup"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/vault"
)

// A staged import is a backup already unpacked and checked into
// home/import-pending, waiting for a moment when nothing has the database
// open: now, from the CLI, or at the next start after an import from the
// web app. Its secrets wait there encrypted with this machine's vault key,
// and the folder goes only once they are in the vault.

// stageSecrets encrypts the secrets of an unpacked backup with the vault
// key of home. It opens the vault on a throwaway database, since the key
// belongs to the machine, not to a database.
func stageSecrets(stage string, key vault.KeySource, secrets map[string]string) error {
	tmp, err := event.Open(filepath.Join(stage, ".key-check.db"))
	if err != nil {
		return err
	}
	defer func() {
		tmp.Close()
		for _, s := range []string{"", "-wal", "-shm"} {
			os.Remove(filepath.Join(stage, ".key-check.db"+s))
		}
	}()
	v, err := vault.Open(tmp.DB(), key)
	if err != nil {
		return err
	}
	return backup.SaveStaged(stage, v, secrets)
}

// applyImport swaps the staged backup in and fills the local vault.
func applyImport(home string, key vault.KeySource) (string, error) {
	stage := filepath.Join(home, "import-pending")
	keep := ""
	// A start that stopped halfway has already placed the files.
	if _, err := os.Stat(filepath.Join(stage, "pimpo.db")); err == nil {
		if keep, err = backup.Place(stage, home); err != nil {
			return keep, err
		}
	}
	store, err := event.Open(filepath.Join(home, "pimpo.db"))
	if err != nil {
		return keep, err
	}
	defer store.Close()
	v, err := vault.Open(store.DB(), key)
	if err != nil {
		return keep, err
	}
	secrets, err := backup.LoadStaged(stage, v)
	if err != nil {
		return keep, err
	}
	ctx := context.Background()
	for k, val := range secrets {
		if err := v.Set(ctx, k, val); err != nil {
			return keep, err
		}
	}
	if err := os.RemoveAll(stage); err != nil {
		return keep, err
	}
	store.Append(ctx, "backup.imported", "human:owner", map[string]any{"secrets": len(secrets)})
	return keep, nil
}

func pendingImport(home string) bool {
	return backup.Pending(filepath.Join(home, "import-pending"))
}
