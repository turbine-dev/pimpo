package app

import (
	"context"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/chatlink"
	"github.com/turbine-dev/pimpo/internal/llm"
)

type fixedChooser map[string]float64

func (f fixedChooser) Choose(context.Context, string, any, map[string]string) (map[string]float64, error) {
	return f, nil
}

func TestRulesTier(t *testing.T) {
	for req, want := range map[string]string{
		"Que horas são em Tóquio?":                                    "simple",
		"Manda um e-mail para a Ana dizendo que atraso?":              "normal",
		"Compare os três orçamentos da reforma e diga qual vale mais": "hard",
		"Todo dia às 7h me manda a agenda":                            "hard",
		"Resuma meus e-mails de hoje":                                 "normal",
	} {
		if got := rulesTier(req); got != want {
			t.Errorf("%q: %s, want %s", req, got, want)
		}
	}
}

// Automatic choice sends clear cases to the light or strong model, keeps
// doubt on the usual one, and never overrides the owner's choice.
func TestRouteModel(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := t.Context()
	s := ta.Settings(ctx)
	s.Models = []ModelOption{{ID: "openai:gpt-5-mini", PriceIn: 0.25, PriceOut: 2}, {ID: "anthropic:claude-opus-5", PriceIn: 5, PriceOut: 25}, {ID: "anthropic:claude-sonnet-5", PriceIn: 2, PriceOut: 10}}
	s.ExploreModel = "anthropic:claude-sonnet-5"
	ta.do(t, "PUT", "/api/settings", s)
	defer func(old func(*App) (chooser, bool)) { tierChooser = old }(tierChooser)

	tierChooser = func(*App) (chooser, bool) { return fixedChooser{"simple": 0.8, "normal": 0.15, "hard": 0.05}, true }
	if r := ta.routeModel(ctx, "que horas são?", "", "", ""); r.Model != "openai:gpt-5-mini" || r.Tier != "simple" || r.By != "jev" {
		t.Fatalf("simple %+v", r)
	}
	tierChooser = func(*App) (chooser, bool) { return fixedChooser{"simple": 0.1, "normal": 0.2, "hard": 0.7}, true }
	if r := ta.routeModel(ctx, "planeje minha viagem", "", "", ""); r.Model != "anthropic:claude-opus-5" || r.Tier != "hard" {
		t.Fatalf("hard %+v", r)
	}
	tierChooser = func(*App) (chooser, bool) { return fixedChooser{"simple": 0.5, "normal": 0.1, "hard": 0.4}, true }
	if r := ta.routeModel(ctx, "hmm", "", "", ""); r.Model != "anthropic:claude-sonnet-5" || r.Tier != "normal" {
		t.Fatalf("doubt should stay on the usual model: %+v", r)
	}
	if r := ta.routeModel(ctx, "que horas são?", "", "openai:gpt-5-mini", ""); r.By != "fixed" {
		t.Fatalf("fixed %+v", r)
	}
	// Near the day's spending limit the strong model is not used.
	tierChooser = func(*App) (chooser, bool) { return fixedChooser{"hard": 0.9}, true }
	ta.do(t, "PUT", "/api/budget", map[string]float64{"daily_usd": 1})
	ta.Budget.Record(ctx, budgetCost(0.9, "exploration"))
	if r := ta.routeModel(ctx, "planeje", "", "", ""); r.Model != "anthropic:claude-sonnet-5" {
		t.Fatalf("spent the strong model near the limit: %+v", r)
	}
	s = ta.Settings(ctx)
	s.AutoOff = true
	ta.do(t, "PUT", "/api/settings", s)
	if r := ta.routeModel(ctx, "que horas são?", "", "", ""); r.By != "default" {
		t.Fatalf("auto off %+v", r)
	}
}

