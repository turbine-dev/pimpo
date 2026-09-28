package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/denerFernandes/pimpo/internal/event"
	"github.com/denerFernandes/pimpo/internal/llm"
	"github.com/denerFernandes/pimpo/internal/routine"
	"github.com/denerFernandes/pimpo/internal/runtime"
)

// A routine asks, the answer runs it again with event.answer, and it
// keeps what was answered.
func TestAskAndAnswer(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	r := routine.Routine{Name: "Treino", Manifest: runtime.Manifest{Schedule: "0 21 * * *", Capabilities: []string{"ask.owner", "notify.send"}},
		Code: `async function run() {
  if (event.answer) {
    const n = (state.get("sim") || 0) + (event.answer.choice === "Sim" ? 1 : 0)
    state.set("sim", n)
    await notify.send({text: "Treinos na semana: " + n})
    return
  }
  await ask.owner({question: "Treinou hoje?", options: ["Sim", "Não"], key: "treino"})
}`}
	ta.Store.SaveRoutine(ctx, "treino", r, "test", "human:owner")
	if _, err := ta.Scheduler.RunNow(ctx, "treino", "owner"); err != nil {
		t.Fatal(err)
	}
	_, out := ta.do(t, "GET", "/api/questions", nil)
	qs := out["list"].([]any)
	if len(qs) != 1 || qs[0].(map[string]any)["question"] != "Treinou hoje?" {
		t.Fatalf("questions %v", out)
	}
	id := qs[0].(map[string]any)["id"].(string)
	// The same question asked again replaces the pending one.
	ta.Scheduler.RunNow(ctx, "treino", "owner")
	_, out = ta.do(t, "GET", "/api/questions", nil)
	if n := len(out["list"].([]any)); n != 1 {
		t.Fatalf("%d pending", n)
	}
	id = out["list"].([]any)[0].(map[string]any)["id"].(string)
	if text, err := (handler{ta.App}).Button(ctx, "answer", id+".0"); err != nil || !strings.Contains(text, "Sim") {
		t.Fatalf("%q %v", text, err)
	}
	var got string
	for range 100 {
		evs, _ := ta.Events.List(ctx, event.Query{Types: []string{"notice.sent"}})
		for _, e := range evs {
			var n struct{ Text string }
			e.Decode(&n)
			if strings.HasPrefix(n.Text, "Treinos") {
				got = n.Text
			}
		}
		if got != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got != "Treinos na semana: 1" {
		t.Fatalf("sent %q", got)
	}
	if _, err := (handler{ta.App}).Button(ctx, "answer", id+".1"); err == nil {
		t.Fatal("answered twice")
	}
	if _, err := (askCap{ta.App}).Call(ctx, "ask.owner", "", map[string]any{"question": "x", "options": []any{"só uma"}}); err == nil {
		t.Fatal("accepted one option")
	}
}
