package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/denerFernandes/pimpo/internal/llm"
)

func TestImportOpenAPI(t *testing.T) {
	spec, _ := os.ReadFile(filepath.Join("..", "connector", "external", "testdata", "petstore.yaml"))
	fetches := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openapi.yaml":
			fetches++
			w.Write(spec)
		case "/api/v3/pet/findByStatus":
			if r.Header.Get("X-Api-Key") != "k" {
				w.WriteHeader(401)
				return
			}
			w.Write([]byte(`[{"name":"Rex"}]`))
		}
	}))
	defer api.Close()
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.Home = t.TempDir()
	src := map[string]any{"url": api.URL + "/openapi.yaml"}
	code, out := ta.do(t, "POST", "/api/connectors/openapi/preview", src)
	if code != 200 || len(out["operations"].([]any)) != 5 || len(out["keys"].([]any)) != 2 {
		t.Fatalf("preview %d %v", code, out)
	}
	add := map[string]any{"url": src["url"], "name": "pets", "operations": map[string]string{"find_pets_by_status": "read", "delete_pet": "irreversible"}, "keys": map[string]string{"API_KEY": "k"}}
	if code, out := ta.do(t, "POST", "/api/connectors/openapi/add", add); code != 200 || out["loaded"] != 1.0 {
		t.Fatalf("add %d %v", code, out)
	}
	if fetches != 1 {
		t.Fatalf("description fetched %d times", fetches)
	}
	res, err := ta.Router.Call(t.Context(), "pets.find_pets_by_status", "", map[string]any{"status": "sold"})
	if err != nil || fmt.Sprint(res) != "[map[name:Rex]]" {
		t.Fatalf("call %v %v", res, err)
	}
	if code, _ := ta.do(t, "POST", "/api/connectors/openapi/add", add); code != 400 {
		t.Fatalf("same name twice: %d", code)
	}
	if code, _ := ta.do(t, "POST", "/api/connectors/openapi/preview", map[string]any{"spec": "hello: world"}); code != 422 {
		t.Fatalf("not a spec: %d", code)
	}
}
