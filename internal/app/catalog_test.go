package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/denerFernandes/zodim/internal/connector/services"
	"github.com/denerFernandes/zodim/internal/host"
	"github.com/denerFernandes/zodim/internal/llm"
)

func TestCatalogConnectors(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer td-secret" {
			w.WriteHeader(401)
			return
		}
		w.Write([]byte(`[{"id":"1","content":"Pagar luz"}]`))
	}))
	defer srv.Close()
	services.BaseURL["todoist"] = srv.URL
	defer delete(services.BaseURL, "todoist")

	_, out := ta.do(t, "GET", "/api/catalog", nil)
	if len(out["connectors"].([]any)) != 9 {
		t.Fatalf("catalog %v", out)
	}
	if code, _ := ta.do(t, "PUT", "/api/catalog/homeassistant", map[string]string{"url": "http://ha.local:8123"}); code != 400 {
		t.Fatalf("saved without the token: %d", code)
	}
	if code, _ := ta.do(t, "PUT", "/api/catalog/todoist", map[string]string{"token": "td-secret"}); code != 200 {
		t.Fatal("save")
	}
	_, out = ta.do(t, "GET", "/api/catalog", nil)
	for _, k := range out["connectors"].([]any) {
		k := k.(map[string]any)
		if k["id"] == "todoist" && (k["configured"] != true || len(k["values"].(map[string]any)) != 0) {
			t.Fatalf("todoist view leaks or is wrong: %v", k)
		}
	}
	if _, out := ta.do(t, "POST", "/api/catalog/todoist/check", nil); out["ok"] != true {
		t.Fatalf("check %v", out)
	}
	h := &host.Host{Env: ta.Explore.Env, Source: "routine:t#1"}
	res, err := h.Call(ctx, "todoist.tasks", "", map[string]any{"filter": "today"})
	if err != nil || len(res.([]map[string]any)) != 1 {
		t.Fatalf("call %v %v", res, err)
	}
	ta.do(t, "DELETE", "/api/catalog/todoist", nil)
	if _, err := h.Call(ctx, "todoist.tasks", "", map[string]any{}); err == nil {
		t.Fatal("still connected after removal")
	}
	if _, out := ta.do(t, "POST", "/api/catalog/todoist/check", nil); out["ok"] != false {
		t.Fatalf("check after removal %v", out)
	}
}
