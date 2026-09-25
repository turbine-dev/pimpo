package app

import (
	"archive/zip"
	"encoding/json"
	"io"
	"strings"

	"fmt"
	"github.com/denerFernandes/zodim/internal/connector/external"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/denerFernandes/zodim/internal/backup"
	"github.com/denerFernandes/zodim/internal/server"
)

func (a *App) backupRoutes() {
	a.Server.Handle("POST /api/backup/export", a.exportBackup)
	a.Server.Handle("POST /api/backup/import", a.importBackup)
	a.Server.Handle("POST /api/connectors/reload", a.reloadConnectors)
	a.Server.Handle("POST /api/connectors/install", a.installConnector)
}

func (a *App) exportBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Passphrase string `json:"passphrase"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	if len(req.Passphrase) < 8 {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "choose a passphrase of at least 8 characters"})
		return
	}
	name := "zodim-" + time.Now().Format("2006-01-02") + ".zodim"
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	if _, err := backup.Export(r.Context(), a.Events.DB(), a.Home, a.Vault, req.Passphrase, a.Version, w); err != nil {
		// Headers are gone by now; the download simply fails.
		return
	}
	a.Events.Append(r.Context(), "backup.exported", "human:owner", map[string]string{"file": name})
}

// importBackup checks and stages a backup; it takes effect when Zodim
// restarts, since the database cannot be swapped while it is open.
func (a *App) importBackup(w http.ResponseWriter, r *http.Request) {
	if a.Home == "" {
		server.WriteError(w, server.StatusError{Status: 503, Msg: "import is not available here"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<30)
	file, _, err := r.FormFile("file")
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "send the backup file"})
		return
	}
	defer file.Close()
	stage := filepath.Join(a.Home, "import-pending")
	os.RemoveAll(stage)
	m, secrets, err := backup.Unpack(file, stage, r.FormValue("passphrase"))
	if err != nil {
		os.RemoveAll(stage)
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	b, _ := json.Marshal(secrets)
	if err := os.WriteFile(filepath.Join(stage, "secrets.json"), b, 0o600); err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(r.Context(), "backup.staged", "human:owner", map[string]any{"created": m.Created, "secrets": m.Secrets})
	server.WriteJSON(w, 200, map[string]any{"created": m.Created, "version": m.Version, "secrets": m.Secrets, "restart": true})
}

// reloadConnectors picks up connectors copied into the connectors folder
// without restarting Zodim.
func (a *App) reloadConnectors(w http.ResponseWriter, r *http.Request) {
	if a.Home == "" {
		server.WriteError(w, server.StatusError{Status: 503, Msg: "no connectors folder"})
		return
	}
	a.mu.Lock()
	old := a.external
	a.external = nil
	a.mu.Unlock()
	for _, c := range old {
		c.Close()
	}
	a.AttachConnectors(filepath.Join(a.Home, "connectors"))
	a.mu.Lock()
	n, broken := len(a.external), append([]string{}, a.externalErrs...)
	a.mu.Unlock()
	server.WriteJSON(w, 200, map[string]any{"loaded": n, "broken": broken})
}

// installConnector takes a zip with a connector.json at its root (or in a
// single top folder), checks the manifest, and installs it.
func (a *App) installConnector(w http.ResponseWriter, r *http.Request) {
	if a.Home == "" {
		server.WriteError(w, server.StatusError{Status: 503, Msg: "no connectors folder"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	file, hdr, err := r.FormFile("file")
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "send the connector as a .zip"})
		return
	}
	defer file.Close()
	zr, err := zip.NewReader(file, hdr.Size)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "not a zip file"})
		return
	}
	tmp, err := os.MkdirTemp(a.Home, ".connector-")
	if err != nil {
		server.WriteError(w, err)
		return
	}
	defer os.RemoveAll(tmp)
	for _, f := range zr.File {
		name := filepath.Clean(filepath.FromSlash(f.Name))
		if filepath.IsAbs(name) || strings.HasPrefix(name, "..") || f.Mode()&os.ModeSymlink != 0 {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "unsafe path in zip: " + f.Name})
			return
		}
		p := filepath.Join(tmp, name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(p, 0o755)
			continue
		}
		os.MkdirAll(filepath.Dir(p), 0o755)
		rc, err := f.Open()
		if err != nil {
			server.WriteError(w, err)
			return
		}
		out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode().Perm()|0o600)
		if err == nil {
			_, err = io.Copy(out, io.LimitReader(rc, 64<<20))
			out.Close()
		}
		rc.Close()
		if err != nil {
			server.WriteError(w, err)
			return
		}
	}
	root := tmp
	if _, err := os.Stat(filepath.Join(root, "connector.json")); err != nil {
		if entries, _ := os.ReadDir(tmp); len(entries) == 1 && entries[0].IsDir() {
			root = filepath.Join(tmp, entries[0].Name())
		}
	}
	m, err := external.Load(root)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 422, Msg: err.Error()})
		return
	}
	dest := filepath.Join(a.Home, "connectors", m.Name)
	os.MkdirAll(filepath.Dir(dest), 0o700)
	os.RemoveAll(dest)
	if err := os.Rename(root, dest); err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(r.Context(), "connector.installed", "human:owner", map[string]string{"name": m.Name})
	a.reloadConnectors(w, r)
}
