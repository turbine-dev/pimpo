package models

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPricesComeFromOpenRouterOrNotAtAll(t *testing.T) {
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[
			{"id":"anthropic/claude-sonnet-4.5","name":"Claude Sonnet 4.5","context_length":200000,"pricing":{"prompt":"0.000003","completion":"0.000015"}},
			{"id":"openai/gpt-5-mini","name":"GPT-5 mini","pricing":{"prompt":"0.00000025","completion":"0.000002"}},
			{"id":"meta/llama-free","name":"Free","pricing":{"prompt":"0","completion":"0"}}]}`))
	}))
	defer or.Close()
	anth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "k" {
			w.WriteHeader(401)
			return
		}
		w.Write([]byte(`{"data":[{"id":"claude-sonnet-4-5-20250929"},{"id":"claude-mystery-9"}]}`))
	}))
	defer anth.Close()
	c := &Client{OpenRouter: or.URL}
	ctx := context.Background()
	ms, err := c.List(ctx, Endpoint{Provider: "anthropic", Base: anth.URL, Key: "k"})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Model{}
	for _, m := range ms {
		byID[m.ID] = m
	}
	if m := byID["claude-sonnet-4-5-20250929"]; !m.Priced || m.PriceIn != 3 || m.PriceOut != 15 || m.Context != 200000 {
		t.Fatalf("dated id not matched to OpenRouter's price: %+v", m)
	}
	if m := byID["claude-mystery-9"]; m.Priced {
		t.Fatalf("a model OpenRouter does not list got a price: %+v", m)
	}
	if _, err := c.List(ctx, Endpoint{Provider: "anthropic", Base: anth.URL, Key: "bad"}); !errors.Is(err, ErrKey) {
		t.Fatalf("refused key: %v", err)
	}
	all, _ := c.List(ctx, Endpoint{Provider: "openrouter"})
	if len(all) != 3 || !all[1].Free {
		t.Fatalf("openrouter %+v", all)
	}
}

func TestLocalServersAreFound(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Write([]byte(`{"data":[{"id":"qwen3:8b"},{"id":"llama3.2:3b"}]}`))
		}
	}))
	defer ollama.Close()
	f := (&Client{}).Detect(context.Background(), ollama.URL, "http://127.0.0.1:1")
	if len(f.Ollama) != 2 || !f.Ollama[0].Free || len(f.LMStudio) != 0 {
		t.Fatalf("%+v", f)
	}
}

func TestProblemsInPlainKinds(t *testing.T) {
	for msg, want := range map[string]string{
		"anthropic refused the API key":           "key",
		"openai answered 429: rate limit":         "rate",
		"answered 402: insufficient credits":      "credits",
		"answered 404: model_not_found":           "model",
		"groq is unreachable: connection refused": "network",
		"something odd":                           "other",
	} {
		if got := Problem(errors.New(msg)); got != want {
			t.Errorf("%q: %s, want %s", msg, got, want)
		}
	}
	if !strings.Contains(norm("models/gemini-2.5-flash"), "gemini-2-5-flash") {
		t.Error(norm("models/gemini-2.5-flash"))
	}
}
