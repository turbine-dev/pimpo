package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/denerFernandes/pimpo/internal/llm"
	"github.com/denerFernandes/pimpo/internal/routine"
	"github.com/denerFernandes/pimpo/internal/runtime"
	"github.com/denerFernandes/pimpo/internal/store"
)

type brokenLink struct{ fakeLink }

func (*brokenLink) Check(context.Context) error { return errors.New("slack refused the app token") }

// The doctor tests each part for real and says what to do about failures,
// failures first.
func TestDoctor(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := t.Context()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":5,"completion_tokens":1}}`)
	}))
	defer good.Close()
	refused := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer refused.Close()
	modelBase["openai"], modelBase["groq"] = good.URL, refused.URL
	defer delete(modelBase, "openai")
	defer delete(modelBase, "groq")
	ta.do(t, "PUT", "/api/models/keys/openai", map[string]string{"key": "o"})
	ta.do(t, "PUT", "/api/models/keys/groq", map[string]string{"key": "g"})
	s := ta.Settings(ctx)
	s.Models = []ModelOption{{ID: "openai:gpt-5-mini", PriceIn: 0.25, PriceOut: 2}, {ID: "groq:llama-4", PriceIn: 0.1, PriceOut: 0.3}}
	s.ExploreModel = "openai:gpt-5-mini"
	s.Fallbacks = map[string][]string{"explore": {"groq:llama-4"}}
	ta.do(t, "PUT", "/api/settings", s)
	ta.links = map[string]*linkRun{"slackchat": {link: &brokenLink{}, cancel: func() {}}}
	ta.Store.SaveRoutine(ctx, "r1", routine.Routine{Name: "R", Code: "async function run(){}", Manifest: runtime.Manifest{Schedule: "0 7 * * *", Capabilities: []string{"telegram.send"}}}, "t", "owner")
	ta.Store.SetRoutineState(ctx, "r1", store.RoutineBroken)

	code, out := ta.do(t, "POST", "/api/doctor", nil)
	if code != 200 {
		t.Fatal(code)
	}
	by := map[string]map[string]any{}
	var order []string
	for _, f := range out["list"].([]any) {
		m := f.(map[string]any)
		by[m["id"].(string)] = m
		order = append(order, m["state"].(string))
	}
	if f := by["slackchat"]; f["state"] != "fail" || f["fix"] != "doc.fix.linkCheck" {
		t.Fatalf("slack %v", f)
	}
	if f := by["model:groq:llama-4"]; f["state"] != "fail" || f["fix"] != "doc.fix.model.key" {
		t.Fatalf("groq %v", f)
	}
	if f := by["model:openai:gpt-5-mini"]; f["state"] != "ok" {
		t.Fatalf("openai %v", f)
	}
	if f := by["routines"]; f["state"] != "warn" || f["link"] != "/routines" {
		t.Fatalf("routines %v", f)
	}
	for i := 1; i < len(order); i++ {
		rank := map[string]int{"fail": 0, "warn": 1, "ok": 2}
		if rank[order[i]] < rank[order[i-1]] {
			t.Fatalf("not failures first: %v", order)
		}
	}
}
