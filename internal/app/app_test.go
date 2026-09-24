package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denerFernandes/vigia/internal/event"
	"github.com/denerFernandes/vigia/internal/llm"
	"github.com/denerFernandes/vigia/internal/vault"
)

type testApp struct {
	*App
	srv *httptest.Server
}

func newApp(t *testing.T, agent llm.Agent, model llm.Model) *testApp {
	t.Helper()
	dir := t.TempDir()
	ev, err := event.Open(filepath.Join(dir, "v.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ev.Close() })
	v, _ := vault.Open(ev.DB(), vault.FileKey(filepath.Join(dir, "k")))
	a, err := New(context.Background(), ev, v, "tok", "")
	if err != nil {
		t.Fatal(err)
	}
	a.Agent, a.LLM = agent, model
	if err := a.AttachMemory(filepath.Join(dir, "memory")); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Server)
	t.Cleanup(srv.Close)
	a.Explore.BaseURL = srv.URL
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a.Start(ctx)
	return &testApp{a, srv}
}

func (ta *testApp) do(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, ta.srv.URL+path, r)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		var list []any
		json.Unmarshal(raw, &list)
		out = map[string]any{"list": list}
	}
	return resp.StatusCode, out
}

func rpc(url string, id int, tool string, args any) error {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	if res, _ := out["result"].(map[string]any); res["isError"] == true {
		return fmt.Errorf("%s: %v", tool, res["content"])
	}
	return nil
}

// The explorer only reads the budget-free weather and "sends" to the
// owner; Telegram is not connected, so the send fails and the explorer
// reports it, which is still a finished exploration.
var weatherAgent = llm.FakeAgent{Script: func(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
	rpc(r.MCPURL, 1, "telegram_send", map[string]any{"text": "Bom dia!"})
	return llm.Response{Text: "Mandaria 'Bom dia!' todo dia.", CostUSD: 0.1}, nil
}}

const greeting = `{"name":"Bom dia","description":"Manda bom dia","manifest":{"schedule":"0 7 * * *","capabilities":["telegram.send"],"locale":"pt-BR"},
"code":"async function run() { await telegram.send({text: \"Bom dia!\"}); }","tests":[]}`

func TestExploreApproveRunThroughTheAPI(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{Responses: []llm.Response{{Structured: json.RawMessage(greeting), CostUSD: 0.05}}})
	code, out := ta.do(t, "POST", "/api/explorations", map[string]string{"request": "Todo dia às 7h me manda bom dia"})
	if code != 202 {
		t.Fatalf("start %d %v", code, out)
	}
	id := out["id"].(string)
	ta.Explore.Wait()
	_, out = ta.do(t, "GET", "/api/explorations/"+id, nil)
	exp := out["exploration"].(map[string]any)
	if exp["state"] != "ready" {
		t.Fatalf("exploration %v", exp)
	}
	if acts := out["actions"].([]any); len(acts) != 1 {
		t.Fatalf("recorded actions %v", acts)
	}
	code, out = ta.do(t, "POST", "/api/explorations/"+id+"/compile", nil)
	if code != 200 || out["id"] != "bom-dia" || out["next_run"] == nil {
		t.Fatalf("compile %d %v", code, out)
	}
	_, out = ta.do(t, "GET", "/api/routines", nil)
	if len(out["list"].([]any)) != 1 {
		t.Fatalf("routines %v", out)
	}
	// Telegram is not connected, so running fails, pauses the routine and says why.
	_, out = ta.do(t, "POST", "/api/routines/bom-dia/run", nil)
	if !strings.Contains(out["error"].(string), "Connections") {
		t.Fatalf("run %v", out)
	}
	_, out = ta.do(t, "GET", "/api/routines/bom-dia", nil)
	if out["summary"].(map[string]any)["state"] != "broken" {
		t.Fatalf("detail %v", out["summary"])
	}
	_, state := ta.do(t, "GET", "/api/state", nil)
	if state["healthy"] != false || state["log_intact"] != true {
		t.Fatalf("state %v", state)
	}
	if spent := state["budget"].(map[string]any)["spent"].(float64); spent < 0.149 {
		t.Fatalf("spent %v", spent)
	}
}