// The chat remembers its model and shows which one answered.
func TestChatModelChoice(t *testing.T) {
	var seen []string
	agent := llm.FakeAgent{Script: func(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
		seen = append(seen, r.Model)
		return llm.Response{Text: "ok"}, nil
	}}
	ta := newApp(t, agent, &llm.Fake{})
	defer func(old func(*App) (chooser, bool)) { tierChooser = old }(tierChooser)
	tierChooser = func(*App) (chooser, bool) { return nil, false }
	code, out := ta.do(t, "POST", "/api/chats", map[string]string{"text": "Resuma meus e-mails de hoje", "model": "opus"})
	if code != 201 {
		t.Fatalf("%d %v", code, out)
	}
	chat := out["chat"].(string)
	ta.Explore.Wait()
	ta.do(t, "POST", "/api/chats/"+chat+"/messages", map[string]string{"text": "e os de ontem?"})
	ta.Explore.Wait()
	_, got := ta.do(t, "GET", "/api/chats/"+chat, nil)
	if got["model"] != "opus" || len(seen) != 2 || seen[0] != "opus" || seen[1] != "opus" {
		t.Fatalf("model %v seen %v", got["model"], seen)
	}
	turn := got["turns"].([]any)[0].(map[string]any)["model"].(map[string]any)
	if turn["model"] != "opus" || turn["by"] != "fixed" {
		t.Fatalf("turn model %v", turn)
	}
	if code, _ := ta.do(t, "POST", "/api/chats", map[string]string{"text": "x", "model": "openai:nope"}); code != 400 {
		t.Fatal("accepted a model that isn't set up")
	}
}

func TestModelCommandOnAChannel(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	l := &fakeLink{}
	run := &linkRun{link: l, cancel: func() {}}
	ta.links = map[string]*linkRun{"signal": run}
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+5511", Text: "pimpo " + ta.Channel.PairingCode()})
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+5511", Text: "/modelo"})
	if !strings.Contains(l.last(), "Modelo desta conversa: auto") {
		t.Fatalf("%q", l.last())
	}
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+5511", Text: "/modelo turbo-9000"})
	if !strings.Contains(l.last(), "Não conheço") {
		t.Fatalf("%q", l.last())
	}
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+5511", Text: "/model auto"})
	if !strings.Contains(l.last(), "esta conversa usa auto") {
		t.Fatalf("%q", l.last())
	}
}

// The same weighing sets how hard the model thinks: little for a quick
// request, hard for a heavy one, the job's default otherwise; the owner's
// level wins, and near the spending limit it does not climb.
func TestRouteEffort(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := t.Context()
	s := ta.Settings(ctx)
	s.Efforts = map[string]string{"explore": "medium"}
	ta.do(t, "PUT", "/api/settings", s)
	defer func(old func(*App) (chooser, bool)) { tierChooser = old }(tierChooser)
	for probs, want := range map[string]string{"simple": "low", "hard": "high", "normal": "medium"} {
		tierChooser = func(*App) (chooser, bool) { return fixedChooser{probs: 0.9}, true }
		if r := ta.routeModel(ctx, "x", "", "", ""); r.Effort != want {
			t.Errorf("%s: effort %+v, want %s", probs, r, want)
		}
	}
	tierChooser = func(*App) (chooser, bool) { return fixedChooser{"simple": 0.9}, true }
	if r := ta.routeModel(ctx, "x", "", "opus", "max"); r.Effort != "max" || r.EffortBy != "fixed" || r.Model != "opus" {
		t.Errorf("owner's level: %+v", r)
	}
	if r := ta.routeModel(ctx, "x", "", "opus", ""); r.Effort != "low" || r.By != "fixed" {
		t.Errorf("fixed model, automatic level: %+v", r)
	}
	tierChooser = func(*App) (chooser, bool) { return fixedChooser{"hard": 0.9}, true }
	ta.do(t, "PUT", "/api/budget", map[string]float64{"daily_usd": 1})
	ta.Budget.Record(ctx, budgetCost(0.9, "exploration"))
	if r := ta.routeModel(ctx, "x", "", "", ""); r.Effort != "medium" {
		t.Errorf("climbed near the limit: %+v", r)
	}
	s = ta.Settings(ctx)
	s.Efforts = map[string]string{"explore": "extreme"}
	if code, _ := ta.do(t, "PUT", "/api/settings", s); code != 400 {
		t.Error("accepted an unknown level")
	}
}

