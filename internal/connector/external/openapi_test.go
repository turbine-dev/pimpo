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

func TestOpenAPIImport(t *testing.T) {
	spec, _ := os.ReadFile("testdata/petstore.yaml")
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/openapi.yaml":
			w.Write(spec)
			return
		case r.Header.Get("X-Api-Key") != "s3cret":
			w.WriteHeader(401)
		case r.URL.Path == "/api/v3/pet/findByStatus":
			fmt.Fprintf(w, `[{"id":1,"name":"Rex","status":%q},{"id":2,"name":"Mia","status":"x"}]`, r.URL.Query().Get("status"))
		case r.URL.Path == "/api/v3/pet/7":
			w.Write([]byte(`{"id":7,"name":"Bob"}`))
		case r.URL.Path == "/api/v3/pet" && r.Method == "POST", r.URL.Path == "/api/v3/pet/search":
			raw, _ := io.ReadAll(r.Body)
			json.Unmarshal(raw, &gotBody)
			w.Write(raw)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	raw, err := FetchSpec(ctx, srv.URL+"/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	api, err := ParseOpenAPI(raw, srv.URL+"/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if api.Base != srv.URL+"/api/v3" || api.Title != "Petstore" || api.Description != "A sample pet store." {
		t.Fatalf("api %q %q %q", api.Base, api.Title, api.Description)
	}
	if len(api.Keys) != 2 || api.Keys[0].Name != "API_KEY" || api.Keys[1].Name != "PETSTORE_AUTH_KEY" {
		t.Fatalf("keys %+v", api.Keys)
	}
	ops := map[string]Operation{}
	for _, o := range api.Operations {
		ops[o.ID] = o
	}
	if len(ops) != 5 || ops["find_pets_by_status"].Risk != "read" || ops["delete_pet"].Risk != "irreversible" || ops["add_pet"].Risk != "irreversible" {
		t.Fatalf("ops %+v", api.Operations)
	}
	if len(api.Unsupported) != 1 || api.Unsupported[0].ID != "upload_file" {
		t.Fatalf("unsupported %+v", api.Unsupported)
	}
	if s := ops["find_pets_by_status"].signature; s != "({page_size?, status})" {
		t.Fatalf("signature %s", s)
	}
	if r := ops["get_pet_by_id"].returns; !strings.HasPrefix(r, "{id, name, status, tags} Find pet by ID") {
		t.Fatalf("returns %s", r)
	}

	chosen := map[string]string{}
	for id, o := range ops {
		chosen[id] = o.Risk
	}
	chosen["search_pets"] = "read" // a search sent as POST
	m, err := api.Manifest("pets", "", srv.URL+"/openapi.yaml", chosen)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	b, _ := json.MarshalIndent(m, "", " ")
	os.WriteFile(filepath.Join(dir, "connector.json"), b, 0o600)
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	if len(loaded.Contract) != 1 || loaded.Contract[0].Capability != "pets.find_pets_by_status" {
		t.Fatalf("contract %+v", loaded.Contract)
	}
	secrets := func(_ context.Context, n string) (string, error) {
		if n == "API_KEY" {
			return "s3cret", nil
		}
		return "", nil
	}
	if problems := Check(ctx, loaded, secrets); len(problems) > 0 {
		t.Fatalf("contract: %v", problems)
	}
	c := &Connector{Manifest: loaded, Secrets: secrets}
	out, err := c.Call(ctx, "pets.find_pets_by_status", "", map[string]any{"status": "sold"})
	if err != nil || fmt.Sprint(out) != "[map[id:1 name:Rex status:sold] map[id:2 name:Mia status:x]]" {
		t.Fatalf("find %v %v", out, err)
	}
	if out, err := c.Call(ctx, "pets.get_pet_by_id", "", map[string]any{"petId": 7}); err != nil || out.(map[string]any)["name"] != "Bob" {
		t.Fatalf("get %v %v", out, err)
	}
	if _, err := c.Call(ctx, "pets.add_pet", "", map[string]any{"name": "Tom", "tags": []any{map[string]any{"name": "a"}}}); err != nil {
		t.Fatal(err)
	}
	if gotBody["name"] != "Tom" || gotBody["id"] != nil || len(gotBody["tags"].([]any)) != 1 {
		t.Fatalf("body %v", gotBody)
	}
	for _, cp := range loaded.Capabilities {
		if cp.Request.Safe != (cp.Name == "pets.search_pets") {
			t.Fatalf("%s safe=%v", cp.Name, cp.Request.Safe)
		}
	}
	if _, err := api.Manifest("pets", "", "", map[string]string{"nope": "read"}); err == nil {
		t.Fatal("unknown operation accepted")
	}
}

func TestSwagger2Import(t *testing.T) {
	spec := `{"swagger":"2.0","info":{"title":"Old"},"host":"api.old.example","basePath":"/v1","schemes":["http","https"],
	 "securityDefinitions":{"token":{"type":"apiKey","in":"query","name":"access_token"}},
	 "paths":{"/notes":{"get":{"parameters":[{"name":"limit","in":"query","type":"integer"}],"responses":{"200":{"schema":{"type":"array","items":{"type":"object","properties":{"text":{"type":"string"}}}}}}},
	   "post":{"operationId":"createNote","consumes":["application/x-www-form-urlencoded"],"parameters":[{"name":"text","in":"formData","type":"string","required":true}],"responses":{"201":{"description":"ok"}}}}}}`
	api, err := ParseOpenAPI([]byte(spec), "")
	if err != nil {
		t.Fatal(err)
	}
	if api.Base != "https://api.old.example/v1" || len(api.Operations) != 2 || api.Operations[0].ID != "get_notes" || api.query["access_token"] != "{{env.TOKEN}}" {
		t.Fatalf("%+v %v", api, api.query)
	}
	m, err := api.Manifest("old", "", "", map[string]string{"get_notes": "read", "create_note": "reversible"})
	if err != nil {
		t.Fatal(err)
	}
	if r := m.Capabilities[1].Request; !r.Form || fmt.Sprint(r.Body) != "map[text:{{text}}]" {
		t.Fatalf("form %+v", r)
	}
	if m.Capabilities[0].Result == nil || m.Capabilities[0].Result.Max != 50 || m.Capabilities[0].Returns != "[{text}]" {
		t.Fatalf("list %+v", m.Capabilities[0])
	}
}

func TestOpenAPIRejects(t *testing.T) {
	for _, bad := range []string{`{"hello":1}`, `openapi: 3.0.0
info: {title: x}
paths: {}`, "::"} {
		if _, err := ParseOpenAPI([]byte(bad), "https://x.example/s.json"); err == nil {
			t.Errorf("parsed %q", bad)
		}
	}
	if _, err := FetchSpec(context.Background(), "http://example.com/spec.json"); err == nil {
		t.Error("plain http fetched")
	}
}
