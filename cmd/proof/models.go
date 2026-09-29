package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/turbine-dev/pimpo/internal/llm"
)

// withModel sends every request to one model, for backends that take the
// model per request.
type withModel struct {
	llm.Model
	name string
}

func (w withModel) Generate(ctx context.Context, r llm.Request) (llm.Response, error) {
	if r.Model == "" {
		r.Model = w.name
	}
	return w.Model.Generate(ctx, r)
}

// keyEnv is where the proof reads a provider's API key.
func keyEnv(provider string) string { return "PIMPO_" + strings.ToUpper(provider) + "_KEY" }

// modelFor resolves -model like the app does: sonnet, opus or haiku through
// Claude Code, codex or codex:<model>, opencode:<provider>/<model>, or
// provider:name through its API, with the key in PIMPO_<PROVIDER>_KEY and
// the price from -price-in and -price-out.
func modelFor(id string, in, out float64) (llm.Model, error) {
	switch {
	case id == "sonnet" || id == "opus" || id == "haiku":
		return llm.ClaudeCLI{Model: id}, nil
	case id == "codex":
		return llm.CodexCLI{}, nil
	case strings.HasPrefix(id, "codex:"):
		return llm.CodexCLI{Model: strings.TrimPrefix(id, "codex:")}, nil
	case llm.IsOpencode(id):
		return withModel{llm.OpencodeCLI{}, id}, nil
	}
	provider, name, ok := strings.Cut(id, ":")
	if !ok || !slices.Contains(llm.Providers, provider) {
		return nil, fmt.Errorf("unknown model %q", id)
	}
	local := provider == "ollama" || provider == "lmstudio"
	key := os.Getenv(keyEnv(provider))
	if key == "" && !local {
		return nil, fmt.Errorf("set %s for %s", keyEnv(provider), id)
	}
	if !local && in == 0 && out == 0 {
		return nil, fmt.Errorf("give -price-in and -price-out for %s: a price is never guessed", id)
	}
	return llm.API{Provider: provider, Model: name, Key: key, Base: llm.Bases[provider], PriceIn: in, PriceOut: out}, nil
}
