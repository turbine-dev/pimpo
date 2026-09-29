//go:build live

package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/turbine-dev/pimpo/internal/connector/services"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/runtime"
	"github.com/turbine-dev/pimpo/internal/trace"
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

// A real trace that failed on a wrong test (an item 25 hours old expected
// in a 24-hour window) compiles, and retries see what was sent.
func TestLiveRealTraceCompiles(t *testing.T) {
	path := os.Getenv("PIMPO_TRACE")
	if path == "" {
		t.Skip("set PIMPO_TRACE to a recorded trace")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var tr trace.Trace
	if err := json.Unmarshal(raw, &tr); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	atts, err := Compiler{Model: llm.ClaudeCLI{}, Attempts: 3}.Compile(ctx, tr)
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range atts {
		t.Logf("attempt %d ($%.3f): %v %s", i+1, a.CostUSD, a.Problems(), a.Invalid)
	}
	last := atts[len(atts)-1]
	b, _ := json.MarshalIndent(last.Routine.Tests, "", " ")
	t.Logf("code:\n%s\ntests: %s", last.Routine.Code, b)
	if !last.Accepted() {
		t.Fatal("rejected")
	}
}

// "Only when it changed" compiles to a routine that keeps state.
func TestLiveStateIsCompiled(t *testing.T) {
	tr := trace.Trace{
		ID: "dolar", Request: "Todo dia às 9h me avise no Telegram só se o dólar subiu desde a última vez que você olhou", Now: "2026-09-25T09:00:00-03:00",
		Calls: []trace.Call{
			{Capability: "http.getJSON", Args: json.RawMessage(`"https://api.frankfurter.app/latest?from=USD&to=BRL"`), Result: json.RawMessage(`{"amount":1,"base":"USD","date":"2026-09-25","rates":{"BRL":5.41}}`)},
		},
		Outcome: "Guardar a cotação de cada dia e mandar mensagem só quando for maior que a anterior; hoje não houve mensagem porque não havia cotação anterior",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	atts, err := Compiler{Model: llm.ClaudeCLI{}, Attempts: 3}.Compile(ctx, tr)
	if err != nil {
		t.Fatal(err)
	}
	last := atts[len(atts)-1]
	for i, a := range atts {
		t.Logf("attempt %d ($%.3f): %v %s", i+1, a.CostUSD, a.Problems(), a.Invalid)
	}
	t.Logf("code:\n%s", last.Routine.Code)
	if !last.Accepted() || !strings.Contains(last.Routine.Code, "state.") {
		t.Fatal("not a stateful routine")
	}
}

// A request that builds on an installed routine reuses it.
func TestLiveUsesAnotherRoutine(t *testing.T) {
	agenda := runtime.Helper{Manifest: runtime.Manifest{Capabilities: []string{"calendar.events"}},
		Code: `async function run() { const evs = await calendar.events({from: dates.today(), to: dates.startOfDay(now(), 1)}); return evs.map(e => ({title: e.title, start: e.start})); }`}
	tr := trace.Trace{
		ID: "brief", Request: "Todo dia às 7h me manda quantos compromissos tenho hoje e o primeiro deles, usando a minha rotina agenda-hoje", Now: "2026-09-25T07:00:00-03:00",
		Calls: []trace.Call{
			{Capability: "calendar.events", Args: json.RawMessage(`{"from":"2026-09-25T00:00:00-03:00","to":"2026-09-26T00:00:00-03:00"}`), Result: json.RawMessage(`[{"title":"Dentista","start":"2026-09-25T10:00:00-03:00"},{"title":"Reunião","start":"2026-09-25T15:00:00-03:00"}]`)},
			{Capability: "notify.send", Args: json.RawMessage(`{"text":"Hoje: 2 compromissos. Primeiro: Dentista às 10:00"}`), Result: json.RawMessage(`{"ok":true}`)},
		},
		Outcome: "Uma mensagem com o total de compromissos de hoje e o primeiro",
	}
	c := Compiler{Model: llm.ClaudeCLI{}, Attempts: 3,
		Installed: func(context.Context) []Installed {
			return []Installed{{ID: "agenda-hoje", Name: "Agenda de hoje", Description: "Retorna os compromissos de hoje como [{title, start}] (não envia nada)", Capabilities: []string{"calendar.events"}}}
		},
		Helpers: func(_ context.Context, id string) (runtime.Helper, error) {
			if id == "agenda-hoje" {
				return agenda, nil
			}
			return runtime.Helper{}, errors.New("no such routine")
		}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	atts, err := c.Compile(ctx, tr)
	if err != nil {
		t.Fatal(err)
	}
	last := atts[len(atts)-1]
	for i, a := range atts {
		t.Logf("attempt %d ($%.3f): %v %s", i+1, a.CostUSD, a.Problems(), a.Invalid)
	}
	t.Logf("uses %v\ncode:\n%s", last.Routine.Manifest.Uses, last.Routine.Code)
	if !last.Accepted() || len(last.Routine.Manifest.Uses) != 1 || !strings.Contains(last.Routine.Code, "routines.run") {
		t.Fatal("did not reuse the routine")
	}
}
