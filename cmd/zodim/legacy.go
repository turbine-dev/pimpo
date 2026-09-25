package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// adoptLegacy moves the data of an install made before the rename (Zodim
// was called Vigia) into dir, and renames its database files, snapshots
// included. It does nothing once dir exists, and waits for the old app to
// be closed rather than moving files it still writes.
func adoptLegacy(old, dir string) error {
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	if _, err := os.Stat(old); err != nil {
		return nil
	}
	if alive(filepath.Join(old, "vigia.pid")) {
		return errors.New("Vigia (o nome antigo do Zodim) ainda está aberto. Saia dele pela barra de menu e abra o Zodim de novo.")
	}
	if err := os.Rename(old, dir); err != nil {
		return err
	}
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && d.Name() == "tailscale" {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), "vigia.") {
			os.Rename(p, filepath.Join(filepath.Dir(p), "zodim."+strings.TrimPrefix(d.Name(), "vigia.")))
		}
		return nil
	})
	return nil
}
