//go:build live

package web

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"testing"
)

// PIMPO_PAGE=https://… go test -tags live -run TestLiveRead ./internal/connector/web
func TestLiveRead(t *testing.T) {
	page := os.Getenv("PIMPO_PAGE")
	if page == "" {
		page = "https://example.com"
	}
	u, _ := url.Parse(page)
	got, err := (&Web{}).Call(context.Background(), "web.read", u.Hostname(), map[string]any{"url": page})
	if err != nil {
		t.Fatal(err)
	}
	p := got.(map[string]any)
	text := p["text"].(string)
	if len(text) > 600 {
		text = text[:600]
	}
	data, _ := json.Marshal(p["data"])
	if len(data) > 600 {
		data = data[:600]
	}
	t.Logf("title: %v\ntext: %s\ndata: %s\nlinks: %d", p["title"], text, data, len(p["links"].([]map[string]string)))
}
