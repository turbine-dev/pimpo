package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/llm"
)

func upload(t *testing.T, ta *testApp, path string, file []byte, fields map[string]string) (int, map[string]any) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "x")
	fw.Write(file)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	mw.Close()
	req, _ := http.NewRequest("POST", ta.srv.URL+path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestBackupFromTheWebApp(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.Home = t.TempDir()
	ta.Vault.Set(t.Context(), "telegram.token", "1:secret")
	if code, _ := ta.do(t, "POST", "/api/backup/export", map[string]string{"passphrase": "short"}); code != 400 {
		t.Fatalf("short passphrase: %d", code)
	}
	b, _ := json.Marshal(map[string]string{"passphrase": "long enough pass"})
	req, _ := http.NewRequest("POST", ta.srv.URL+"/api/backup/export", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer tok")
	resp, _ := http.DefaultClient.Do(req)
	archive, _ := io.ReadAll(resp.Body)
	if resp.Header.Get("Content-Disposition") == "" || len(archive) < 200 || bytes.Contains(archive, []byte("1:secret")) {
		t.Fatalf("export %d bytes, %v", len(archive), resp.Header)
	}
	if code, _ := upload(t, ta, "/api/backup/import", archive, map[string]string{"passphrase": "wrong one!"}); code != 400 {
		t.Fatalf("wrong passphrase accepted: %d", code)
	}
	code, out := upload(t, ta, "/api/backup/import", archive, map[string]string{"passphrase": "long enough pass"})
	if code != 200 || out["restart"] != true {
		t.Fatalf("import %d %v", code, out)
	}
	if _, err := os.Stat(filepath.Join(ta.Home, "import-pending", "secrets.json")); err != nil {
		t.Fatal("import not staged")
	}
}

func TestInstallConnectorWithoutRestart(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not installed")
	}
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.Home = t.TempDir()
	// Stop the connector before its folder is removed (Windows refuses to
	// remove a folder a running program uses).
	t.Cleanup(func() {
		for _, c := range ta.external {
			c.Close()
		}
	})
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"connector.json", "server.py"} {
		src, _ := os.ReadFile(filepath.Join("..", "..", "examples", "connectors", "tides", name))
		f, _ := zw.Create("tides/" + name)
		f.Write(src)
	}
	zw.Close()
	code, out := upload(t, ta, "/api/connectors/install", buf.Bytes(), nil)
	if code != 200 || out["loaded"] != 1.0 {
		t.Fatalf("install %d %v", code, out)
	}
	res, err := ta.Router.Call(t.Context(), "tides.today", "", map[string]any{"port": "Santos"})
	if err != nil || len(res.([]any)) != 2 {
		t.Fatalf("call %v %v", res, err)
	}
	var bad bytes.Buffer
	zw = zip.NewWriter(&bad)
	f, _ := zw.Create("../../evil.sh")
	f.Write([]byte("x"))
	zw.Close()
	if code, _ := upload(t, ta, "/api/connectors/install", bad.Bytes(), nil); code != 400 {
		t.Fatalf("zip slip accepted: %d", code)
	}
}

func TestInstallJSONConnector(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"hits":[{"title":"Go 2","points":300,"junk":1}]}`))
	}))
	defer api.Close()
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.Home = t.TempDir()
	man := `{"name":"hnlocal","description":"test","http":{"base":"` + api.URL + `"},
	  "capabilities":[{"name":"hnlocal.stories","risk":"read","signature":"hnlocal.stories({query})","returns":"[{title, points}]",
	    "request":{"method":"GET","path":"/search","query":{"query":"{{query}}"}},"result":{"path":"hits","fields":{"title":"title","points":"points"}}}],
	  "contract":[{"capability":"hnlocal.stories","args":{"query":"go"},"keys":["title","points"]}]}`
	code, out := upload(t, ta, "/api/connectors/install", []byte(man), nil)
	if code != 200 || out["loaded"] != 1.0 {
		t.Fatalf("install %d %v", code, out)
	}
	res, err := ta.Router.Call(t.Context(), "hnlocal.stories", "", map[string]any{"query": "go"})
	if err != nil || fmt.Sprint(res) != "[map[points:300 title:Go 2]]" {
		t.Fatalf("call %v %v", res, err)
	}
	if code, _ := upload(t, ta, "/api/connectors/install", []byte(`{"name":"x"}`), nil); code != 422 {
		t.Fatalf("bad manifest: %d", code)
	}
	// Installing it again replaces it; a built-in family's name is refused.
	if code, _ := upload(t, ta, "/api/connectors/install", []byte(man), nil); code != 200 {
		t.Fatalf("reinstall: %d", code)
	}
	for _, name := range []string{"gmail", "notify", "web"} {
		clash := strings.ReplaceAll(man, "hnlocal", name)
		if code, out := upload(t, ta, "/api/connectors/install", []byte(clash), nil); code != 409 {
			t.Fatalf("a connector named %s was installed: %d %v", name, code, out)
		}
	}
}
