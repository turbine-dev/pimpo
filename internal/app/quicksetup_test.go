package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/models"
)

func ids(ss ...string) []models.Model {
	out := []models.Model{}
	for _, s := range ss {
		out = append(out, models.Model{ID: s, Priced: true, PriceIn: 1, PriceOut: 2})
	}
	return out
}

func TestRecommendPicksTheCurrentModels(t *testing.T) {
	for provider, tc := range map[string]struct {
		list        []models.Model
		main, light string
	}{
		"anthropic": {ids("claude-3-5-sonnet-20241022", "claude-sonnet-4-5-20250929", "claude-sonnet-5-5", "claude-opus-5-5", "claude-haiku-4-5-20251001", "claude-3-haiku-20240307"), "claude-sonnet-5-5", "claude-haiku-4-5-20251001"},
		"openai":    {ids("gpt-4o", "gpt-4.1", "gpt-5", "gpt-5-mini", "gpt-5-nano", "gpt-4o-mini-transcribe", "text-embedding-3-large", "gpt-5-codex", "gpt-realtime"), "gpt-5", "gpt-5-mini"},
		"google":    {ids("gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.5-flash-lite", "gemini-3-pro-preview", "text-embedding-004"), "gemini-2.5-pro", "gemini-2.5-flash"},
		"deepseek":  {ids("deepseek-chat", "deepseek-reasoner"), "deepseek-chat", "deepseek-chat"},
		"mistral":   {ids("mistral-large-latest", "mistral-small-latest", "codestral-latest"), "mistral-large-latest", "mistral-small-latest"},
	} {
		main, light, ok := recommend(provider, tc.list)
		if !ok || main.ID != tc.main || light.ID != tc.light {
			t.Errorf("%s: got %s / %s, want %s / %s", provider, main.ID, light.ID, tc.main, tc.light)
		}
	}
	// A provider Pimpo has no families for falls back to prices.
	list := []models.Model{{ID: "big", Priced: true, PriceOut: 15}, {ID: "luxury", Priced: true, PriceOut: 80}, {ID: "small", Priced: true, PriceOut: 0.4}}
	if main, light, ok := recommend("custom", list); !ok || main.ID != "big" || light.ID != "small" {
		t.Errorf("fallback: %s / %s", main.ID, light.ID)
	}
	if _, _, ok := recommend("custom", []models.Model{{ID: "x"}}); ok {
		t.Error("recommended an unpriced model")
	}
}

// A new owner with only an Anthropic key gets every job set up, the main
// model tested with one word, and nothing that needs Claude Code.
func TestQuickSetupWithOnlyAnAPIKey(t *testing.T) {
	var said int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "sk-ant-new" {
			w.WriteHeader(401)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			io.WriteString(w, `{"data":[{"id":"claude-sonnet-5-5"},{"id":"claude-haiku-4-5-20251001"},{"id":"claude-3-haiku-20240307"}]}`)
		default:
			said++
			io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":10,"output_tokens":1}}`)
		}
	}))
	defer api.Close()
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[{"id":"anthropic/claude-sonnet-5.5","pricing":{"prompt":"0.000003","completion":"0.000015"}},{"id":"anthropic/claude-haiku-4.5","pricing":{"prompt":"0.000001","completion":"0.000005"}}]}`)
	}))
	defer or.Close()
	modelBase["anthropic"] = api.URL
	defer delete(modelBase, "anthropic")
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.Models = &models.Client{OpenRouter: or.URL}
	ctx := t.Context()

	code, out := ta.do(t, "POST", "/api/setup/model", map[string]string{"kind": "provider", "provider": "anthropic", "key": "sk-ant-new"})
	if code != 200 || out["explore"] != "anthropic:claude-sonnet-5-5" || out["judge"] != "anthropic:claude-haiku-4-5-20251001" || out["text"] != "ok" {
		t.Fatalf("%d %v", code, out)
	}
	got := ta.Settings(ctx)
	if got.ExploreModel != "anthropic:claude-sonnet-5-5" || got.CompileModel != got.ExploreModel || got.JudgeModel != "anthropic:claude-haiku-4-5-20251001" || len(got.Models) != 2 || said != 1 {
		t.Fatalf("settings %+v, said %d", got, said)
	}
	if code, _ := ta.do(t, "POST", "/api/setup/model", map[string]string{"kind": "provider", "provider": "anthropic", "key": "wrong"}); code != 502 {
		t.Fatalf("a refused key was accepted: %d", code)
	}
	if code, _ := ta.do(t, "POST", "/api/setup/model", map[string]string{"kind": "ollama"}); code != 400 {
		t.Fatalf("ollama without a model: %d", code)
	}
	if code, out := ta.do(t, "POST", "/api/setup/model", map[string]string{"kind": "codex"}); code != 200 || ta.Settings(ctx).ExploreModel != "codex" {
		t.Fatalf("codex: %d %v", code, out)
	}
}
