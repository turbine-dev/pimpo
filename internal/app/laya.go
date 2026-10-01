package app

import (
	"context"
	"net/http"
	"time"

	"github.com/turbine-dev/pimpo/internal/judge"
	"github.com/turbine-dev/pimpo/internal/server"
)

// Laya, Jev's open decision model, runs on this computer with laya-serve
// and answers judgments and company decisions with the same API as Jev.

// layaCheck says whether the Laya server at base answers, and how a
// simple judgment comes back.
func (a *App) layaCheck(ctx context.Context, base string) map[string]any {
	if base == "" {
		base = layaDefault
	}
	l := judge.Laya(base, func(ctx context.Context) (string, error) { return a.secret(ctx, "laya.key") })
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := l.Healthy(ctx); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	start := time.Now()
	ans, err := l.Ask(ctx, "Is this message asking for a refund?", "I was charged twice, please give my money back")
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true, "p": ans.P, "ms": time.Since(start).Milliseconds()}
}

func (a *App) layaRoutes() {
	a.Server.Handle("POST /api/judge/laya/test", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			URL string `json:"url"`
		}
		if err := server.Decode(r, &in); err != nil {
			server.WriteError(w, err)
			return
		}
		server.WriteJSON(w, 200, a.layaCheck(r.Context(), in.URL))
	})
}