func TestEmptyListsAreArraysNotNull(t *testing.T) {
	ta := newApp(t, llm.FakeAgent{Script: func(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
		return llm.Response{Text: "nada"}, nil
	}}, &llm.Fake{})
	_, out := ta.do(t, "POST", "/api/explorations", map[string]string{"request": "algo"})
	ta.Explore.Wait()
	_, out = ta.do(t, "GET", "/api/explorations/"+out["id"].(string), nil)
	if _, ok := out["actions"].([]any); !ok {
		t.Fatalf("actions is %T, want a list", out["actions"])
	}
	for _, path := range []string{"/api/routines", "/api/explorations", "/api/receipts", "/api/approvals", "/api/rules", "/api/events"} {
		_, out := ta.do(t, "GET", path, nil)
		if _, ok := out["list"].([]any); !ok {
			t.Fatalf("%s returned %v, want a list", path, out)
		}
	}
}

func TestConnectionsValidateAndHideSecrets(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	code, _ := ta.do(t, "PUT", "/api/connections/calendar", map[string]string{"feeds": "http://insecure.example/basic.ics"})
	if code != 400 {
		t.Fatalf("http feed accepted: %d", code)
	}
	code, _ = ta.do(t, "PUT", "/api/connections/calendar", map[string]string{"feeds": "https://calendar.google.com/calendar/ical/x/private-abc/basic.ics\n"})
	if code != 200 {
		t.Fatalf("calendar %d", code)
	}
	code, _ = ta.do(t, "PUT", "/api/connections/mail", map[string]string{"user": "eu@gmail.com", "password": "abcd efgh ijkl mnop"})
	if code != 200 {
		t.Fatalf("mail %d", code)
	}
	_, out := ta.do(t, "GET", "/api/connections", nil)
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "private-abc") || strings.Contains(string(raw), "abcdefgh") {
		t.Fatalf("secret leaked: %s", raw)
	}
	if !strings.Contains(string(raw), `"detail":"1 agenda"`) || !strings.Contains(string(raw), "eu@gmail.com") {
		t.Fatalf("connections %s", raw)
	}
	pw, _ := ta.Vault.Get(context.Background(), "mail.password")
	if pw != "abcdefghijklmnop" {
		t.Fatalf("app password not normalized: %q", pw)
	}
}

func TestSettingsAndBudget(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	code, _ := ta.do(t, "PUT", "/api/settings", Settings{Zone: "Mars/Olympus", JudgeBackend: "local"})
	if code != 400 {
		t.Fatalf("bad zone accepted: %d", code)
	}
	code, out := ta.do(t, "PUT", "/api/settings", Settings{Zone: "America/Sao_Paulo", Locale: "pt-BR", JudgeBackend: "jev"})
	if code != 200 || out["judge_backend"] != "jev" {
		t.Fatalf("settings %d %v", code, out)
	}
	ta.do(t, "PUT", "/api/budget", map[string]float64{"daily_usd": 0.5})
	_, state := ta.do(t, "GET", "/api/state", nil)
	if state["budget"].(map[string]any)["limit"] != 0.5 {
		t.Fatalf("budget %v", state["budget"])
	}
}

func TestSetupAndPresets(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	_, s := ta.do(t, "GET", "/api/setup", nil)
	if s["done"] != false || s["telegram"] != false {
		t.Fatalf("fresh setup %v", s)
	}
	code, rules := ta.do(t, "PUT", "/api/rules/preset", map[string]string{"preset": "conservative"})
	if code != 200 || len(rules["list"].([]any)) != 1 {
		t.Fatalf("preset %d %v", code, rules)
	}
	if code, _ := ta.do(t, "PUT", "/api/rules/preset", map[string]string{"preset": "yolo"}); code != 400 {
		t.Fatalf("unknown preset accepted: %d", code)
	}
	ta.do(t, "POST", "/api/setup/done", nil)
	_, s = ta.do(t, "GET", "/api/setup", nil)
	if s["done"] != true || s["preset"] != "conservative" {
		t.Fatalf("after setup %v", s)
	}
}

func TestMemoryAPI(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	code, f := ta.do(t, "POST", "/api/memory", map[string]string{"text": "Minha chefe é a Ana", "topic": "trabalho"})
	if code != 200 || f["trust"] != "high" {
		t.Fatalf("add %d %v", code, f)
	}
	_, m := ta.do(t, "GET", "/api/memory", nil)
	if len(m["facts"].([]any)) != 1 || len(m["history"].([]any)) != 1 {
		t.Fatalf("memory %v", m)
	}
	hash := m["history"].([]any)[0].(map[string]any)["hash"].(string)
	ta.do(t, "DELETE", "/api/memory/"+f["id"].(string), nil)
	ta.do(t, "POST", "/api/memory-versions/"+hash+"/restore", nil)
	_, m = ta.do(t, "GET", "/api/memory", nil)
	if len(m["facts"].([]any)) != 1 {
		t.Fatalf("after restore %v", m["facts"])
	}
}
