package external

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeAPI is a small REST service: items by query, one item by id, and a
// POST that records what it was sent.
func fakeAPI(t *testing.T) (*httptest.Server, *[]map[string]any) {
	var posted []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k3y" || r.URL.Query().Get("lang") != "pt" {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/items":
			q := r.URL.Query().Get("q")
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []map[string]any{
				{"name": q + " 1", "html_url": "https://x/1", "extra": "drop", "labels": []map[string]any{{"name": "a"}, {"name": "b"}}},
				{"name": q + " 2", "html_url": "https://x/2"},
				{"name": q + " 3", "html_url": "https://x/3"},
			}}})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/items/"):
			json.NewEncoder(w).Encode(map[string]any{"id": strings.TrimPrefix(r.URL.EscapedPath(), "/v1/items/"), "name": "one"})
		case r.Method == "POST" && r.URL.Path == "/v1/notes":
			var b map[string]any
			raw, _ := io.ReadAll(r.Body)
			json.Unmarshal(raw, &b)
			posted = append(posted, b)
			w.WriteHeader(201)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &posted
}

func httpManifest(t *testing.T, base string, edit func(m map[string]any)) string {
	dir := t.TempDir()
	m := map[string]any{
		"name": "notes", "description": "Notes service", "env": []string{"NOTES_KEY"},
		"http": map[string]any{"base": base, "headers": map[string]string{"Authorization": "Bearer {{env.NOTES_KEY}}"}, "query": map[string]string{"lang": "pt"}},
		"capabilities": []map[string]any{
			{"name": "notes.find", "risk": "read", "signature": "notes.find({query, max?})", "returns": "[{title, link}]",
				"request": map[string]any{"method": "GET", "path": "/v1/items", "query": map[string]string{"q": "{{query}}", "limit": "{{max}}"}},
				"result":  map[string]any{"path": "data.items", "fields": map[string]string{"title": "name", "link": "html_url", "labels": "labels.*.name"}, "max": 2}},
			{"name": "notes.get", "risk": "read", "signature": "notes.get({id})", "returns": "{id, name}",
				"request": map[string]any{"method": "GET", "path": "/v1/items/{{id}}"}},
			{"name": "notes.add", "risk": "reversible", "signature": "notes.add({text, tags?})", "returns": "{ok}",
				"request": map[string]any{"method": "POST", "path": "/v1/notes", "body": map[string]any{"text": "{{text}}", "tags": "{{tags}}", "source": "pimpo: {{text}}"}}},
		},
		"contract": []map[string]any{
			{"capability": "notes.find", "args": map[string]any{"query": "casa"}, "keys": []string{"title", "link"}},
			{"capability": "notes.get", "args": map[string]any{"id": "7"}, "keys": []string{"id", "name"}},
		},
	}
	if edit != nil {
		edit(m)
	}
	b, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(dir, "connector.json"), b, 0o600)
	return dir
}

func secretKey(_ context.Context, name string) (string, error) {
	if name == "NOTES_KEY" {
		return "k3y", nil
	}
	return "", nil
}

func TestHTTPConnector(t *testing.T) {
	srv, posted := fakeAPI(t)
	man, err := Load(httpManifest(t, srv.URL, nil))
	if err != nil {
		t.Fatal(err)
	}
	if problems := Check(context.Background(), man, secretKey); len(problems) > 0 {
		t.Fatalf("contract: %v", problems)
	}
	c := &Connector{Manifest: man, Secrets: secretKey}
	ctx := context.Background()

	out, err := c.Call(ctx, "notes.find", "", map[string]any{"query": "casa"})
	if err != nil {
		t.Fatal(err)
	}
	list := out.([]any)
	first := list[0].(map[string]any)
	if len(list) != 2 || first["title"] != "casa 1" || first["extra"] != nil || fmt.Sprint(first["labels"]) != "[a b]" {
		t.Fatalf("find shaped wrong: %v", out)
	}

	// Path values are escaped, so an id cannot walk to another endpoint.
	out, err = c.Call(ctx, "notes.get", "", map[string]any{"id": "../notes?x=1"})
	if err != nil {
		t.Fatal(err)
	}
	if id := out.(map[string]any)["id"]; id != "..%2Fnotes%3Fx=1" {
		t.Fatalf("id escaped as %v", id)
	}
	if _, err := c.Call(ctx, "notes.get", "", map[string]any{}); err == nil || !strings.Contains(err.Error(), "missing id") {
		t.Fatalf("missing arg: %v", err)
	}

	// Body keeps types; optional values left out drop their field.
	if _, err := c.Call(ctx, "notes.add", "", map[string]any{"text": "oi", "tags": []any{"a", "b"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Call(ctx, "notes.add", "", map[string]any{"text": "só"}); err != nil {
		t.Fatal(err)
	}
	p := *posted
	if len(p) != 2 || p[0]["source"] != "pimpo: oi" || len(p[0]["tags"].([]any)) != 2 {
		t.Fatalf("posted %v", p)
	}
	if _, has := p[1]["tags"]; has {
		t.Fatalf("absent tags sent: %v", p[1])
	}

	// A wrong key is reported as such.
	bad := &Connector{Manifest: man, Secrets: func(context.Context, string) (string, error) { return "nope", nil }}
	if _, err := bad.Call(ctx, "notes.get", "", map[string]any{"id": "1"}); err == nil || !strings.Contains(err.Error(), "refused the key") {
		t.Fatalf("bad key: %v", err)
	}
}

func TestHTTPConnectorRefusesUnsafeManifests(t *testing.T) {
	cases := map[string]func(m map[string]any){
		"plain http elsewhere": func(m map[string]any) { m["http"].(map[string]any)["base"] = "http://api.example.com" },
		"undeclared secret": func(m map[string]any) {
			m["http"].(map[string]any)["headers"] = map[string]string{"X": "{{env.OTHER}}"}
		},
		"secret in path": func(m map[string]any) {
			m["capabilities"].([]map[string]any)[1]["request"].(map[string]any)["path"] = "/v1/{{env.NOTES_KEY}}"
		},
		"other host in path": func(m map[string]any) {
			m["capabilities"].([]map[string]any)[1]["request"].(map[string]any)["path"] = "https://evil.example/x"
		},
		"POST marked read":      func(m map[string]any) { m["capabilities"].([]map[string]any)[2]["risk"] = "read" },
		"no request":            func(m map[string]any) { delete(m["capabilities"].([]map[string]any)[2], "request") },
		"both http and command": func(m map[string]any) { m["command"] = "/bin/sh" },
	}
	for name, edit := range cases {
		if name == "POST marked read" {
			edit = func(m map[string]any) {
				m["capabilities"].([]map[string]any)[2]["risk"] = "read"
				m["contract"] = append(m["contract"].([]map[string]any), map[string]any{"capability": "notes.add", "args": map[string]any{}, "keys": []string{"ok"}})
			}
		}
		if _, err := Load(httpManifest(t, "https://api.example.com", edit)); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
}

func TestHTTPConnectorStaysOnHost(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"leak":true}`)) }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+"/x", http.StatusFound)
	}))
	defer srv.Close()
	man, err := Load(httpManifest(t, srv.URL, nil))
	if err != nil {
		t.Fatal(err)
	}
	c := &Connector{Manifest: man, Secrets: secretKey}
	if _, err := c.Call(context.Background(), "notes.get", "", map[string]any{"id": "1"}); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("redirect followed: %v", err)
	}
}
