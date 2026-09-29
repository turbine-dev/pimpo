package app

import (
	"context"
	"encoding/json"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"net/http"
	"os"
	"path/filepath"

	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/snapshot"
)

// UpgradeKey holds {from, to, snapshot} after Pimpo starts on a new
// version, until the owner has been told.
const UpgradeKey = "upgrade.notice"

// Local snapshots: Pimpo copies its data aside every day, before an
// update and before an import. Any of them can be put back; it takes
// effect when Pimpo restarts, since the database is open while it runs.
func (a *App) snapshotRoutes() {
	a.Server.Handle("GET /api/snapshots", a.listSnapshots)
	a.Server.Handle("POST /api/snapshots", a.createSnapshot)
	a.Server.Handle("POST /api/snapshots/restore", a.stageRestore)
	a.Server.Handle("DELETE /api/snapshots/restore", a.cancelRestore)
}

func (a *App) listSnapshots(w http.ResponseWriter, r *http.Request) {
	if a.Home == "" {
		server.WriteJSON(w, 200, map[string]any{"snapshots": []snapshot.Snapshot{}, "available": false})
		return
	}
	list, err := snapshot.List(a.Home)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	server.WriteJSON(w, 200, map[string]any{"snapshots": list, "staged": snapshot.Staged(a.Home), "available": true, "version": a.Version})
}

func (a *App) createSnapshot(w http.ResponseWriter, r *http.Request) {
	if a.Home == "" {
		server.WriteError(w, server.StatusError{Status: 503, Msg: "snapshots are not available here"})
		return
	}
	s, err := snapshot.Create(a.Events.DB(), a.Home, "manual")
	if err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(r.Context(), "snapshot.created", "human:owner", map[string]string{"name": s.Name})
	server.WriteJSON(w, 200, s)
}

func (a *App) stageRestore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	if a.Home == "" {
		server.WriteError(w, server.StatusError{Status: 503, Msg: "snapshots are not available here"})
		return
	}
	if _, err := os.Stat(filepath.Join(a.Home, "import-pending")); err == nil {
		server.WriteError(w, server.StatusError{Status: 409, Msg: "a backup import is already waiting for the restart"})
		return
	}
	if err := snapshot.Stage(a.Home, req.Name); err != nil {
		server.WriteError(w, server.StatusError{Status: 404, Msg: err.Error()})
		return
	}
	a.Events.Append(r.Context(), "snapshot.staged", "human:owner", map[string]string{"name": req.Name})
	server.WriteJSON(w, 200, map[string]any{"staged": req.Name, "restart": true})
}

func (a *App) cancelRestore(w http.ResponseWriter, r *http.Request) {
	if a.Home != "" {
		snapshot.Unstage(a.Home)
	}
	server.WriteJSON(w, 200, map[string]any{"staged": ""})
}

// announceUpgrade tells the owner once that Pimpo was updated and that the
// state from before is kept.
func (a *App) announceUpgrade(ctx context.Context) {
	raw, _ := a.Events.Get(ctx, UpgradeKey)
	if raw == "" {
		return
	}
	a.Events.Put(ctx, UpgradeKey, "")
	var u struct{ From, To string }
	if json.Unmarshal([]byte(raw), &u) != nil {
		return
	}
	a.Channel.Notify(ctx, explore.Notice{Kind: "backup",
		Text: i18n.T(ctx, "msg.upgrade", "from", u.From, "to", u.To)})
}
