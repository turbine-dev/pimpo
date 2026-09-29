package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/denerFernandes/pimpo/internal/connector/external"
	"github.com/denerFernandes/pimpo/internal/server"
)

// Importing a REST API from its OpenAPI description: Pimpo reads it, the
// owner picks the operations and their risk, and a declarative connector
// is written. Nothing is installed until the owner chooses.

func (a *App) openapiRoutes() {
	a.Server.Handle("POST /api/connectors/openapi/preview", a.previewOpenAPI)
	a.Server.Handle("POST /api/connectors/openapi/add", a.addOpenAPI)
}

type openapiSource struct {
	URL  string `json:"url"`
	Spec string `json:"spec"`
	// Header, when the description declares no key, sends one anyway.
	Header string `json:"header"`
}

// The last descriptions read, so choosing operations right after the
// preview does not download a large one again.
var specCache = struct {
	sync.Mutex
	url string
	at  time.Time
	raw []byte
}{}

func (a *App) readOpenAPI(r *http.Request, v any) (*external.API, string, error) {
	// A pasted description can be far larger than other requests.
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<20)).Decode(v); err != nil {
		return nil, "", server.StatusError{Status: 400, Msg: "invalid request body: " + err.Error()}
	}
	var src openapiSource
	switch s := v.(type) {
	case *openapiSource:
		src = *s
	case *openapiAdd:
		src = s.openapiSource
	}
	raw, source := []byte(src.Spec), "pasted"
	if strings.TrimSpace(src.Spec) == "" {
		u := strings.TrimSpace(src.URL)
		if u == "" {
			return nil, "", server.StatusError{Status: 400, Msg: "give the address of the OpenAPI description or paste it"}
		}
		raw = nil
		specCache.Lock()
		if specCache.url == u && time.Since(specCache.at) < 10*time.Minute {
			raw = specCache.raw
		}
		specCache.Unlock()
		if raw == nil {
			var err error
			if raw, err = external.FetchSpec(r.Context(), u); err != nil {
				return nil, "", server.StatusError{Status: 422, Msg: err.Error()}
			}
			specCache.Lock()
			specCache.url, specCache.at, specCache.raw = u, time.Now(), raw
			specCache.Unlock()
		}
		source = u
	}
	specURL := ""
	if source != "pasted" {
		specURL = source
	}
	api, err := external.ParseOpenAPI(raw, specURL)
	if err != nil {
		return nil, "", server.StatusError{Status: 422, Msg: err.Error()}
	}
	if h := strings.TrimSpace(src.Header); h != "" {
		if err := api.AddHeader(h); err != nil {
			return nil, "", server.StatusError{Status: 400, Msg: err.Error()}
		}
	}
	return api, source, nil
}

func (a *App) previewOpenAPI(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var src openapiSource
	api, _, err := a.readOpenAPI(r, &src)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	server.WriteJSON(w, 200, api)
}

type openapiAdd struct {
	openapiSource
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Operations  map[string]string `json:"operations"` // id → risk
	Keys        map[string]string `json:"keys"`
}

func (a *App) addOpenAPI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ownerOnly(w, r) {
		return
	}
	if a.Home == "" {
		server.WriteError(w, server.StatusError{Status: 503, Msg: "no connectors folder"})
		return
	}
	var req openapiAdd
	api, source, err := a.readOpenAPI(r, &req)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	bad := func(msg string) { server.WriteError(w, server.StatusError{Status: 400, Msg: msg}) }
	name := req.Name
	if !connectorName.MatchString(name) {
		bad("the name must be 2 to 31 lowercase letters and digits, starting with a letter")
		return
	}
	if a.nameTaken(name) {
		bad(name + " is already in use; choose another name")
		return
	}
	m, err := api.Manifest(name, strings.TrimSpace(req.Description), source, req.Operations)
	if err != nil {
		bad(err.Error())
		return
	}
	dir := filepath.Join(a.Home, "connectors", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		server.WriteError(w, err)
		return
	}
	b, _ := json.MarshalIndent(m, "", " ")
	if err := os.WriteFile(filepath.Join(dir, "connector.json"), b, 0o600); err != nil {
		server.WriteError(w, err)
		return
	}
	if _, err := external.Load(dir); err != nil {
		os.RemoveAll(dir)
		bad(err.Error())
		return
	}
	for _, k := range m.Env {
		if v := strings.TrimSpace(req.Keys[k]); v != "" {
			a.Vault.Set(ctx, "connector."+name+"."+k, v)
		}
	}
	a.Events.Append(ctx, "connector.installed", actor(ctx), map[string]any{"name": name, "source": source, "openapi": true, "capabilities": len(m.Capabilities)})
	a.reloadConnectors(w, r)
}
