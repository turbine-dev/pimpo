package app

import (
	"context"
	"net/http"
	"time"

	"github.com/turbine-dev/pimpo/internal/server"
)

// Laya, Jev's open decision model, runs on this computer with laya-serve
// and answers judgments and company decisions with the same API as Jev.

// layaCheck says whether the Laya server in the settings answers, and how
// a simple judgment comes back. It reaches only the saved address, never
// one a request names.
func (a *App) layaCheck(ctx context.Context) map[string]any {
	l := a.laya(ctx)
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
		server.WriteJSON(w, 200, a.layaCheck(r.Context()))
	})
}
