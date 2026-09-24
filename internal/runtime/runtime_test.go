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
