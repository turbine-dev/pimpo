package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const shop = `<!doctype html><html><head><title>Notebook Ultra 14 | Loja</title>
<meta name="description" content="O notebook mais leve.">
<script type="application/ld+json">{"@type":"Product","name":"Notebook Ultra 14","offers":{"@type":"Offer","price":"3099.00","priceCurrency":"BRL"}}</script>
<style>.x{color:red}</style><script>track()</script></head>
<body><nav><a href="/conta">Minha conta</a></nav>
<main><h1>Notebook Ultra 14</h1><p>Por <b>R$&nbsp;3.099</b> à vista</p>
<div hidden>preço antigo R$ 3.999</div><div style="display: none">oculto</div>
<ul><li>16 GB</li><li>512 GB</li></ul><a href="https://outra.example/x">Ver mais</a></main></body></html>`

func TestParsePage(t *testing.T) {
	base, _ := url.Parse("https://loja.example/p/1")
	p, err := ParsePage(strings.NewReader(shop), base)
	if err != nil {
		t.Fatal(err)
	}
	text := p["text"].(string)
	for _, want := range []string{"Notebook Ultra 14", "R$ 3.099 à vista", "16 GB\n512 GB"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
	for _, bad := range []string{"track()", "color:red", "3.999", "oculto"} {
		if strings.Contains(text, bad) {
			t.Errorf("text has %q", bad)
		}
	}
	if p["title"] != "Notebook Ultra 14 | Loja" || p["description"] != "O notebook mais leve." {
		t.Errorf("title %v description %v", p["title"], p["description"])
	}
	data := p["data"].([]any)
	if len(data) != 1 || data[0].(map[string]any)["offers"].(map[string]any)["price"] != "3099.00" {
		t.Errorf("data %v", data)
	}
	links := p["links"].([]map[string]string)
	if len(links) != 2 || links[0]["url"] != "https://loja.example/conta" || links[1]["text"] != "Ver mais" {
		t.Errorf("links %v", links)
	}
}

func TestReadStaysOnItsHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(shop))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	w := &Web{AllowPrivate: true}
	got, err := w.Call(context.Background(), "web.read", u.Hostname(), map[string]any{"url": srv.URL + "/p/1"})
	if err != nil || !strings.Contains(got.(map[string]any)["text"].(string), "3.099") {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := w.Call(context.Background(), "web.read", "other.example", map[string]any{"url": srv.URL}); err == nil {
		t.Fatal("read a host outside the scope")
	}
	if _, err := (&Web{}).Call(context.Background(), "web.read", u.Hostname(), map[string]any{"url": srv.URL}); err == nil {
		t.Fatal("reached a private address")
	}
}
