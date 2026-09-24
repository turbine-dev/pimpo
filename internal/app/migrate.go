package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/denerFernandes/vigia/internal/memory"
	"github.com/denerFernandes/vigia/internal/migrate"
	"github.com/denerFernandes/vigia/internal/server"
	"github.com/denerFernandes/vigia/internal/store"
)

// ImportOptions says which parts of a plan to bring over.
type ImportOptions struct {
	Memories bool `json:"memories"`
	Rules    bool `json:"rules"`
	Tasks    bool `json:"tasks"`
	// Secrets copies the Telegram token and mail password into the vault.
	Secrets bool `json:"secrets"`
	// Trust marks imported memories and rules as the owner's own words.
	// Off by default: the other agent wrote much of its memory itself.
	Trust bool `json:"trust"`
}

type Imported struct {
	Memories int  `json:"memories"`
	Rules    int  `json:"rules"`
	Tasks    int  `json:"tasks"`
	Telegram bool `json:"telegram"`
	Mail     bool `json:"mail"`
}

func (a *App) Import(ctx context.Context, p migrate.Plan, o ImportOptions, actor string) (Imported, error) {
	var n Imported
	trust := memory.Low
	if o.Trust {
		trust = memory.High
	}
	if (o.Memories || o.Rules) && a.Memory == nil {
		return n, errors.New("memory is not available")
	}
	source := "import:" + p.From
	if o.Memories {
		for _, m := range p.Memories {
			if _, err := a.Memory.Add(m.Text, m.Topic, source, trust); err != nil {
				return n, err
			}
			n.Memories++
		}
	}
	if o.Rules {
		for _, r := range p.Rules {
			if _, err := a.Memory.Add(r.Text, "regras ("+r.File+")", source, trust); err != nil {
				return n, err
			}
			n.Rules++
		}
	}
	if o.Tasks {
		for _, t := range p.Tasks {
			summary := "Trazida do " + p.From + ": " + t.Name
			if !t.Enabled {
				summary += " (estava pausada)"
			}
			e := store.Exploration{ID: importID(), Request: t.Request(), State: store.ExplorationImported, Summary: summary}
			if err := a.Store.SaveExploration(ctx, e); err != nil {
				return n, err
			}
			n.Tasks++
		}
	}
	if o.Secrets {
		if p.Telegram.Token != "" {
			if err := a.Vault.Set(ctx, "telegram.token", p.Telegram.Token); err != nil {
				return n, err
			}
			n.Telegram = true
		}
		if p.Mail.IMAP != "" && p.Mail.Password != "" {
			a.Events.Put(ctx, "mail.addr", p.Mail.IMAP)
			a.Events.Put(ctx, "mail.user", p.Mail.Address)
			if p.Mail.SMTP != "" {
				a.Events.Put(ctx, "mail.smtp", p.Mail.SMTP)
			}
			if err := a.Vault.Set(ctx, "mail.password", p.Mail.Password); err != nil {
				return n, err
			}
			n.Mail = true
		}
	}
	_, err := a.Events.Append(ctx, "migration.imported", actor, map[string]any{"from": p.From, "home": p.Home, "imported": n})
	return n, err
}

func importID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (a *App) migrateRoutes() {
	a.Server.Handle("POST /api/migrate/preview", a.migratePreview)
	a.Server.Handle("POST /api/migrate/apply", a.migrateApply)
}

type migrateRequest struct {
	From string `json:"from"`
	Home string `json:"home"`
	ImportOptions
}

func (a *App) readPlan(w http.ResponseWriter, r *http.Request) (migrate.Plan, migrateRequest, bool) {
	var req migrateRequest
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return migrate.Plan{}, req, false
	}
	p, err := migrate.Read(req.From, req.Home)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 404, Msg: err.Error()})
		return p, req, false
	}
	return p, req, true
}

func (a *App) migratePreview(w http.ResponseWriter, r *http.Request) {
	if p, _, ok := a.readPlan(w, r); ok {
		server.WriteJSON(w, 200, p)
	}
}

func (a *App) migrateApply(w http.ResponseWriter, r *http.Request) {
	p, req, ok := a.readPlan(w, r)
	if !ok {
		return
	}
	n, err := a.Import(r.Context(), p, req.ImportOptions, "human:owner")
	if err != nil {
		server.WriteError(w, err)
		return
	}
	server.WriteJSON(w, 200, n)
}
