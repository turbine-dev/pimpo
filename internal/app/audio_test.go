package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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

// With OpenAI chosen, audio.send reads through it with the owner's OpenAI
// key, counts the exact cost, and stops at the daily limit.
func TestCloudVoice(t *testing.T) {
	if _, err := exec.LookPath("afconvert"); err != nil {
		t.Skip("no afconvert")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			w.WriteHeader(401)
			return
		}
		w.Write(make([]byte, 48000))
	}))
	defer srv.Close()
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.Home, ta.VoiceAPI = t.TempDir(), map[string]string{"openai": srv.URL}
	ctx := context.Background()
	s := ta.Settings(ctx)
	s.Voice, s.VoiceModel, s.VoiceName = "openai", "tts-1-hd", "nova"
	if code, out := ta.do(t, "PUT", "/api/settings", s); code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	if _, err := (audioCap{ta.App}).Call(ctx, "audio.send", "", map[string]any{"title": "x", "text": "Bom dia", "language": "pt-BR"}); err == nil || !strings.Contains(err.Error(), "OpenAI key") {
		t.Fatalf("without a key: %v", err)
	}
	ta.Vault.Set(ctx, "model.openai.key", "sk-test")
	text := strings.Repeat("Bom dia. ", 100) // 900 characters
	out, err := (audioCap{ta.App}).Call(ctx, "audio.send", "", map[string]any{"title": "x", "text": text, "language": "pt-BR"})
	if err != nil || out.(map[string]any)["voice"] != "OpenAI nova" {
		t.Fatalf("%v %v", out, err)
	}
	if spent, _ := ta.Budget.Today(ctx); spent < 0.0269 || spent > 0.0271 {
		t.Fatalf("spent %v, want 900 chars at $30/M", spent)
	}
	ta.do(t, "PUT", "/api/budget", map[string]float64{"daily_usd": 0.03})
	if _, err := (audioCap{ta.App}).Call(ctx, "audio.send", "", map[string]any{"title": "x", "text": text, "language": "pt-BR"}); err == nil {
		t.Fatal("read past the daily limit")
	}
	s.Voice, s.VoiceModel = "openai", "gpt-9"
	if code, _ := ta.do(t, "PUT", "/api/settings", s); code != 400 {
		t.Fatal("accepted an unknown voice model")
	}
}

// The chat's readings are kept: the same text with the same voice comes
// back from the cache, another voice is read anew.
func TestSpeakCache(t *testing.T) {
	if _, err := exec.LookPath("say"); err != nil {
		t.Skip("no macOS voices")
	}
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.Home = t.TempDir()
	speak := func() (int, string) {
		req, _ := http.NewRequest("POST", ta.srv.URL+"/api/speak", strings.NewReader(`{"text":"Bom dia, tudo certo?","language":"pt-BR"}`))
		req.Header.Set("Authorization", "Bearer tok")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode, resp.Header.Get("X-Pimpo-Cache")
	}
	if code, cache := speak(); code != 200 || cache != "" {
		t.Fatalf("first %d %q", code, cache)
	}
	if code, cache := speak(); code != 200 || cache != "hit" {
		t.Fatalf("second %d %q", code, cache)
	}
	s := ta.Settings(context.Background())
	s.ChatVoice = "system"
	ta.do(t, "PUT", "/api/settings", s)
	if _, cache := speak(); cache == "hit" {
		t.Fatal("another voice came from the cache")
	}
}
