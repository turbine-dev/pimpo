package services

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/denerFernandes/zodim/internal/capability"
)

// Web search through Brave's official API or a SearXNG instance the owner
// trusts. DuckDuckGo has no official API for web results, and scraping it
// breaks its terms; SearXNG can include it among its engines.
func init() {
	register(Kind{
		ID: "websearch", Title: "Busca na web", Description: "Pesquisa na internet: notícias, preços, horários, qualquer coisa.",
		Help: "Use a API do Brave Search (chave grátis em api-dashboard.search.brave.com, 2.000 buscas por mês) ou o endereço de uma instância SearXNG sua ou de confiança, com o formato JSON ligado. Se preencher os dois, o Brave é usado.",
		Fields: []Field{
			{Name: "brave_key", Label: "Chave da API do Brave Search", Secret: true, Optional: true},
			{Name: "searxng_url", Label: "Endereço do SearXNG", Placeholder: "https://searx.exemplo.org", Optional: true},
		},
		Specs: []capability.Spec{{Name: "web.search", Risk: capability.Read, Signature: "web.search({query, count})", Returns: "[{title, url, snippet}] most relevant first; count up to 10",
			Schema: obj(`"query":{"type":"string"},"count":{"type":"integer","minimum":1,"maximum":10}`, "query")}},
		Call: callSearch,
		Probe: func(ctx context.Context, cfg Config) error {
			_, err := callSearch(ctx, cfg, "web.search", "", map[string]any{"query": "tempo hoje", "count": 1})
			return err
		},
	})
}

type result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func callSearch(ctx context.Context, cfg Config, _, _ string, args any) (any, error) {
	m, _ := args.(map[string]any)
	q, _ := m["query"].(string)
	if q = strings.TrimSpace(q); q == "" {
		return nil, errors.New("web.search needs a query")
	}
	count := 5
	if c, ok := m["count"].(float64); ok && c >= 1 {
		count = min(int(c), 10)
	}
	key, _ := cfg(ctx, "brave_key")
	searx, _ := cfg(ctx, "searxng_url")
	out := []result{}
	switch {
	case strings.TrimSpace(key) != "":
		var r struct {
			Web struct {
				Results []struct {
					Title       string `json:"title"`
					URL         string `json:"url"`
					Description string `json:"description"`
				} `json:"results"`
			} `json:"web"`
		}
		u := base("websearch", "https://api.search.brave.com") + "/res/v1/web/search?" + url.Values{"q": {q}, "count": {strconv.Itoa(count)}}.Encode()
		if err := doJSON(ctx, "GET", u, map[string]string{"X-Subscription-Token": strings.TrimSpace(key), "Accept": "application/json"}, nil, &r); err != nil {
			return nil, err
		}
		for _, x := range r.Web.Results {
			out = append(out, result{plain(x.Title), x.URL, plain(x.Description)})
		}
	case strings.TrimSpace(searx) != "":
		b, err := url.Parse(strings.TrimRight(strings.TrimSpace(searx), "/"))
		if err != nil || b.Host == "" || (b.Scheme != "https" && b.Scheme != "http") {
			return nil, errors.New("the SearXNG address must look like https://searx.example.org")
		}
		var r struct {
			Results []struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Content string `json:"content"`
			} `json:"results"`
		}
		if err := doJSON(ctx, "GET", b.String()+"/search?"+url.Values{"q": {q}, "format": {"json"}}.Encode(), nil, nil, &r); err != nil {
			return nil, errors.New("SearXNG did not answer with JSON; turn on the json format in its settings: " + err.Error())
		}
		for _, x := range r.Results {
			out = append(out, result{plain(x.Title), x.URL, plain(x.Content)})
		}
	default:
		return nil, errors.New("web search is not set up; open Connections")
	}
	if len(out) > count {
		out = out[:count]
	}
	return out, nil
}
