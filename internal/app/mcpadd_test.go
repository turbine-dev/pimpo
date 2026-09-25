package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denerFernandes/zodim/internal/capability"
	"github.com/denerFernandes/zodim/internal/connector/external"
	"github.com/denerFernandes/zodim/internal/llm"
)

// weatherMCP is a remote MCP server answering plain JSON.
func weatherMCP(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "k-123" {
			w.WriteHeader(401)
			return
		}
		var req struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.ID == nil {
			w.WriteHeader(202)
			return
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{
				{"name": "get-forecast", "description": "Forecast for a city", "inputSchema": map[string]any{"properties": map[string]any{"city": map[string]any{}, "days": map[string]any{}}, "required": []string{"city"}}, "annotations": map[string]any{"readOnlyHint": true}},
				{"name": "delete-station"},
			}}
		case "tools/call":
			result = map[string]any{"content": []map[string]string{{"type": "text", "text": `{"temp": 23}`}}}
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
	}))
}

func TestAddMCPServerFromTheRegistry(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.Home = t.TempDir()
	mcp := weatherMCP(t)
	defer mcp.Close()
	// Zodim requires https for remote servers.
	tlsMCP := httptest.NewTLSServer(mcp.Config.Handler)
	defer tlsMCP.Close()
	external.HTTP = tlsMCP.Client()
	defer func() { external.HTTP = nil }()

	reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"servers":[{"server":{"name":"io.github.acme/weather-mcp","description":"Weather","version":"1.0.0","remotes":[{"type":"streamable-http","url":"`+tlsMCP.URL+`/mcp","headers":[{"name":"X-Api-Key","isSecret":true,"isRequired":true}]}]}}],"metadata":{}}`)
	}))
	defer reg.Close()
	old := registryBase
	registryBase = reg.URL
	defer func() { registryBase = old }()

	_, found := ta.do(t, "GET", "/api/connectors/registry?q=weather", nil)
	s := found["servers"].([]any)[0].(map[string]any)
	if s["name"] != "weather" || s["kind"] != "remote" {
		t.Fatalf("%v", s)
	}
	src := map[string]any{"name": "weather", "url": s["url"], "headers": map[string]string{"X-Api-Key": "k-123"}}
	code, probe := ta.do(t, "POST", "/api/connectors/probe", src)
	if code != 200 {
		t.Fatalf("%d %v", code, probe)
	}
	tools := probe["tools"].([]any)
	if len(tools) != 2 || tools[0].(map[string]any)["risk"] != "read" || tools[1].(map[string]any)["risk"] != "irreversible" || tools[0].(map[string]any)["capability"] != "weather.get_forecast" {
		t.Fatalf("%v", tools)
	}

	add := map[string]any{"name": "weather", "url": s["url"], "headers": map[string]string{"X-Api-Key": "k-123"}, "source": "io.github.acme/weather-mcp@1.0.0", "tools": map[string]string{"get-forecast": "read"}}
	if code, out := ta.do(t, "POST", "/api/connectors/add", map[string]any{"name": "gmail", "url": s["url"], "tools": map[string]string{"get-forecast": "read"}}); code != 400 {
		t.Fatalf("took a built-in name: %d %v", code, out)
	}
	if code, out := ta.do(t, "POST", "/api/connectors/add", add); code != 200 || out["loaded"] != float64(1) {
		t.Fatalf("%d %v", code, out)
	}
	raw, _ := os.ReadFile(filepath.Join(ta.Home, "connectors", "weather", "connector.json"))
	if strings.Contains(string(raw), "k-123") || strings.Contains(string(raw), "delete-station") {
		t.Fatalf("manifest holds a secret or an unchosen tool: %s", raw)
	}
	spec, ok := capability.Catalog["weather.get_forecast"]
	if !ok || spec.Risk != capability.Read || spec.Signature != "get_forecast({city, days?})" {
		t.Fatalf("%+v", spec)
	}
	res, err := ta.Router.Call(t.Context(), "weather.get_forecast", "", map[string]any{"city": "Lisboa"})
	if err != nil || res.(map[string]any)["temp"] != float64(23) {
		t.Fatalf("%v %v", res, err)
	}
	if _, err := ta.Router.Call(t.Context(), "weather.delete_station", "", nil); err == nil {
		t.Fatal("a tool the owner left out is reachable")
	}

	if code, _ := ta.do(t, "DELETE", "/api/connectors/weather", nil); code != 200 {
		t.Fatal(code)
	}
	if _, err := os.Stat(filepath.Join(ta.Home, "connectors", "weather")); !os.IsNotExist(err) {
		t.Fatal("folder left behind")
	}
	if v, _ := ta.Vault.Get(t.Context(), "connector.weather.X-Api-Key"); v != "" {
		t.Fatal("key left in the vault")
	}
	if _, ok := capability.Catalog["weather.get_forecast"]; ok || ta.Router.Has("weather.get_forecast") {
		t.Fatal("the capability outlived its connector")
	}
	if code, out := ta.do(t, "POST", "/api/connectors/add", add); code != 200 {
		t.Fatalf("could not add it again: %d %v", code, out)
	}
	ta.do(t, "DELETE", "/api/connectors/weather", nil)
}

func TestProbeNeedsAnAddress(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	for _, body := range []map[string]any{
		{"url": "http://example.com/mcp"},
		{},
		{"command": "npx", "args": []string{"-y", "pkg", "--dir", "{folder}"}},
	} {
		if code, out := ta.do(t, "POST", "/api/connectors/probe", body); code != 400 {
			t.Fatalf("%v: %d %v", body, code, out)
		}
	}
}
