package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/denerFernandes/vigia/internal/llm"
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
	ta.Home, ta.Version = t.TempDir(), "test"
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
