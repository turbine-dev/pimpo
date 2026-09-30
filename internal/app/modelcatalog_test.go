package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/models"
)

// The catalog comes from the provider, keeps the owner's prices, marks
// models listed later as new and the owner's models no longer listed as
// retired, and a fresh look asks the provider again.
func TestLiveCatalogMarksNewAndRetired(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := t.Context()
	var round atomic.Int32
	anth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "sk-ant" {
			w.WriteHeader(401)
			return
		}
		if round.Load() == 0 {
			io.WriteString(w, `{"data":[{"id":"claude-old"},{"id":"claude-sonnet-5"}]}`)
			return
		}
		io.WriteString(w, `{"data":[{"id":"claude-sonnet-5"},{"id":"claude-brand-new"}]}`)
	}))
	defer anth.Close()
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[{"id":"anthropic/claude-sonnet-5","pricing":{"prompt":"0.000009","completion":"0.00009"}}]}`)
	}))
	defer or.Close()
	ta.Models = &models.Client{OpenRouter: or.URL}
	modelBase["anthropic"] = anth.URL
	defer delete(modelBase, "anthropic")
	ta.Vault.Set(ctx, "model.anthropic.key", "sk-ant")
	s := ta.Settings(ctx)
	s.Models = []ModelOption{{ID: "anthropic:claude-old", PriceIn: 1, PriceOut: 5}, {ID: "anthropic:claude-sonnet-5", PriceIn: 2, PriceOut: 10}}
	s.JudgeModel = "anthropic:claude-old"
	if code, out := ta.do(t, "PUT", "/api/settings", s); code != 200 {
		t.Fatalf("%d %v", code, out)
	}

	byID := func(out map[string]any) map[string]map[string]any {
		m := map[string]map[string]any{}
		for _, x := range out["list"].([]any) {
			m[x.(map[string]any)["id"].(string)] = x.(map[string]any)
		}
		return m
	}
	code, out := ta.do(t, "GET", "/api/models/catalog/anthropic", nil)
	list := byID(out)
	if code != 200 || len(list) != 2 || list["claude-old"]["new"] == true || list["claude-sonnet-5"]["price_in"] != 2.0 || list["claude-sonnet-5"]["mine"] != true {
		t.Fatalf("first look %d %v", code, out)
	}

	// The provider drops claude-old and adds a model: the day's cache
	// hides it until a fresh look.
	round.Store(1)
	if _, out := ta.do(t, "GET", "/api/models/catalog/anthropic", nil); byID(out)["claude-brand-new"] != nil {
		t.Fatal("the cached list was not used")
	}
	_, out = ta.do(t, "GET", "/api/models/catalog/anthropic?fresh=1", nil)
	list = byID(out)
	if m := list["claude-brand-new"]; m == nil || m["new"] != true || m["priced"] == true {
		t.Fatalf("new model %v", out)
	}
	if m := list["claude-old"]; m == nil || m["retired"] != true || m["price_in"] != 1.0 {
		t.Fatalf("retired model %v", out)
	}
	_, out = ta.do(t, "GET", "/api/models/retired", nil)
	if r, _ := out["retired"].([]any); len(r) != 1 || r[0] != "anthropic:claude-old" {
		t.Fatalf("retired %v", out)
	}
	evs, _ := ta.Events.List(ctx, event.Query{Types: []string{"model.retired"}})
	if len(evs) != 1 {
		t.Fatalf("retired events %v", evs)
	}
	// An unpriced model still cannot be chosen.
	s = ta.Settings(ctx)
	s.ExploreModel = "anthropic:claude-brand-new"
	if code, _ := ta.do(t, "PUT", "/api/settings", s); code != 400 {
		t.Fatalf("chose a model with no price: %d", code)
	}
	// The daily check runs again only after a day.
	ta.checkCatalogs(ctx)
	if ta.dueForCatalog(ctx, time.Now()) {
		t.Fatal("due again right after a check")
	}
}

// Anthropic API models compact long conversations unless turned off.
func TestCompactionSetting(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := t.Context()
	api, _, _ := ta.apiFor(ctx, "anthropic", "claude-opus-5-5", 4, 20)
	if api.CompactAt != 150000 {
		t.Fatalf("default %d", api.CompactAt)
	}
	if o, _, _ := ta.apiFor(ctx, "openai", "gpt-5", 1, 1); o.CompactAt != 0 {
		t.Fatal("compaction on for another provider")
	}
	s := ta.Settings(ctx)
	s.CompactAt = 1000
	if code, _ := ta.do(t, "PUT", "/api/settings", s); code != 400 {
		t.Fatalf("accepted a trigger below the API's minimum: %d", code)
	}
	s.CompactAt, s.CompactOff = 0, true
	if code, out := ta.do(t, "PUT", "/api/settings", s); code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	if api, _, _ := ta.apiFor(ctx, "anthropic", "claude-opus-5-5", 4, 20); api.CompactAt != 0 {
		t.Fatal("compaction still on")
	}
}
