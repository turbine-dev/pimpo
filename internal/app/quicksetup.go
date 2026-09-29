package app

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/budget"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/models"
	"github.com/turbine-dev/pimpo/internal/server"
)

// Quick setup picks the models for every job from what the owner already
// has (Claude Code, Codex, opencode, Ollama, LM Studio) or from one API
// key, tests the main one with a single word, and saves the choice. It is
// what the welcome screen uses, so a first routine needs no Claude Code.

func (a *App) quickSetupRoutes() {
	a.Server.Handle("POST /api/setup/model", a.quickSetup)
}

type quickChoice struct {
	// Kind is claude_code, codex, opencode, ollama, lmstudio or provider.
	Kind     string `json:"kind"`
	Provider string `json:"provider"`
	Key      string `json:"key"`
	// Model is the model to use, for opencode, Ollama and LM Studio.
	Model string `json:"model"`
}

type quickResult struct {
	Explore string  `json:"explore"`
	Judge   string  `json:"judge"`
	Text    string  `json:"text,omitempty"`
	CostUSD float64 `json:"cost_usd"`
	MS      int64   `json:"ms,omitempty"`
}

func (a *App) quickSetup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ownerOnly(w, r) {
		return
	}
	var c quickChoice
	if err := server.Decode(r, &c); err != nil {
		server.WriteError(w, err)
		return
	}
	bad := func(status int, msg string) { server.WriteError(w, server.StatusError{Status: status, Msg: msg}) }
	s := a.Settings(ctx)
	var main, light models.Model
	switch c.Kind {
	case "claude_code":
		main, light = models.Model{ID: "sonnet"}, models.Model{ID: "haiku"}
	case "codex":
		main, light = models.Model{ID: "codex"}, models.Model{ID: "codex"}
	case "opencode":
		if !llm.IsOpencode(c.Model) {
			bad(400, "choose an opencode model")
			return
		}
		main, light = models.Model{ID: c.Model}, models.Model{ID: c.Model}
	case "ollama", "lmstudio":
		name := strings.TrimSpace(c.Model)
		if name == "" {
			bad(400, "choose a model")
			return
		}
		main = models.Model{ID: c.Kind + ":" + name, Priced: true}
		light = main
	case "provider":
		p, ok := models.Get(c.Provider)
		if !ok || p.Local || c.Provider == "custom" {
			bad(400, "unknown provider")
			return
		}
		if key := strings.TrimSpace(c.Key); key != "" {
			if err := a.Vault.Set(ctx, "model."+c.Provider+".key", key); err != nil {
				server.WriteError(w, err)
				return
			}
			a.Events.Append(ctx, "model.key", actor(ctx), map[string]any{"provider": c.Provider, "set": true})
		}
		e, err := a.modelEndpoint(ctx, c.Provider)
		if err != nil {
			bad(400, err.Error())
			return
		}
		list, err := a.modelClient().List(ctx, e)
		if err != nil {
			server.WriteJSON(w, 502, map[string]string{"error": err.Error(), "problem": models.Problem(err)})
			return
		}
		var found bool
		if main, light, found = recommend(c.Provider, list); !found {
			bad(422, "Pimpo could not tell which of this provider's models to use; choose them in Ajustes › Modelos")
			return
		}
		for _, m := range []models.Model{main, light} {
			if !m.Priced {
				// A price is never guessed.
				bad(422, "the price of "+m.ID+" is unknown; choose the models and type their price in Ajustes › Modelos")
				return
			}
		}
		main.ID, light.ID = c.Provider+":"+main.ID, c.Provider+":"+light.ID
	default:
		bad(400, "unknown choice")
		return
	}

	res := quickResult{Explore: main.ID, Judge: light.ID}
	for _, m := range []models.Model{main, light} {
		if m.Priced && !slices.ContainsFunc(s.Models, func(o ModelOption) bool { return o.ID == m.ID }) {
			s.Models = append(s.Models, ModelOption{ID: m.ID, PriceIn: m.PriceIn, PriceOut: m.PriceOut})
		}
	}
	// One word from the main model before anything is saved.
	if slices.Contains(llm.Providers, providerOf(main.ID)) {
		api, _, err := a.apiFor(ctx, providerOf(main.ID), nameOf(main.ID), main.PriceIn, main.PriceOut)
		if err != nil {
			bad(400, err.Error())
			return
		}
		start := time.Now()
		resp, err := api.Generate(context.WithoutCancel(ctx), llm.Request{Prompt: "Answer with the single word: ok", MaxCostUSD: 0.01})
		if err != nil {
			server.WriteJSON(w, 502, map[string]string{"error": err.Error(), "problem": models.Problem(err)})
			return
		}
		a.Budget.Record(ctx, budget.Cost{USD: resp.CostUSD, Source: "model test", Ref: main.ID})
		res.Text, res.CostUSD, res.MS = strings.TrimSpace(resp.Text), resp.CostUSD, time.Since(start).Milliseconds()
	}
	s.ExploreModel, s.CompileModel, s.JudgeModel = main.ID, main.ID, light.ID
	if s.JudgeBackend == "" {
		s.JudgeBackend = "local"
	}
	if err := a.SaveSettings(ctx, s, actor(ctx)); err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(ctx, "setup.model", actor(ctx), map[string]string{"kind": c.Kind, "explore": main.ID, "judge": light.ID})
	server.WriteJSON(w, 200, res)
}

