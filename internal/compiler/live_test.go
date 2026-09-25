//go:build live

package compiler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/denerFernandes/zodim/internal/llm"
	"github.com/denerFernandes/zodim/internal/trace"
)

// A real model compiles "tell me when Ana emails me" into a watch, not a
// schedule that would repeat the same alert.
func TestLiveWatchIsCompiled(t *testing.T) {
	tr := trace.Trace{
		ID: "ana", Request: "Me avise no Telegram quando chegar e-mail da Ana (ana@acme.com)", Now: "2026-09-25T09:00:00-03:00",
		Calls: []trace.Call{
			{Capability: "gmail.search", Args: json.RawMessage(`{"query":"from:ana@acme.com","days":2}`), Result: json.RawMessage(`[{"id":"m-9","from":"ana@acme.com","from_name":"Ana Souza","subject":"Contrato Q4","snippet":"Precisa da sua assinatura hoje"}]`)},
			{Capability: "telegram.send", Args: json.RawMessage(`{"text":"📬 Ana Souza: Contrato Q4"}`), Result: json.RawMessage(`{"ok":true}`)},
		},
		Outcome: "Uma mensagem no Telegram com o assunto de cada e-mail novo da Ana",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	atts, err := Compiler{Model: llm.ClaudeCLI{}, Attempts: 2}.Compile(ctx, tr)
	if err != nil {
		t.Fatal(err)
	}
	last := atts[len(atts)-1]
	b, _ := json.MarshalIndent(last.Routine.Manifest, "", " ")
	t.Logf("manifest: %s\ncode:\n%s\nproblems: %v %s", b, last.Routine.Code, last.Problems(), last.Invalid)
	if !last.Accepted() {
		t.Fatalf("rejected: %s %v", last.Invalid, last.Problems())
	}
	if last.Routine.Manifest.Watch == nil {
		t.Fatal("compiled to a schedule instead of a watch")
	}
}

// Composed text becomes a write step; copied fields stay plain code.
func TestLiveWriteIsCompiled(t *testing.T) {
	tr := trace.Trace{
		ID: "ana-ask", Request: "Quando chegar e-mail da Ana (ana@acme.com), me avise no Telegram com o assunto e, em uma frase, o que ela está pedindo", Now: "2026-09-25T09:00:00-03:00",
		Calls: []trace.Call{
			{Capability: "gmail.search", Args: json.RawMessage(`{"query":"from:ana@acme.com","days":2}`), Result: json.RawMessage(`[{"id":"m-9","from":"ana@acme.com","from_name":"Ana Souza","subject":"Contrato Q4","snippet":"Oi! Preciso que você assine o contrato do Q4 até hoje às 18h, senão perdemos o desconto."}]`)},
			{Capability: "telegram.send", Args: json.RawMessage(`{"text":"📬 Ana Souza: Contrato Q4\nEla pede que você assine o contrato do Q4 até as 18h de hoje."}`), Result: json.RawMessage(`{"ok":true}`)},
		},
		Outcome: "Uma mensagem com o assunto e uma frase dizendo o que a Ana pede",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	atts, err := Compiler{Model: llm.ClaudeCLI{}, Attempts: 2}.Compile(ctx, tr)
	if err != nil {
		t.Fatal(err)
	}
	last := atts[len(atts)-1]
	b, _ := json.MarshalIndent(last.Routine.Manifest, "", " ")
	t.Logf("manifest: %s\ncode:\n%s\nproblems: %v %s", b, last.Routine.Code, last.Problems(), last.Invalid)
	if !last.Accepted() {
		t.Fatalf("rejected: %s %v", last.Invalid, last.Problems())
	}
	if len(last.Routine.Manifest.Writes) == 0 || last.Routine.Manifest.Watch == nil {
		t.Fatal("expected a watch with a write step")
	}
}
