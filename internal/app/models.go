package app

import (
	"time"

	"context"
	"github.com/turbine-dev/pimpo/internal/models"
	"net/http"
	"slices"
	"strings"

	"github.com/turbine-dev/pimpo/internal/budget"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/server"
)

// API keys for model providers live in the vault; the models and their
// prices are part of the settings.
var sharedModels = &models.Client{}

// modelClient finds models; tests swap it for fakes.
func (a *App) modelClient() *models.Client {
	if a.Models != nil {
		return a.Models
	}
	return sharedModels
}

func (a *App) modelRoutes() {
	a.Server.Handle("GET /api/models", func(w http.ResponseWriter, r *http.Request) {
		keys := map[string]bool{}
		for _, p := range llm.Providers {
			v, _ := a.secret(r.Context(), "model."+p+".key")
			keys[p] = v != ""
		}
		light, strong := a.autoModels(r.Context())
		weigher := "rules"
		if _, ok := tierChooser(a); ok {
			weigher = "jev"
		}
		auto := map[string]string{"light": light, "strong": strong, "base": a.Settings(r.Context()).ExploreModel, "weigher": weigher}
		server.WriteJSON(w, 200, map[string]any{"keys": keys, "claude_code": claudeInstalled(), "providers": models.Providers, "auto": auto})
	})
	// opencode lists the models of the providers signed in to in opencode.
	a.Server.Handle("GET /api/models/opencode", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		list, err := llm.OpencodeModels(ctx)
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 404, Msg: err.Error()})
			return
		}
		if list == nil {
			list = []llm.OpencodeModel{}
		}
		server.WriteJSON(w, 200, list)
	})
	// detect finds Claude Code and local model servers on this computer.
	a.Server.Handle("GET /api/models/detect", func(w http.ResponseWriter, r *http.Request) {
		s := a.Settings(r.Context())
		server.WriteJSON(w, 200, a.modelClient().Detect(r.Context(), s.OllamaURL, s.LMStudioURL))
	})
	// catalog lists what a provider offers, from the provider itself (kept
	// a day; ?fresh=1 asks again), priced where OpenRouter knows, with the
	// owner's own models and prices, new and retired ones marked.
	a.Server.Handle("GET /api/models/catalog/{provider}", func(w http.ResponseWriter, r *http.Request) {
		p := r.PathValue("provider")
		if _, ok := models.Get(p); !ok {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "unknown provider"})
			return
		}
		if _, err := a.modelEndpoint(r.Context(), p); err != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
			return
		}
		list, err := a.catalog(r.Context(), p, r.URL.Query().Get("fresh") == "1")
		if err != nil {
			server.WriteJSON(w, 502, map[string]string{"error": err.Error(), "problem": models.Problem(err)})
			return
		}
		server.WriteJSON(w, 200, list)
	})
	// retired lists the house's models their providers stopped offering,
	// so a chat or routine using one can suggest another. Model names only.
	a.Server.Handle("GET /api/models/retired", func(w http.ResponseWriter, r *http.Request) {
		server.WriteJSON(w, 200, map[string][]string{"retired": a.retiredModels(r.Context())})
	})
	a.Server.Handle("PUT /api/models/keys/{provider}", func(w http.ResponseWriter, r *http.Request) {
		if !ownerOnly(w, r) {
			return
		}
		p := r.PathValue("provider")
		if prov, ok := models.Get(p); !ok || prov.Local {
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
	// test asks a model for one word, capped at one cent, and says how long
	// it took or, in plain kinds, why it failed. A model not added yet is
	// tested with the price the owner is about to accept.
	a.Server.Handle("POST /api/models/test", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID       string   `json:"id"`
			PriceIn  *float64 `json:"price_in"`
			PriceOut *float64 `json:"price_out"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		var api llm.API
		var ok bool
		var err error
		if provider, name, found := strings.Cut(req.ID, ":"); found && req.PriceIn != nil && req.PriceOut != nil && slices.Contains(llm.Providers, provider) {
			api, ok, err = a.apiFor(r.Context(), provider, name, *req.PriceIn, *req.PriceOut)
		} else {
			api, ok, err = a.apiModel(r.Context(), req.ID)
		}
		if !ok {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "only API models are tested here"})
			return
		}
		start := time.Now()
		if err == nil {
			var resp llm.Response
			resp, err = api.Generate(context.WithoutCancel(r.Context()), llm.Request{Prompt: "Answer with the single word: ok", MaxCostUSD: 0.01})
			if err == nil {
				a.Budget.Record(r.Context(), budget.Cost{USD: resp.CostUSD, Source: "model test", Ref: req.ID})
				server.WriteJSON(w, 200, map[string]any{"ok": true, "text": strings.TrimSpace(resp.Text), "cost_usd": resp.CostUSD, "ms": time.Since(start).Milliseconds()})
				return
			}
		}
		server.WriteJSON(w, 502, map[string]any{"ok": false, "error": err.Error(), "problem": models.Problem(err)})
	})
}