func providerOf(id string) string { p, _, _ := strings.Cut(id, ":"); return p }
func nameOf(id string) string     { _, n, _ := strings.Cut(id, ":"); return n }

// A family names what to look for in a provider's model ids: every word in
// has, none in not. The first family with a match wins; within it, the
// highest version.
type family struct{ has, not []string }

var skipAlways = []string{"embed", "audio", "tts", "whisper", "transcribe", "image", "realtime", "moderation", "search", "vision", "guard", "dall-e", "codex", "computer"}

var mainFamilies = map[string][]family{
	"anthropic":  {{has: []string{"sonnet"}}, {has: []string{"opus"}}},
	"openai":     {{has: []string{"gpt-"}, not: []string{"mini", "nano", "oss", "chat-latest", "instruct"}}},
	"google":     {{has: []string{"gemini", "pro"}}, {has: []string{"gemini", "flash"}, not: []string{"lite"}}},
	"openrouter": {{has: []string{"anthropic/claude", "sonnet"}}, {has: []string{"openai/gpt-"}, not: []string{"mini", "nano", "oss"}}, {has: []string{"google/gemini", "pro"}}},
	"dashscope":  {{has: []string{"qwen", "max"}}, {has: []string{"qwen-plus"}}},
	"deepseek":   {{has: []string{"deepseek-chat"}}, {has: []string{"deepseek-v"}}},
	"groq":       {{has: []string{"70b"}}, {has: []string{"120b"}}},
	"mistral":    {{has: []string{"mistral-large"}}, {has: []string{"mistral-medium"}}},
	"xai":        {{has: []string{"grok"}, not: []string{"mini", "fast"}}},
}

var lightFamilies = map[string][]family{
	"anthropic":  {{has: []string{"haiku"}}},
	"openai":     {{has: []string{"gpt-", "mini"}}, {has: []string{"gpt-", "nano"}}},
	"google":     {{has: []string{"gemini", "flash"}, not: []string{"lite"}}, {has: []string{"gemini", "flash"}}},
	"openrouter": {{has: []string{"anthropic/claude", "haiku"}}, {has: []string{"google/gemini", "flash"}, not: []string{"lite"}}, {has: []string{"openai/gpt-", "mini"}}},
	"dashscope":  {{has: []string{"qwen", "flash"}}, {has: []string{"qwen-turbo"}}, {has: []string{"qwen-plus"}}},
	"deepseek":   {{has: []string{"deepseek-chat"}}},
	"groq":       {{has: []string{"8b"}}, {has: []string{"instant"}}, {has: []string{"70b"}}},
	"mistral":    {{has: []string{"mistral-small"}}, {has: []string{"ministral"}}},
	"xai":        {{has: []string{"grok", "mini"}}, {has: []string{"grok", "fast"}}},
}

var number = regexp.MustCompile(`\d+`)

// version orders ids by the numbers in them (claude-sonnet-5-5 over
// claude-sonnet-4-5-20250929), dates last, and puts previews and
// experiments below finished models.
func version(id string) []int {
	var v []int
	date := 0
	for _, n := range number.FindAllString(id, -1) {
		x, _ := strconv.Atoi(n)
		if len(n) == 8 || len(n) == 4 && x > 2000 {
			date = x
			continue
		}
		v = append(v, x)
	}
	stable := 1
	if strings.Contains(id, "preview") || strings.Contains(id, "exp") || strings.Contains(id, "beta") {
		stable = 0
	}
	return append([]int{stable}, append(v, date)...)
}

func newer(a, b []int) bool {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return len(a) > len(b)
}

func pick(fams []family, list []models.Model) (models.Model, bool) {
	for _, f := range fams {
		var got []models.Model
		for _, m := range list {
			id := strings.ToLower(m.ID)
			ok := true
			for _, w := range f.has {
				ok = ok && strings.Contains(id, w)
			}
			for _, w := range append(append([]string{}, f.not...), skipAlways...) {
				ok = ok && !strings.Contains(id, w)
			}
			if ok {
				got = append(got, m)
			}
		}
		if len(got) > 0 {
			sort.SliceStable(got, func(i, j int) bool {
				return newer(version(strings.ToLower(got[i].ID)), version(strings.ToLower(got[j].ID)))
			})
			return got[0], true
		}
	}
	return models.Model{}, false
}

// recommend chooses a provider's main model (explore and compile) and its
// light one (judgments), falling back to the priciest and the cheapest
// priced models when no family matches.
func recommend(provider string, list []models.Model) (main, light models.Model, ok bool) {
	main, okMain := pick(mainFamilies[provider], list)
	light, okLight := pick(lightFamilies[provider], list)
	if okMain && okLight {
		return main, light, true
	}
	var priced []models.Model
	for _, m := range list {
		id := strings.ToLower(m.ID)
		if m.Priced && !m.Free && !slices.ContainsFunc(skipAlways, func(w string) bool { return strings.Contains(id, w) }) {
			priced = append(priced, m)
		}
	}
	if len(priced) == 0 {
		return main, light, false
	}
	sort.SliceStable(priced, func(i, j int) bool { return priced[i].PriceOut > priced[j].PriceOut })
	if !okMain {
		main = priced[0]
		for _, m := range priced {
			if m.PriceOut <= 20 { // the priciest that is not a luxury
				main = m
				break
			}
		}
	}
	if !okLight {
		light = priced[len(priced)-1]
	}
	return main, light, true
}
