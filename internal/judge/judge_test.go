package judge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/denerFernandes/pimpo/internal/llm"
)

func TestJevSendsANoulQuestion(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(401)
			return
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"answers":{"q":{"type":"noul","noul":0.87}}}`))
	}))
	defer srv.Close()
	j := Jev{BaseURL: srv.URL, Key: func(context.Context) (string, error) { return "k", nil }}
	a, err := j.Ask(context.Background(), "Is this email important?", map[string]string{"subject": "Contrato"})
	if err != nil || a.P != 0.87 || a.Backend != "jev" {
		t.Fatalf("%+v %v", a, err)
	}
	q := got["questions"].(map[string]any)["q"].(map[string]any)
	if q["type"] != "noul" || !strings.Contains(q["instructions"].(string), "important") {
		t.Fatalf("question %+v", q)
	}
	bad := Jev{BaseURL: srv.URL, Key: func(context.Context) (string, error) { return "wrong", nil }}
	if _, err := bad.Ask(context.Background(), "?", nil); err == nil {
		t.Fatal("401 accepted")
	}
}

func TestLLMBackend(t *testing.T) {
	fake := &llm.Fake{Responses: []llm.Response{{Structured: json.RawMessage(`{"p":1.4}`), CostUSD: 0.001}}}
	a, err := LLM{Model: fake}.Ask(context.Background(), "Is it spam?", "hello")
	if err != nil || a.P != 1 || a.CostUSD != 0.001 {
		t.Fatalf("%+v %v", a, err)
	}
}

func TestOllamaReadsTokenProbabilities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"message":{"content":"yes"},"logprobs":[{"top_logprobs":[{"token":"yes","logprob":-0.105},{"token":"no","logprob":-2.302}]}]}`))
	}))
	defer srv.Close()
	a, err := Ollama{BaseURL: srv.URL, Model: "qwen3:0.6b"}.Ask(context.Background(), "?", "x")
	if err != nil || a.P < 0.89 || a.P > 0.91 {
		t.Fatalf("%+v %v", a, err)
	}
}

type failing struct{}

func (failing) Ask(context.Context, string, any) (Answer, error) { return Answer{}, errors.New("down") }

func TestChainFallsBack(t *testing.T) {
	fake := &llm.Fake{Responses: []llm.Response{{Structured: json.RawMessage(`{"p":0.3}`)}}}
	a, err := Chain{failing{}, LLM{Model: fake}}.Ask(context.Background(), "?", 1)
	if err != nil || a.P != 0.3 {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := (Chain{failing{}}).Ask(context.Background(), "?", 1); err == nil || !strings.Contains(err.Error(), "down") {
		t.Fatalf("%v", err)
	}
}

func TestLocalJudgeServer(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"p":0.83}`))
	}))
	defer srv.Close()
	a, err := Local{URL: srv.URL}.Ask(context.Background(), "Is it important?", map[string]string{"subject": "Contrato"})
	if err != nil || a.P != 0.83 || a.Backend != "local" || got["question"] != "Is it important?" {
		t.Fatalf("%+v %v %v", a, err, got)
	}
	if _, err := (Local{URL: "http://127.0.0.1:1"}).Ask(context.Background(), "?", nil); err == nil {
		t.Fatal("unreachable server accepted")
	}
}

type fixed struct {
	p     float64
	err   error
	asked *int
}

func (f fixed) Ask(context.Context, string, any) (Answer, error) {
	if f.asked != nil {
		*f.asked++
	}
	return Answer{P: f.p, Backend: fmt.Sprint(f.p), CostUSD: 0.01}, f.err
}

func TestCascadeEscalatesOnlyWhenUnsure(t *testing.T) {
	ctx := context.Background()
	n := 0
	strong := fixed{p: 0.99, asked: &n}
	if a, _ := (Cascade{First: fixed{p: 0.95}, Then: strong, Band: 0.4}).Ask(ctx, "q", nil); a.P != 0.95 || n != 0 {
		t.Fatalf("sure answer escalated: %+v %d", a, n)
	}
	if a, _ := (Cascade{First: fixed{p: 0.3}, Then: strong, Band: 0.4}).Ask(ctx, "q", nil); a.P != 0.99 || n != 1 || a.CostUSD != 0.02 {
		t.Fatalf("unsure answer kept: %+v", a)
	}
	if a, err := (Cascade{First: fixed{p: 0.6}, Then: fixed{err: errors.New("down")}, Band: 0.4}).Ask(ctx, "q", nil); err != nil || a.P != 0.6 {
		t.Fatalf("strong judge down should keep the cheap answer: %+v %v", a, err)
	}
	if a, err := (Cascade{First: fixed{err: errors.New("no local")}, Then: strong, Band: 0.4}).Ask(ctx, "q", nil); err != nil || a.P != 0.99 {
		t.Fatalf("cheap judge down: %+v %v", a, err)
	}
}
