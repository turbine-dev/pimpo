package app

import (
	"context"
	"testing"

	"github.com/denerFernandes/pimpo/internal/llm"
)

// A call paid by a subscription costs no money and stays out of the
// daily limit; its API equivalent is shown apart in Custo.
func TestSubscriptionIsNotSpending(t *testing.T) {
	defer func(old func(string) bool) { onSubscription = old }(onSubscription)
	onSubscription = func(model string) bool { return model == "sonnet" }
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	resp, err := ta.withFallback(ctx, "explore", "sonnet", func(string) (llm.Response, error) { return llm.Response{Text: "ok", CostUSD: 0.4}, nil })
	if err != nil || resp.CostUSD != 0 {
		t.Fatalf("%+v %v", resp, err)
	}
	paid, _ := ta.withFallback(ctx, "explore", "openai:gpt-5", func(string) (llm.Response, error) { return llm.Response{Text: "ok", CostUSD: 0.1}, nil })
	if paid.CostUSD != 0.1 {
		t.Fatalf("paid call lost its cost: %+v", paid)
	}
	_, c := ta.do(t, "GET", "/api/cost", nil)
	sub := c["subscription"].(map[string]any)
	if sub["today"].(float64) != 0.4 || sub["by_model"].(map[string]any)["sonnet"].(float64) != 0.4 {
		t.Fatalf("subscription %v", sub)
	}
	if bm := c["by_model"].(map[string]any); bm["sonnet"] != nil || bm["openai:gpt-5"].(float64) != 0.1 {
		t.Fatalf("by_model %v", bm)
	}
}

// Tests do not ask the real CLIs how they are signed in.
func init() { onSubscription = func(string) bool { return false } }
