package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type recordingHost struct {
	calls  []string
	result map[string]any
}

func (h *recordingHost) Call(_ context.Context, name, scope string, args any) (any, error) {
	h.calls = append(h.calls, name+"@"+scope)
	if name == "gmail.search" {
		return []map[string]any{{"id": "m1", "subject": "Invoice"}, {"id": "m2", "subject": "Lunch"}}, nil
	}
	if r, ok := h.result[name]; ok {
		if err, ok := r.(error); ok {
			return nil, err
		}
		return r, nil
	}
	return map[string]any{"ok": true}, nil
}

func (h *recordingHost) Judge(_ context.Context, name, _ string, item any) (float64, error) {
	if m, ok := item.(map[string]any); ok && m["id"] == "m1" {
		return 0.9, nil
	}
	return 0.1, nil
}

var brief = Manifest{
	Capabilities: []string{"gmail.search", "telegram.send", "http.getJSON:api.open-meteo.com"},
	Judgments:    map[string]string{"important": "Is this email important?"},
}

func TestRunAsyncRoutine(t *testing.T) {
	h := &recordingHost{result: map[string]any{"http.getJSON": map[string]any{"temp": 21}}}
	code := `
async function run() {
  const mails = await gmail.search({query: "is:unread", days: 1});
  const keep = [];
  for (const m of mails) { if ((await judge.important(m)).p > 0.5) keep.push(m.subject); }
  const w = await http.getJSON("https://api.open-meteo.com/v1/forecast?lat=1");
  log(now());
  await telegram.send({text: keep.join(",") + " " + w.temp});
}`
	res, err := Run(context.Background(), code, brief, h, Options{Now: time.Date(2026, 9, 24, 7, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	want := "gmail.search@ http.getJSON@api.open-meteo.com telegram.send@"
	if got := strings.Join(h.calls, " "); got != want {
		t.Fatalf("calls %q, want %q", got, want)
	}
	if res.Logs[0] != "2026-09-24T07:00:00Z" || res.Calls != 5 {
		t.Fatalf("result %+v", res)
	}
}

func TestRoutineCannotReachUndeclaredCapabilities(t *testing.T) {
	for name, code := range map[string]string{
		"capability not in manifest": `async function run() { await gmail.archive({id: "m1"}); }`,
		"host outside scope":         `async function run() { await http.getJSON("https://evil.example/x"); }`,
		"no fetch":                   `async function run() { await fetch("https://api.open-meteo.com"); }`,
		"no require":                 `function run() { require("fs"); }`,
	} {
		t.Run(name, func(t *testing.T) {
			h := &recordingHost{}
			if _, err := Run(context.Background(), code, brief, h, Options{}); err == nil {
				t.Fatal("routine escaped its manifest")
			}
			for _, c := range h.calls {
				if !strings.HasPrefix(c, "gmail.search") && !strings.HasPrefix(c, "telegram.send") {
					t.Fatalf("host saw forbidden call %s", c)
				}
			}
		})
	}
}

func TestRunawayRoutinesStop(t *testing.T) {
	loop := `function run() { for (;;) {} }`
	_, err := Run(context.Background(), loop, brief, &recordingHost{}, Options{Timeout: 50 * time.Millisecond})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("infinite loop: %v", err)
	}
	spam := `async function run() { for (;;) { await telegram.send({text: "hi"}); } }`
	_, err = Run(context.Background(), spam, brief, &recordingHost{}, Options{MaxCalls: 10})
	if err == nil || !strings.Contains(err.Error(), "more than 10") {
		t.Fatalf("call flood: %v", err)
	}
}

func TestErrorsReachTheCaller(t *testing.T) {
	h := &recordingHost{result: map[string]any{"telegram.send": errors.New("bot blocked")}}
	_, err := Run(context.Background(), `async function run() { await telegram.send({text: "x"}); }`, brief, h, Options{})
	if err == nil || !strings.Contains(err.Error(), "bot blocked") {
		t.Fatalf("host error: %v", err)
	}
	_, err = Run(context.Background(), `async function run() { throw new Error("boom"); }`, brief, h, Options{})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("thrown error: %v", err)
	}
	_, err = Run(context.Background(), `const x = 1;`, brief, h, Options{})
	if err == nil || !strings.Contains(err.Error(), "run()") {
		t.Fatalf("missing run: %v", err)
	}
}

func TestManifestValidation(t *testing.T) {
	for name, m := range map[string]Manifest{
		"empty":             {},
		"unknown":           {Capabilities: []string{"bank.transfer"}},
		"scope required":    {Capabilities: []string{"http.getJSON"}},
		"scope not allowed": {Capabilities: []string{"telegram.send:someone"}},
		"bad judgment":      {Capabilities: []string{"telegram.send"}, Judgments: map[string]string{"is important": "?"}},
	} {
		if m.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

type writingHost struct {
	recordingHost
	inputs []string
	sent   []string
}

func (h *writingHost) Call(ctx context.Context, name, scope string, args any) (any, error) {
	if m, ok := args.(map[string]any); ok && name == "telegram.send" {
		h.sent = append(h.sent, m["text"].(string))
	}
	return h.recordingHost.Call(ctx, name, scope, args)
}

func (h *writingHost) Write(_ context.Context, name, instruction string, input any) (string, error) {
	h.inputs = append(h.inputs, name+":"+input.(map[string]any)["subject"].(string))
	return "Ela pede a assinatura do contrato hoje.", nil
}

func TestWriteStep(t *testing.T) {
	m := Manifest{Schedule: "0 7 * * *", Capabilities: []string{"telegram.send"}, Writes: map[string]string{"pedido": "Diga em uma frase o que o e-mail pede"}}
	code := `async function run() { for (const e of event.items) { const w = await write.pedido(e); await telegram.send({text: e.subject + " — " + w.text}); } }`
	h := &writingHost{}
	ev := map[string]any{"items": []any{map[string]any{"subject": "Contrato Q4"}}}
	if _, err := Run(context.Background(), code, m, h, Options{Event: ev}); err != nil {
		t.Fatal(err)
	}
	if len(h.sent) != 1 || h.sent[0] != "Contrato Q4 — Ela pede a assinatura do contrato hoje." || h.inputs[0] != "pedido:Contrato Q4" {
		t.Fatalf("%v %v", h.sent, h.inputs)
	}
	if _, err := Run(context.Background(), code, m, &recordingHost{}, Options{Event: ev}); err == nil || !strings.Contains(err.Error(), "no model") {
		t.Fatalf("ran without a writer: %v", err)
	}
	many := `async function run() { for (let i = 0; i < 30; i++) await write.pedido({subject: "x"}); }`
	if _, err := Run(context.Background(), many, m, &writingHost{}, Options{}); err == nil || !strings.Contains(err.Error(), "more than 20 texts") {
		t.Fatalf("no cap on writes: %v", err)
	}
	m.Writes = map[string]string{"pedido": ""}
	if m.Validate() == nil {
		t.Fatal("accepted a write without instruction")
	}
}

func TestStateIsKeptBetweenRuns(t *testing.T) {
	m := Manifest{Schedule: "0 7 * * *", Capabilities: []string{"telegram.send"}}
	code := `async function run() {
  const last = state.get("price");
  const now = 120;
  if (last !== null && now > last) await telegram.send({text: "subiu de " + last + " para " + now});
  state.set("price", now);
  state.set("history", (state.get("history") || []).concat([now]).slice(-3));
}`
	h := &writingHost{}
	res, err := Run(context.Background(), code, m, h, Options{})
	if err != nil || !res.Changed || res.State["price"] != float64(120) || len(h.sent) != 0 {
		t.Fatalf("first run %+v %v %v", res, err, h.sent)
	}
	res, err = Run(context.Background(), code, m, h, Options{State: map[string]any{"price": 100.0}})
	if err != nil || len(h.sent) != 1 || h.sent[0] != "subiu de 100 para 120" {
		t.Fatalf("second run %v %v", err, h.sent)
	}
	big := `async function run() { state.set("x", "a".repeat(70000)); }`
	if _, err := Run(context.Background(), big, m, h, Options{}); err == nil || !strings.Contains(err.Error(), "64 KB") {
		t.Fatalf("no state limit: %v", err)
	}
}

func TestRoutinesUseOthersWithinTheirCapabilities(t *testing.T) {
	agenda := Helper{Code: `async function run() { const e = await calendar.events({}); return {count: e.length, first: params.label + e[0].title}; }`,
		Manifest: Manifest{Capabilities: []string{"calendar.events"}, Params: []Param{{Name: "label", Type: "text", Default: "→ "}}}}
	lib := func(_ context.Context, id string) (Helper, error) {
		switch id {
		case "agenda":
			return agenda, nil
		case "wide":
			return Helper{Code: `async function run() {}`, Manifest: Manifest{Capabilities: []string{"gmail.send"}}}, nil
		case "loop":
			return Helper{Code: `async function run() { await routines.run("brief") }`, Manifest: Manifest{Capabilities: []string{"telegram.send"}, Uses: []string{"brief"}}}, nil
		}
		return Helper{}, errors.New("no such routine")
	}
	h := &writingHost{recordingHost: recordingHost{result: map[string]any{"calendar.events": []map[string]any{{"title": "Dentista"}}}}}
	m := Manifest{Schedule: "0 7 * * *", Capabilities: []string{"calendar.events", "telegram.send"}, Uses: []string{"agenda", "wide", "loop"}}
	code := `async function run() { const a = await routines.run("agenda", {label: "* "}); await telegram.send({text: a.count + " " + a.first}); }`
	if _, err := Run(context.Background(), code, m, h, Options{Library: lib, ID: "brief"}); err != nil || len(h.sent) != 1 || h.sent[0] != "1 * Dentista" {
		t.Fatalf("helper: %v %v", err, h.sent)
	}
	for name, c := range map[string]string{
		"not in uses":       `async function run() { await routines.run("other") }`,
		"wider than caller": `async function run() { await routines.run("wide") }`,
		"loop":              `async function run() { await routines.run("loop") }`,
	} {
		if _, err := Run(context.Background(), c, m, h, Options{Library: lib, ID: "brief"}); err == nil {
			t.Errorf("%s: ran", name)
		}
	}
}
