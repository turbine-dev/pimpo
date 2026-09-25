package external

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const registryPage = `{"servers":[
 {"server":{"name":"io.github.acme/files-mcp","description":"Files","version":"1.3.1","packages":[{"registryType":"npm","identifier":"@acme/files-mcp","version":"1.3.1","transport":{"type":"stdio"},
   "runtimeArguments":[{"type":"positional","value":"-y"}],
   "packageArguments":[{"type":"named","name":"allowed-directories","isRequired":true,"description":"Folders it may touch"}],
   "environmentVariables":[{"name":"FILES_TOKEN","isSecret":true,"isRequired":true},{"name":"FILES_MODE","default":"readonly"}]}]}},
 {"server":{"name":"io.github.acme/weather","title":"Weather","description":"Forecasts","version":"0.2.0","packages":[{"registryType":"pypi","identifier":"acme-weather","version":"0.2.0","transport":{"type":"stdio"}}]}},
 {"server":{"name":"com.example/github-mcp-server","description":"GitHub","version":"2.0.0","remotes":[{"type":"sse","url":"https://old.example.com/sse"},{"type":"streamable-http","url":"https://mcp.example.com/mcp","headers":[{"name":"Authorization","value":"Bearer {token}","isSecret":true,"isRequired":true}]}]}},
 {"server":{"name":"io.docker/only","description":"Docker","version":"1.0.0","packages":[{"registryType":"oci","identifier":"docker.io/acme/x","version":"1","transport":{"type":"stdio"}}]}}
],"metadata":{"nextCursor":"io.docker/only:1.0.0","count":4}}`

func TestSearchMapsRegistryServers(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		io.WriteString(w, registryPage)
	}))
	defer srv.Close()
	list, next, err := Search(context.Background(), nil, srv.URL, "files", "")
	if err != nil || next != "io.docker/only:1.0.0" || len(list) != 4 {
		t.Fatalf("%v %q %v", list, next, err)
	}
	if !strings.Contains(query, "version=latest") || !strings.Contains(query, "search=files") {
		t.Fatalf("query %s", query)
	}
	npm := list[0]
	if npm.Kind != "npm" || npm.Command != "npx" || strings.Join(npm.Args, " ") != "-y @acme/files-mcp@1.3.1 --allowed-directories {allowed-directories}" {
		t.Fatalf("npm %+v", npm)
	}
	if len(npm.Inputs) != 3 || npm.Inputs[0].Kind != "arg" || !npm.Inputs[1].Secret || npm.Inputs[2].Default != "readonly" {
		t.Fatalf("inputs %+v", npm.Inputs)
	}
	if py := list[1]; py.Kind != "pypi" || py.Command != "uvx" || strings.Join(py.Args, " ") != "acme-weather==0.2.0" || py.Title != "Weather" {
		t.Fatalf("pypi %+v", py)
	}
	if rm := list[2]; rm.Kind != "remote" || rm.URL != "https://mcp.example.com/mcp" || rm.Inputs[0].Kind != "header" || rm.Inputs[0].Default != "Bearer {token}" {
		t.Fatalf("remote %+v", rm)
	}
	if list[3].Kind != "unsupported" {
		t.Fatalf("oci %+v", list[3])
	}
}

func TestSuggestName(t *testing.T) {
	for in, want := range map[string]string{
		"io.github.acme/files-mcp":      "files",
		"com.example/github-mcp-server": "github",
		"io.github.x/mcp-server-fetch":  "fetch",
		"io.github.x/2fa":               "x2fa",
	} {
		if got := SuggestName(in); got != want || !nameRe.MatchString(got) {
			t.Errorf("%s: %q", in, got)
		}
	}
}
