package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/denerFernandes/pimpo/internal/llm"
	"github.com/denerFernandes/pimpo/internal/routine"
	"github.com/denerFernandes/pimpo/internal/runtime"
)

func TestRoutineSettingsAndDestinations(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()

	// Two Telegram bots: the owner's and a family group bot.
	var mu sync.Mutex
	sent := map[string][]string{}
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		tok := strings.Split(strings.TrimPrefix(r.URL.Path, "/bot"), "/")[0]
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			name := map[string]string{"owner-tok": "pimpo_bot", "family-tok": "familia_bot"}[tok]
			if name == "" {
				w.WriteHeader(401)
				w.Write([]byte(`{"ok":false,"description":"Unauthorized"}`))
				return
			}
			w.Write([]byte(`{"ok":true,"result":{"username":"` + name + `"}}`))
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			w.Write([]byte(`{"ok":true,"result":[{"update_id":1,"message":{"message_id":1,"text":"oi","chat":{"id":-500,"title":"Família"},"from":{"first_name":"Ana"}}}]}`))
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			mu.Lock()
			sent[tok] = append(sent[tok], body["text"].(string))
			mu.Unlock()
			w.Write([]byte(`{"ok":true,"result":{"message_id":2,"chat":{"id":1}}}`))
		}
	}))
	defer tg.Close()
	ta.TelegramAPI = tg.URL
	ta.Vault.Set(ctx, "telegram.token", "owner-tok")
	ta.Events.Put(ctx, "telegram.chat", "42")
	if code, _ := ta.do(t, "POST", "/api/telegram/bots", map[string]string{"name": "Família", "token": "bad"}); code != 400 {
		t.Fatalf("accepted a bad token: %d", code)
	}
	_, bot := ta.do(t, "POST", "/api/telegram/bots", map[string]string{"name": "Família", "token": "family-tok"})
	id := bot["id"].(string)
	if _, d := ta.do(t, "POST", "/api/telegram/bots/"+id+"/detect", nil); d["chat"] != -500.0 || d["chat_name"] != "Família" {
		t.Fatalf("detect %v", d)
	}
	_, dests := ta.do(t, "GET", "/api/destinations", nil)
	ready := map[string]bool{}
	for _, d := range dests["list"].([]any) {
		m := d.(map[string]any)
		ready[m["id"].(string)] = m["ready"].(bool)
	}
	if !ready["telegram"] || !ready["bot:"+id] || ready["whatsapp"] {
		t.Fatalf("destinations %v", ready)
	}

	// A routine with parameters.
	body := routine.Routine{Name: "Clima", Code: `async function run() { await notify.send({text: "Clima em " + params.cidade.name + " (" + params.chuva + "%)"}) }`,
		Manifest: runtime.Manifest{Schedule: "0 7 * * *", Capabilities: []string{"notify.send"}, Params: []runtime.Param{
			{Name: "cidade", Label: "Cidade", Type: "location", Default: map[string]any{"name": "São Paulo", "latitude": -23.55, "longitude": -46.63}},
			{Name: "chuva", Label: "Avisar chuva a partir de (%)", Type: "number", Default: 50.0},
			{Name: "destinos", Label: "Onde avisar", Type: "destinations", Default: []any{}},
		}}}
	ta.Store.SaveRoutine(ctx, "clima", body, "test", "human:owner")
	ta.Scheduler.Changed(ctx, "clima")
	_, sum := ta.do(t, "GET", "/api/routines", nil)
	first := sum["list"].([]any)[0].(map[string]any)
	if len(first["params"].([]any)) != 3 || first["values"].(map[string]any)["chuva"] != 50.0 {
		t.Fatalf("summary %v", first)
	}

	for _, bad := range []map[string]any{
		{"schedule": "every morning"},
		{"params": map[string]any{"chuva": "muito"}},
		{"params": map[string]any{"destinos": []string{"pigeon"}}},
		{"params": map[string]any{"ghost": 1}},
	} {
		if code, _ := ta.do(t, "PUT", "/api/routines/clima/settings", bad); code != 400 {
			t.Errorf("accepted %v", bad)
		}
	}
	lisboa := map[string]any{"name": "Lisboa", "latitude": 38.72, "longitude": -9.14, "timezone": "Europe/Lisbon"}
	code, out := ta.do(t, "PUT", "/api/routines/clima/settings", map[string]any{"schedule": "30 6 * * 1-5", "params": map[string]any{"cidade": lisboa, "chuva": 70, "destinos": []string{"telegram", "bot:" + id}}})
	if code != 200 || out["schedule"] != "30 6 * * 1-5" || out["default_schedule"] != "0 7 * * *" {
		t.Fatalf("save %d %v", code, out)
	}
	if n := ta.Scheduler.Next("clima"); n.Hour() != 6 || n.Minute() != 30 {
		t.Fatalf("the scheduler did not take the new time: %v", n)
	}
	if code, run := ta.do(t, "POST", "/api/routines/clima/run", nil); code != 200 || run["error"] != "" {
		t.Fatalf("run %d %v", code, run)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sent["owner-tok"]) != 1 || len(sent["family-tok"]) != 1 || sent["family-tok"][0] != "Clima em Lisboa (70%)" {
		t.Fatalf("delivered %v", sent)
	}
}

func TestGeocode(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	geo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "Lisboa" || r.URL.Query().Get("language") != "pt" {
			w.Write([]byte(`{}`))
			return
		}
		w.Write([]byte(`{"results":[{"name":"Lisboa","latitude":38.72,"longitude":-9.14,"timezone":"Europe/Lisbon","country":"Portugal","admin1":"Lisboa"},{"name":"Lisboa","latitude":-5.1,"longitude":-42.8,"timezone":"America/Fortaleza","country":"Brasil","admin1":"Piauí"}]}`))
	}))
	defer geo.Close()
	ta.GeocodeAPI = geo.URL
	_, out := ta.do(t, "GET", "/api/geocode?q=Lisboa", nil)
	list := out["list"].([]any)
	if len(list) != 2 || list[1].(map[string]any)["name"] != "Lisboa, Piauí" || list[0].(map[string]any)["name"] != "Lisboa" {
		t.Fatalf("%v", list)
	}
	if _, out := ta.do(t, "GET", "/api/geocode?q=L", nil); len(out["list"].([]any)) != 0 {
		t.Fatal("searched with one letter")
	}
}
