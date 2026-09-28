package local

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// serve hands out the fixtures; slow makes each response wait.
func serve(t *testing.T, slow time.Duration) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := os.ReadFile(filepath.Join("testdata", filepath.Base(r.URL.Path)))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(b)))
		if slow > 0 {
			w.Write(b[:10])
			w.(http.Flusher).Flush()
			select {
			case <-time.After(slow):
			case <-r.Context().Done():
				return
			}
			w.Write(b[10:])
			return
		}
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// withCatalog replaces the engine and voices for a test.
func withCatalog(t *testing.T, srv string, files map[string]string) {
	oldE, oldV := engines, Voices
	t.Cleanup(func() { engines, Voices = oldE, oldV })
	item := func(id, kind, file string) Item {
		b, _ := os.ReadFile(filepath.Join("testdata", file))
		sha := sum(b)
		if files[id] != "" {
			sha = files[id]
		}
		return Item{ID: id, Kind: kind, Name: id, Size: int64(len(b)), URL: srv + "/" + file, SHA256: sha, Folder: "voice-x", Languages: []string{"pt-BR"}, Voice: &VoiceSpec{Type: "piper", Model: "model.onnx"}}
	}
	engines = map[string]Item{runtime.GOOS + "/" + runtime.GOARCH: item("sherpa-onnx", "engine", "voice.tar.bz2")}
	Voices = []Item{item("good", "voice", "voice.tar.bz2"), item("tampered", "voice", "voice.tar.bz2"), item("evil", "voice", "evil.tar.bz2"), item("evil-link", "voice", "evil-link.tar.bz2")}
}

