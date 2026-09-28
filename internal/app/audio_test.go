package app

import (
	"context"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/denerFernandes/pimpo/internal/llm"
)

// audio.send reads the text aloud, keeps the recording and lists it with
// a player; markdown and addresses are not read.
func TestAudioSend(t *testing.T) {
	if _, err := exec.LookPath("say"); err != nil {
		t.Skip("no macOS voices")
	}
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.Home = t.TempDir()
	out, err := audioCap{ta.App}.Call(context.Background(), "audio.send", "", map[string]any{"title": "Resumo do HN", "text": "**Bom dia!** Hoje no Hacker News https://news.ycombinator.com três histórias.", "language": "pt-BR"})
	if err != nil {
		t.Fatal(err)
	}
	if d := out.(map[string]any)["delivered"].([]string); len(d) != 1 || d[0] != "inbox" {
		t.Fatalf("delivered %v", d)
	}
	_, list := ta.do(t, "GET", "/api/media", nil)
	items := list["list"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["title"] != "Resumo do HN" {
		t.Fatalf("media %v", list)
	}
	id := items[0].(map[string]any)["id"].(string)
	req, _ := http.NewRequest("GET", ta.srv.URL+"/api/media/"+id, nil)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "audio/mp4" || len(b) < 2000 {
		t.Fatalf("%d %s %d bytes", resp.StatusCode, resp.Header.Get("Content-Type"), len(b))
	}
	if code, _ := ta.do(t, "GET", "/api/media/aud-..%2F..%2Fpimpo.db", nil); code == 200 {
		t.Fatal("served a path outside the recordings")
	}
	if s := spoken("**Oi** veja https://x.com/a e `code`"); strings.Contains(s, "http") || strings.Contains(s, "*") || strings.Contains(s, "`") {
		t.Fatalf("spoken %q", s)
	}
}
