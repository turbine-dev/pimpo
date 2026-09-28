package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func cfgMap(m map[string]string) Config {
	return func(_ context.Context, f string) (string, error) { return m[f], nil }
}

func TestWebSearchWithBraveOrSearXNG(t *testing.T) {
	var gotKey, gotQuery string
	brave := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey, gotQuery = r.Header.Get("X-Subscription-Token"), r.URL.Query().Get("q")
		w.Write([]byte(`{"web":{"results":[{"title":"Previsão <b>Lisboa</b>","url":"https://tempo.pt/lisboa","description":"Sol, 24&deg;"},{"title":"b","url":"https://b"}]}}`))
	}))
	defer brave.Close()
	BaseURL["websearch"] = brave.URL
	defer delete(BaseURL, "websearch")
	ctx := context.Background()
	out, err := callSearch(ctx, cfgMap(map[string]string{"brave_key": "bk", "searxng_url": "https://ignored"}), "web.search", "", map[string]any{"query": "tempo lisboa", "count": float64(1)})
	if err != nil || gotKey != "bk" || gotQuery != "tempo lisboa" {
		t.Fatalf("%v %q %q", err, gotKey, gotQuery)
	}
	res := out.([]result)
	if len(res) != 1 || res[0].Title != "Previsão Lisboa" || res[0].Snippet != "Sol, 24°" {
		t.Fatalf("%+v", res)
	}

	searx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") != "json" {
			w.WriteHeader(403)
			return
		}
		w.Write([]byte(`{"results":[{"title":"Pimpo","url":"https://pimpo.app","content":"agente pessoal"}]}`))
	}))
	defer searx.Close()
	out, err = callSearch(ctx, cfgMap(map[string]string{"searxng_url": searx.URL + "/"}), "web.search", "", map[string]any{"query": "pimpo"})
	if err != nil || out.([]result)[0].URL != "https://pimpo.app" {
		t.Fatalf("%v %v", out, err)
	}
	if _, err := callSearch(ctx, cfgMap(nil), "web.search", "", map[string]any{"query": "x"}); err == nil || !strings.Contains(err.Error(), "Connections") {
		t.Fatalf("%v", err)
	}
	if _, err := callSearch(ctx, cfgMap(map[string]string{"searxng_url": "searx.local"}), "web.search", "", map[string]any{"query": "x"}); err == nil {
		t.Fatal("accepted an address without a scheme")
	}
}

func TestWebSearchWithPerplexity(t *testing.T) {
	var auth, query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		auth, query = r.Header.Get("Authorization"), fmt.Sprint(body["query"])
		if r.Method != "POST" || r.URL.Path != "/search" || body["max_results"] != float64(2) {
			w.WriteHeader(400)
			return
		}
		w.Write([]byte(`{"results":[{"title":"Selic hoje","url":"https://bcb.gov.br/selic","snippet":"A taxa está em 10,5%","date":"2026-09-28"}]}`))
	}))
	defer srv.Close()
	BaseURL["perplexity"] = srv.URL
	defer delete(BaseURL, "perplexity")
	out, err := callSearch(context.Background(), cfgMap(map[string]string{"perplexity_key": "pk", "searxng_url": "https://ignored"}), "web.search", "", map[string]any{"query": "selic", "count": float64(2)})
	if err != nil || auth != "Bearer pk" || query != "selic" {
		t.Fatalf("%v %q %q", err, auth, query)
	}
	if res := out.([]result); len(res) != 1 || res[0].URL != "https://bcb.gov.br/selic" || res[0].Snippet != "A taxa está em 10,5%" {
		t.Fatalf("%+v", res)
	}
}
