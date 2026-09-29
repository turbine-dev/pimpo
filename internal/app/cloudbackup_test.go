package app

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/llm"
)

type bucket struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (b *bucket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	key, ok := strings.CutPrefix(r.URL.Path, "/casa/")
	if !ok {
		w.WriteHeader(404)
		io.WriteString(w, `<Error><Code>NoSuchBucket</Code></Error>`)
		return
	}
	switch {
	case r.Method == "PUT":
		b.objects[key], _ = io.ReadAll(r.Body)
	case r.Method == "DELETE":
		delete(b.objects, key)
	case key == "":
		type item struct {
			Key  string
			Size int
		}
		var page struct {
			XMLName  xml.Name `xml:"ListBucketResult"`
			Contents []item
		}
		for k, v := range b.objects {
			page.Contents = append(page.Contents, item{k, len(v)})
		}
		xml.NewEncoder(w).Encode(page)
	default:
		w.Write(b.objects[key])
	}
}

func (b *bucket) names() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for k := range b.objects {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestCloudBackupsToS3(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.Home = t.TempDir()
	ctx := context.Background()
	b := &bucket{objects: map[string][]byte{
		"pimpo-20200101-000000.pimpo": []byte("old"),
		"pimpo-20200102-000000.pimpo": []byte("old"),
		"notes.txt":                   []byte("not ours"),
	}}
	srv := httptest.NewServer(b)
	defer srv.Close()
	ta.Vault.Set(ctx, "telegram.token", "123:secret")
	ta.Events.Put(ctx, "mail.user", "eu@exemplo.com")

	cfg := map[string]any{"kind": "s3", "endpoint": srv.URL, "bucket": "outro", "keep": 2, "access_key": "AK", "secret_key": "SK", "passphrase": "correct horse"}
	if code, out := ta.do(t, "PUT", "/api/backup/cloud", cfg); code != 400 || !strings.Contains(out["error"].(string), "does not exist") {
		t.Fatalf("wrong bucket saved: %d %v", code, out)
	}
	cfg["bucket"] = "casa"
	if code, out := ta.do(t, "PUT", "/api/backup/cloud", map[string]any{"kind": "s3", "endpoint": srv.URL, "bucket": "casa", "access_key": "AK", "secret_key": "SK"}); code != 400 || !strings.Contains(out["error"].(string), "passphrase") {
		t.Fatalf("saved without a passphrase: %d %v", code, out)
	}
	code, out := ta.do(t, "PUT", "/api/backup/cloud", cfg)
	if code != 200 || out["has_keys"] != true || out["has_passphrase"] != true {
		t.Fatalf("%d %v", code, out)
	}
	_, view := ta.do(t, "GET", "/api/backup/cloud", nil)
	if raw, _ := json.Marshal(view); bytes.Contains(raw, []byte(`"SK"`)) || bytes.Contains(raw, []byte("correct horse")) {
		s := string(raw)
		t.Fatalf("a secret came back: %s", s)
	}

	if code, run := ta.do(t, "POST", "/api/backup/cloud/run", nil); code != 200 || run["ok"] != true {
		t.Fatalf("%d %v", code, run)
	}
	names := b.names()
	if len(names) != 3 || names[0] != "notes.txt" || names[1] != "pimpo-20200102-000000.pimpo" {
		t.Fatalf("keep 2 and never touch other files: %v", names)
	}
	sealed := b.objects[names[2]]
	if bytes.Contains(sealed, []byte("eu@exemplo.com")) || bytes.Contains(sealed, []byte("123:secret")) {
		t.Fatal("the uploaded backup is readable")
	}

	_, files := ta.do(t, "GET", "/api/backup/cloud/files", nil)
	if list := files["list"].([]any); len(list) != 2 || list[0].(map[string]any)["name"] != names[2] {
		t.Fatalf("%v", files)
	}
	if code, out := ta.do(t, "POST", "/api/backup/cloud/restore", map[string]string{"name": names[2], "passphrase": "wrong horse"}); code != 400 {
		t.Fatalf("wrong passphrase restored: %d %v", code, out)
	}
	if code, out := ta.do(t, "POST", "/api/backup/cloud/restore", map[string]string{"name": names[2]}); code != 200 || out["restart"] != true {
		t.Fatalf("%d %v", code, out)
	}
	if _, err := os.Stat(filepath.Join(ta.Home, "import-pending", "pimpo.db")); err != nil {
		t.Fatal("restore was not staged")
	}
	if code, _ := ta.do(t, "POST", "/api/backup/cloud/restore", map[string]string{"name": "../notes.txt"}); code != 400 {
		t.Fatal("restored a file that is not a backup")
	}

	now := time.Now()
	if ta.cloudDue(ctx, now) || !ta.cloudDue(ctx, now.Add(25*time.Hour)) {
		t.Fatal("daily schedule")
	}
	srv.Close()
	if run := ta.backupToCloud(ctx); run.OK {
		t.Fatal("uploaded to a closed server")
	}
	if ta.cloudDue(ctx, time.Now().Add(30*time.Minute)) || !ta.cloudDue(ctx, time.Now().Add(61*time.Minute+24*time.Hour)) {
		t.Fatal("retry after a failure")
	}
	ta.do(t, "DELETE", "/api/backup/cloud", nil)
	if ta.cloudDue(ctx, now.Add(48*time.Hour)) {
		t.Fatal("ran while off")
	}
}

func TestDriveNeedsItsPermission(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	code, out := ta.do(t, "PUT", "/api/backup/cloud", map[string]any{"kind": "drive", "passphrase": "correct horse"})
	if code != 400 || !strings.Contains(out["error"].(string), "connect Google") {
		t.Fatalf("%d %v", code, out)
	}
	ta.Vault.Set(ctx, "google.refresh", "r1")
	ta.Vault.Set(ctx, "google.scopes", "https://mail.google.com/")
	code, out = ta.do(t, "PUT", "/api/backup/cloud", map[string]any{"kind": "drive", "passphrase": "correct horse"})
	if code != 400 || !strings.Contains(out["error"].(string), "allow Drive") {
		t.Fatalf("%d %v", code, out)
	}
}