func wait(t *testing.T, m *Manager, id string) Job {
	t.Helper()
	for range 200 {
		for _, j := range m.Jobs() {
			if j.ID == id && j.State != "downloading" && j.State != "verifying" && j.State != "unpacking" {
				return j
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("download never finished")
	return Job{}
}

func TestInstall(t *testing.T) {
	srv := serve(t, 0)
	withCatalog(t, srv.URL, map[string]string{"tampered": strings.Repeat("0", 64)})
	dir := t.TempDir()
	var changed atomic.Int32
	m := &Manager{Dir: filepath.Join(dir, "local"), Changed: func() { changed.Add(1) }}

	j, err := m.Install("good")
	if err != nil {
		t.Fatal(err)
	}
	if j = wait(t, m, j.ID); j.State != "done" || j.Done != j.Total || j.Total == 0 {
		t.Fatalf("job %+v", j)
	}
	good, _ := Find("good")
	eng, _ := Engine()
	if !m.Installed(good) || !m.Installed(eng) || changed.Load() != 1 {
		t.Fatalf("installed %v engine %v changed %d", m.Installed(good), m.Installed(eng), changed.Load())
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "local", "good", "tokens.txt")); string(b) != "a 1\nb 2\n" {
		t.Fatalf("tokens %q", b)
	}
	if v, ok := m.VoiceFor("pt-BR"); !ok || v.ID != "good" {
		t.Fatalf("voice for pt-BR %v", v)
	}
	if _, err := m.Install("good"); err == nil {
		t.Fatal("installed twice")
	}

	for id, want := range map[string]string{"tampered": "checksum", "evil": "outside", "evil-link": "outside"} {
		j, err := m.Install(id)
		if err != nil {
			t.Fatal(err)
		}
		if j = wait(t, m, j.ID); j.State != "failed" || !strings.Contains(j.Error, want) {
			t.Errorf("%s: %+v", id, j)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.txt")); err == nil {
		t.Fatal("an archive wrote outside its folder")
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "local", "*.part")); len(matches) != 0 {
		t.Fatalf("left %v", matches)
	}
	if err := m.Remove("good"); err != nil || m.Installed(good) {
		t.Fatal("remove")
	}
}

func TestCancel(t *testing.T) {
	srv := serve(t, 5*time.Second)
	withCatalog(t, srv.URL, nil)
	m := &Manager{Dir: t.TempDir()}
	j, _ := m.Install("good")
	time.Sleep(50 * time.Millisecond)
	m.Cancel(j.ID)
	if j = wait(t, m, j.ID); j.State != "cancelled" {
		t.Fatalf("job %+v", j)
	}
}

func TestPull(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "pull") {
			for _, l := range []string{`{"status":"pulling manifest"}`, `{"status":"pulling a","digest":"sha256:a","total":1000,"completed":400}`,
				`{"status":"pulling b","digest":"sha256:b","total":3000,"completed":3000}`, `{"status":"pulling a","digest":"sha256:a","total":1000,"completed":1000}`,
				`{"status":"verifying sha256 digest"}`, `{"status":"success"}`} {
				fmt.Fprintln(w, l)
			}
			return
		}
	}))
	defer ollama.Close()
	m := &Manager{Dir: t.TempDir(), OllamaURL: func() string { return ollama.URL }}
	j, err := m.Pull("qwen3:4b")
	if err != nil {
		t.Fatal(err)
	}
	if j = wait(t, m, j.ID); j.State != "done" || j.Total != 4000 || j.Done != 4000 {
		t.Fatalf("job %+v", j)
	}
	for _, bad := range []string{"", "a b", "qwen3:4b; rm -rf", "../x"} {
		if _, err := m.Pull(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

// A catalog entry must say where its file is and what it hashes to.
func TestCatalog(t *testing.T) {
	oldE, oldV, oldS := engines, Voices, Suggestions
	defer func() { engines, Voices, Suggestions = oldE, oldV, oldS }()
	if len(Voices) == 0 || Voices[0].Voice == nil || len(Suggestions) == 0 {
		t.Fatal("built-in catalog is empty")
	}
	good := `{"engines":{},"voices":[{"id":"v","kind":"voice","name":"V","languages":["pt-BR"],"size":10,"url":"https://x/v.tar.bz2","sha256":"` + strings.Repeat("a", 64) + `","folder":"v","voice":{"type":"piper","model":"v.onnx"}}]}`
	if err := Use([]byte(good)); err != nil || len(Voices) != 1 {
		t.Fatalf("%v %v", err, Voices)
	}
	for _, bad := range []string{
		strings.Replace(good, strings.Repeat("a", 64), "abc", 1),
		strings.Replace(good, "https://", "http://", 1),
		strings.Replace(good, `"id":"v"`, `"id":"../v"`, 1),
		strings.Replace(good, `"type":"piper"`, `"type":"shell"`, 1),
		`{"voices":[{"id":"v"}]}`,
	} {
		if err := Use([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	dir := t.TempDir()
	if err := UseFile(filepath.Join(dir, "missing.json")); err != nil {
		t.Fatal("a missing owner catalog is not an error")
	}
}

// An item of loose files downloads each, checks each, and becomes the
// transcriber; one bad file fails the whole item.
func TestLooseFiles(t *testing.T) {
	srv := serve(t, 0)
	withCatalog(t, srv.URL, nil)
	oldT := Transcribers
	defer func() { Transcribers = oldT }()
	file := func(name string, bad bool) File {
		b, _ := os.ReadFile(filepath.Join("testdata", name))
		s := sum(b)
		if bad {
			s = strings.Repeat("1", 64)
		}
		return File{Name: name, URL: srv.URL + "/" + name, SHA256: s, Size: int64(len(b))}
	}
	spec := &TranscriberSpec{Type: "whisper", Encoder: "enc.onnx", Decoder: "dec.onnx", Tokens: "tok.txt"}
	Transcribers = []Item{
		{ID: "whisper-t", Kind: "transcriber", Name: "T", Size: 30, Quality: 1, Transcriber: spec, Files: []File{file("enc.onnx", false), file("dec.onnx", false), file("tok.txt", false)}},
		{ID: "whisper-bad", Kind: "transcriber", Name: "Bad", Size: 30, Quality: 2, Transcriber: spec, Files: []File{file("enc.onnx", false), file("dec.onnx", true)}},
	}
	m := &Manager{Dir: t.TempDir()}
	j, _ := m.Install("whisper-t")
	if j = wait(t, m, j.ID); j.State != "done" {
		t.Fatalf("%+v", j)
	}
	if b, _ := os.ReadFile(filepath.Join(m.Dir, "whisper-t", "dec.onnx")); string(b) != "decoder-bytes" {
		t.Fatalf("dec %q", b)
	}
	j, _ = m.Install("whisper-bad")
	if j = wait(t, m, j.ID); j.State != "failed" || !strings.Contains(j.Error, "dec.onnx") {
		t.Fatalf("%+v", j)
	}
	if tr, ok := m.Transcriber(); !ok || tr.ID != "whisper-t" {
		t.Fatalf("transcriber %v", tr)
	}
	if _, err := os.Stat(filepath.Join(m.Dir, "whisper-bad")); err == nil {
		t.Fatal("kept a failed item")
	}
}
