package external

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeRemote is a streamable-HTTP MCP server that answers tool calls as
// event streams and requires a session and a bearer token.
type fakeRemote struct {
	mu      sync.Mutex
	tools   []map[string]any
	calls   []string
	deleted bool
}

func (f *fakeRemote) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer k1" {
		w.WriteHeader(401)
		return
	}
	if r.Method == http.MethodDelete {
		f.mu.Lock()
		f.deleted = true
		f.mu.Unlock()
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
	if req.Method != "initialize" && r.Header.Get("Mcp-Session-Id") != "s-1" {
		w.WriteHeader(400)
		return
	}
	var result any
	switch req.Method {
	case "initialize":
		w.Header().Set("Mcp-Session-Id", "s-1")
		result = map[string]any{"protocolVersion": protocolVersion}
	case "tools/list":
		f.mu.Lock()
		result = map[string]any{"tools": f.tools}
		f.mu.Unlock()
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		json.Unmarshal(req.Params, &p)
		f.mu.Lock()
		f.calls = append(f.calls, p.Name)
		f.mu.Unlock()
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": map[string]any{"structuredContent": map[string]any{"repo": p.Arguments["repo"], "stars": 42}}})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: message\ndata: %s\n\n", `{"jsonrpc":"2.0","method":"notifications/progress","params":{}}`)
		fmt.Fprintf(w, "event: message\ndata: %s\n\n", body)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
}

func TestProbeSuggestsRisksFromHints(t *testing.T) {
	f := &fakeRemote{tools: []map[string]any{
		{"name": "get-repo", "description": "Read a repo", "annotations": map[string]any{"readOnlyHint": true}},
		{"name": "star", "annotations": map[string]any{"destructiveHint": false}},
		{"name": "delete_repo"},
	}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	if _, err := Probe(context.Background(), Endpoint{Name: "gh", URL: srv.URL}); err == nil || !strings.Contains(err.Error(), "refused the key") {
		t.Fatalf("no key: %v", err)
	}
	tools, err := Probe(context.Background(), Endpoint{Name: "gh", URL: srv.URL, Headers: map[string]string{"Authorization": "Bearer k1"}})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, tl := range tools {
		got[tl.Name] = tl.SuggestedRisk()
	}
	if got["get-repo"] != "read" || got["star"] != "reversible" || got["delete_repo"] != "irreversible" {
		t.Fatalf("%v", got)
	}
}

func TestImportedRemoteConnector(t *testing.T) {
	f := &fakeRemote{tools: []map[string]any{{"name": "get-repo"}, {"name": "delete_repo"}}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	m := Manifest{Name: "gh", URL: srv.URL, Headers: []string{"Authorization"}, Imported: true, Capabilities: []Capability{
		{Name: "gh.get_repo", Tool: "get-repo", Risk: "read", Signature: "get_repo({repo})", Returns: "the repo"},
	}}
	secrets := func(_ context.Context, n string) (string, error) {
		if n == "Authorization" {
			return "Bearer k1", nil
		}
		return "", nil
	}
	c := &Connector{Manifest: m, Secrets: secrets}
	out, err := c.Call(context.Background(), "gh.get_repo", "", map[string]any{"repo": "pimpo"})
	if err != nil || out.(map[string]any)["stars"] != float64(42) {
		t.Fatalf("%v %v", out, err)
	}
	c.Close()
	if !f.deleted {
		t.Fatal("the session was not ended")
	}
	// A tool that vanished is a change the owner must review.
	f.tools = []map[string]any{{"name": "delete_repo"}, {"name": "exfiltrate"}}
	c = &Connector{Manifest: m, Secrets: secrets}
	if _, err := c.Call(context.Background(), "gh.get_repo", "", nil); err == nil || !strings.Contains(err.Error(), "no longer offers") {
		t.Fatalf("%v", err)
	}
	if strings.Join(f.calls, ",") != "get-repo" {
		t.Fatalf("calls %v", f.calls)
	}
}

func TestLoadAcceptsImportedWithoutContract(t *testing.T) {
	dir := t.TempDir()
	write := func(m Manifest) {
		b, _ := json.Marshal(m)
		writeFile(t, dir, "connector.json", string(b))
	}
	m := Manifest{Name: "gh", URL: "https://mcp.example.com/mcp", Capabilities: []Capability{{Name: "gh.get_repo", Tool: "get-repo", Risk: "read", Signature: "s", Returns: "r"}}}
	write(m)
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "contract") {
		t.Fatalf("local read capability without a contract: %v", err)
	}
	m.Imported = true
	write(m)
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
	m.URL = "http://mcp.example.com/mcp"
	write(m)
	if _, err := Load(dir); err == nil {
		t.Fatal("accepted a remote server over plain http")
	}
	m.URL, m.Command = "https://mcp.example.com/mcp", "npx"
	write(m)
	if _, err := Load(dir); err == nil {
		t.Fatal("accepted both a command and a url")
	}
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
