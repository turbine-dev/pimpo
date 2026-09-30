package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/snapshot"
)

// Recovery mode: the database failed its check at start and was set
// aside. Pimpo then serves only the recovery page, to the owner alone
// (their login link, a browser or device of theirs that was signed in,
// or the recovery link printed at start), until they choose: go back to
// the newest copy that passes its check, start fresh, or take the
// damaged file away. Every other API answers that Pimpo is recovering.

// RecoveryServer is the server of recovery mode. pinned is the owner's
// fixed token (PIMPO_TOKEN), if any; done is called once a choice has put
// a database in place, so Pimpo can start normally.
func RecoveryServer(home string, rec snapshot.Recovery, pinned string, done func()) (*server.Server, error) {
	// The server wants an event log; the real one is what is damaged.
	blank, err := event.Open(":memory:")
	if err != nil {
		return nil, err
	}
	master := pinned
	if master == "" {
		master = rec.Token
	}
	s := server.New(blank, master)
	token, hashes := rec.Salvage(home)
	s.Device = func(t string) (string, bool) {
		if t == "" {
			return "", false
		}
		ok := 0
		for _, k := range []string{rec.Token, token} {
			if k != "" {
				ok |= subtle.ConstantTimeCompare([]byte(t), []byte(k))
			}
		}
		h := sha256.Sum256([]byte(t))
		for _, d := range hashes {
			ok |= subtle.ConstantTimeCompare([]byte(hex.EncodeToString(h[:])), []byte(d))
		}
		return people.OwnerID, ok == 1
	}
	var once sync.Once
	finish := func() { once.Do(done) }
	s.Handle("GET /api/recovery", func(w http.ResponseWriter, r *http.Request) {
		list, _ := snapshot.Checked(home)
		newest := ""
		for _, sn := range list {
			if !sn.Damaged {
				newest = sn.Name
				break
			}
		}
		server.WriteJSON(w, 200, map[string]any{"when": rec.When, "reason": rec.Reason, "folder": "quarantine/" + rec.Folder, "snapshots": list, "newest_good": newest})
	})
	s.Handle("POST /api/recovery/restore", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		if err := rec.RecoverFrom(home, req.Name); err != nil {
			status := 404
			if errors.Is(err, event.ErrDamaged) {
				status = 409
			}
			server.WriteError(w, server.StatusError{Status: status, Msg: err.Error()})
			return
		}
		server.WriteJSON(w, 200, map[string]any{"restored": req.Name})
		finish()
	})
	s.Handle("POST /api/recovery/fresh", func(w http.ResponseWriter, r *http.Request) {
		if err := rec.StartFresh(home); err != nil {
			server.WriteError(w, err)
			return
		}
		server.WriteJSON(w, 200, map[string]any{"fresh": true})
		finish()
	})
	s.Handle("GET /api/recovery/export", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="pimpo-damaged-`+rec.Folder+`.zip"`)
		rec.Export(home, w)
	})
	s.HandlePublic("/api/", func(w http.ResponseWriter, r *http.Request) {
		server.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "Pimpo is recovering its database", "recovery": true})
	})
	return s, nil
}

// Database damage found while running, such as before a snapshot, is
// kept here for the check-up and the state, and told to the owner once.

func (a *App) damage() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.damaged
}

// ReportDamage records that the database failed its check.
func (a *App) ReportDamage(ctx context.Context, err error) {
	if err == nil || !errors.Is(err, event.ErrDamaged) {
		return
	}
	a.mu.Lock()
	first := a.damaged == ""
	a.damaged = err.Error()
	a.mu.Unlock()
	if !first {
		return
	}
	a.Events.Append(ctx, "database.damaged", "system", map[string]string{"error": err.Error()})
	a.Channel.Notify(ctx, explore.Notice{Kind: "failure", Text: i18n.T(ctx, "msg.dbDamaged")})
}

// Snapshot takes a snapshot, reporting a damaged database instead.
func (a *App) Snapshot(ctx context.Context, label string) (snapshot.Snapshot, error) {
	s, err := snapshot.Create(a.Events.DB(), a.Home, label)
	a.ReportDamage(ctx, err)
	return s, err
}
