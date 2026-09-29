package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/turbine-dev/pimpo/internal/llm"
)

func TestLocalJudgeUsesTheConfiguredOllama(t *testing.T) {
	var hits atomic.Int32
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(500)
	}))
	defer ollama.Close()
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	s := ta.Settings(ctx)
	s.JudgeBackend, s.OllamaURL, s.OllamaModel, s.LocalJudgeURL = "local", ollama.URL, "qwen3:4b", "http://127.0.0.1:1"
	if err := ta.SaveSettings(ctx, s, "test"); err != nil {
		t.Fatal(err)
	}
	ta.judge(ctx, "is this important?", map[string]string{"subject": "x"})
	if hits.Load() == 0 {
		t.Fatal("the judge did not call the configured Ollama")
	}
}

func TestPutSettingsKeepsWhatIsNotSent(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	s := ta.Settings(ctx)
	s.OllamaURL, s.Locale = "http://127.0.0.1:11999", "pt-BR"
	if err := ta.SaveSettings(ctx, s, "test"); err != nil {
		t.Fatal(err)
	}
	if code, out := ta.do(t, "PUT", "/api/settings", map[string]any{"locale": "en"}); code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	got := ta.Settings(ctx)
	if got.Locale != "en" || got.OllamaURL != "http://127.0.0.1:11999" || got.JudgeBackend != s.JudgeBackend {
		t.Fatalf("settings after a partial update: %+v", got)
	}
}

func TestHomeNetworkFollowsTheListenPort(t *testing.T) {
	for addr, want := range map[string][2]any{"127.0.0.1:8080": {8080, false}, "0.0.0.0:9000": {9000, true}, ":7000": {7000, true}, "": {7788, false}} {
		ta := newApp(t, weatherAgent, &llm.Fake{})
		ta.ListenAddr = addr
		ta.AttachRemote(t.TempDir(), nil)
		if ta.LAN.Port != want[0] || ta.LAN.Served != want[1] {
			t.Errorf("%q: port %d served %v", addr, ta.LAN.Port, ta.LAN.Served)
		}
	}
}