// A chat's level reaches the model, and the answer says which it was.
func TestChatEffort(t *testing.T) {
	var seen []string
	agent := llm.FakeAgent{Script: func(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
		seen = append(seen, r.Effort)
		return llm.Response{Text: "ok"}, nil
	}}
	ta := newApp(t, agent, &llm.Fake{})
	defer func(old func(*App) (chooser, bool)) { tierChooser = old }(tierChooser)
	tierChooser = func(*App) (chooser, bool) { return nil, false }
	_, out := ta.do(t, "POST", "/api/chats", map[string]string{"text": "Resuma meus e-mails de hoje", "effort": "high"})
	chat := out["chat"].(string)
	ta.Explore.Wait()
	ta.do(t, "POST", "/api/chats/"+chat+"/messages", map[string]string{"text": "e os de ontem?"})
	ta.Explore.Wait()
	ta.do(t, "POST", "/api/chats/"+chat+"/messages", map[string]string{"text": "ok", "effort": "auto"})
	ta.Explore.Wait()
	_, got := ta.do(t, "GET", "/api/chats/"+chat, nil)
	if len(seen) != 3 || seen[0] != "high" || seen[1] != "high" || seen[2] == "high" || got["effort"] != "auto" {
		t.Fatalf("seen %v, chat effort %v", seen, got["effort"])
	}
	turn := got["turns"].([]any)[0].(map[string]any)["model"].(map[string]any)
	if turn["effort"] != "high" || turn["effort_by"] != "fixed" {
		t.Fatalf("turn %v", turn)
	}
	if code, _ := ta.do(t, "POST", "/api/chats", map[string]string{"text": "x", "effort": "turbo"}); code != 400 {
		t.Fatal("accepted an unknown level")
	}
}

func TestEffortCommandOnAChannel(t *testing.T) {
	var seen []string
	agent := llm.FakeAgent{Script: func(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
		seen = append(seen, r.Effort)
		return llm.Response{Text: "ok"}, nil
	}}
	ta := newApp(t, agent, &llm.Fake{})
	defer func(old func(*App) (chooser, bool)) { tierChooser = old }(tierChooser)
	tierChooser = func(*App) (chooser, bool) { return nil, false }
	ctx := context.Background()
	l := &fakeLink{}
	run := &linkRun{link: l, cancel: func() {}}
	ta.links = map[string]*linkRun{"signal": run}
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+5511", Text: "pimpo " + ta.Channel.PairingCode()})
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+5511", Text: "/pensar"})
	if !strings.Contains(l.last(), "Raciocínio desta conversa: auto") {
		t.Fatalf("%q", l.last())
	}
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+5511", Text: "/pensar máximo"})
	if !strings.Contains(l.last(), "nível máximo") {
		t.Fatalf("%q", l.last())
	}
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+5511", Text: "/think turbo"})
	if !strings.Contains(l.last(), "turbo") {
		t.Fatalf("%q", l.last())
	}
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+5511", Text: "Resuma meus e-mails de hoje"})
	ta.Explore.Wait()
	if len(seen) != 1 || seen[0] != "max" {
		t.Fatalf("seen %v", seen)
	}
}

// opencode models join the owner's list without a price, can be chosen,
// and stay out of the price-based light and strong models.
func TestOpencodeModels(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := t.Context()
	s := ta.Settings(ctx)
	s.Models = []ModelOption{{ID: "opencode:deepseek/deepseek-flash"}, {ID: "openai:gpt-5-mini", PriceIn: 0.25, PriceOut: 2}, {ID: "openai:gpt-5", PriceIn: 2, PriceOut: 10}}
	s.ExploreModel = "openai:gpt-5"
	if code, out := ta.do(t, "PUT", "/api/settings", s); code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	if !ta.usableModel(ctx, "opencode:deepseek/deepseek-flash") || ta.usableModel(ctx, "opencode:x/other") {
		t.Fatal("usable models")
	}
	if light, _ := ta.autoModels(ctx); light != "openai:gpt-5-mini" {
		t.Fatalf("light %q", light)
	}
	s.Models = append(s.Models, ModelOption{ID: "opencode:nomodel"})
	if code, _ := ta.do(t, "PUT", "/api/settings", s); code != 400 {
		t.Fatal("accepted an opencode model without provider/model")
	}
}
