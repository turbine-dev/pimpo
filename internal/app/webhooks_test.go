package app

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/denerFernandes/pimpo/internal/event"
	"github.com/denerFernandes/pimpo/internal/llm"
	"github.com/denerFernandes/pimpo/internal/routine"
	"github.com/denerFernandes/pimpo/internal/runtime"
	"github.com/denerFernandes/pimpo/internal/store"
)

func TestWebhook(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	r := routine.Routine{Name: "Pedido novo", Manifest: runtime.Manifest{Schedule: "", Capabilities: []string{"notify.send"}},
		Code: `async function run() { const w = event.webhook; await notify.send({text: "Pedido de " + w.body.cliente + " via " + w.method + " " + (w.query.loja || "")}) }`}
	if _, err := ta.Store.SaveRoutine(ctx, "pedido", r, "test", "human:owner"); err != nil {
		t.Fatal(err)
	}
	_, out := ta.do(t, "POST", "/api/routines/pedido/webhook/on", nil)
	url := out["urls"].(map[string]any)["local"].(string)
	path := url[strings.Index(url, "/hook/"):]
	call := func(p, body string) int {
		resp, err := http.Post(ta.srv.URL+p, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := call(path+"?loja=centro", `{"cliente":"Ana"}`); code != 202 {
		t.Fatalf("call %d", code)
	}
	var sent string
	for range 100 {
		evs, _ := ta.Events.List(ctx, event.Query{Types: []string{"notice.sent"}})
		for _, e := range evs {
			var n struct{ Text string }
			e.Decode(&n)
			if strings.Contains(n.Text, "Pedido") {
				sent = n.Text
			}
		}
		if sent != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if sent != "Pedido de Ana via POST centro" {
		t.Fatalf("sent %q", sent)
	}
	if code := call("/hook/pedido/"+strings.Repeat("0", 48), `{}`); code != 404 {
		t.Fatalf("a wrong token got %d", code)
	}
	ta.do(t, "POST", "/api/routines/pedido/webhook/rotate", nil)
	if code := call(path, `{}`); code != 404 {
		t.Fatalf("the old address still works: %d", code)
	}
	_, out = ta.do(t, "GET", "/api/routines/pedido/webhook", nil)
	url = out["urls"].(map[string]any)["local"].(string)
	path = url[strings.Index(url, "/hook/"):]
	ta.Store.SetRoutineState(ctx, "pedido", store.RoutinePaused)
	if code := call(path, `{}`); code != 409 {
		t.Fatalf("a paused routine got %d", code)
	}
	ta.Store.SetRoutineState(ctx, "pedido", store.RoutineActive)
	if code := call(path, strings.Repeat("x", webhookMax+10)); code != 413 {
		t.Fatalf("a large body got %d", code)
	}
	codes := map[int]int{}
	for range webhookPerMin + 2 {
		codes[call(path, `{"cliente":"x"}`)]++
	}
	if codes[429] == 0 {
		t.Fatalf("no limit: %v", codes)
	}
	ta.do(t, "POST", "/api/routines/pedido/webhook/off", nil)
	if code := call(path, `{}`); code != 404 {
		t.Fatalf("off still answers: %d", code)
	}
}
