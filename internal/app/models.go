package app

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/denerFernandes/zodim/internal/budget"
	"github.com/denerFernandes/zodim/internal/llm"
	"github.com/denerFernandes/zodim/internal/server"
)

// API keys for model providers live in the vault; the models and their
// prices are part of the settings.
func (a *App) modelRoutes() {
	a.Server.Handle("GET /api/models", func(w http.ResponseWriter, r *http.Request) {
		keys := map[string]bool{}
		for _, p := range llm.Providers {
			v, _ := a.secret(r.Context(), "model."+p+".key")
			keys[p] = v != ""
		}
		server.WriteJSON(w, 200, map[string]any{"keys": keys, "claude_code": claudeInstalled()})
	})
	a.Server.Handle("PUT /api/models/keys/{provider}", func(w http.ResponseWriter, r *http.Request) {
		if !ownerOnly(w, r) {
			return
		}
		p := r.PathValue("provider")
		if !slices.Contains(llm.Providers, p) || p == "ollama" {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "unknown provider"})
			return
		}
		var req struct {
			Key string `json:"key"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		key := strings.TrimSpace(req.Key)
		var err error
		if key == "" {
			err = a.Vault.Delete(r.Context(), "model."+p+".key")
		} else {
			err = a.Vault.Set(r.Context(), "model."+p+".key", key)
		}
		if err != nil {
			server.WriteError(w, err)
			return
		}
		a.Events.Append(r.Context(), "model.key", actor(r.Context()), map[string]any{"provider": p, "set": key != ""})
		server.WriteJSON(w, 200, map[string]bool{"set": key != ""})
	})
	// test asks a model for one word, capped at one cent.
	a.Server.Handle("POST /api/models/test", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"id"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		api, ok, err := a.apiModel(r.Context(), req.ID)
		if !ok {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "only API models are tested here"})
			return
		}
		if err == nil {
			var resp llm.Response
			resp, err = api.Generate(context.WithoutCancel(r.Context()), llm.Request{Prompt: "Answer with the single word: ok", MaxCostUSD: 0.01})
			if err == nil {
				a.Budget.Record(r.Context(), budget.Cost{USD: resp.CostUSD, Source: "model test", Ref: req.ID})
				server.WriteJSON(w, 200, map[string]any{"ok": true, "text": strings.TrimSpace(resp.Text), "cost_usd": resp.CostUSD})
				return
			}
		}
		server.WriteError(w, server.StatusError{Status: 502, Msg: err.Error()})
	})
}
