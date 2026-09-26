package app

import (
	"context"
	"strings"
	"testing"

	"github.com/denerFernandes/pimpo/internal/judge"
	"github.com/denerFernandes/pimpo/internal/llm"
	"github.com/denerFernandes/pimpo/internal/memory"
)

// sameJudge says two facts match unless their texts differ in a digit.
type sameJudge struct{ asked int }

func (s *sameJudge) Ask(_ context.Context, _ string, item any) (judge.Answer, error) {
	s.asked++
	m := item.(map[string]string)
	digits := func(t string) string {
		return strings.Map(func(r rune) rune { return map[bool]rune{true: r, false: -1}[r >= '0' && r <= '9'] }, t)
	}
	if digits(m["a"]) != digits(m["b"]) {
		return judge.Answer{P: 0.1}, nil
	}
	return judge.Answer{P: 0.95}, nil
}

func TestOrganizeMergesDuplicatesOnly(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	j := &sameJudge{}
	ta.DemoJudge = j
	m := ta.Memory
	m.Add("Academia às terças", "rotina", "owner", memory.High)
	m.Add("Vou à academia às terças", "rotina", "email", memory.Low)
	m.Add("Reunião com a Ana dia 12", "agenda", "owner", memory.High)
	m.Add("Reunião com a Ana dia 19", "agenda", "email", memory.Low)
	m.Add("Minha irmã se chama Ana", "família", "owner", memory.High)
	before, _ := m.History(1)
	if _, never := ta.do(t, "GET", "/api/memory/organized", nil); never["at"] != nil || never["merged"] == nil {
		t.Fatalf("never organized: %v", never)
	}

	code, out := ta.do(t, "POST", "/api/memory/organize", nil)
	if code != 200 || out["checked"] != float64(2) || len(out["merged"].([]any)) != 1 {
		t.Fatalf("%d %v", code, out)
	}
	merged := out["merged"].([]any)[0].(map[string]any)
	if merged["kept"] != "Academia às terças" || merged["dropped"] != "Vou à academia às terças" {
		t.Fatalf("%v", merged)
	}
	facts, _ := m.List()
	if len(facts) != 4 {
		t.Fatalf("%d facts", len(facts))
	}
	_, last := ta.do(t, "GET", "/api/memory/organized", nil)
	if len(last["merged"].([]any)) != 1 {
		t.Fatalf("%v", last)
	}
	m.Restore(before[0].Hash)
	if facts, _ := m.List(); len(facts) != 5 {
		t.Fatal("organizing could not be undone")
	}
}

type fakeChooser struct{ options map[string]string }

func (f *fakeChooser) Choose(_ context.Context, _ string, _ any, options map[string]string) (map[string]float64, error) {
	f.options = options
	out := map[string]float64{"none": 0.1}
	for id, text := range options {
		if strings.Contains(text, "Ana") {
			out[id] = 0.8
		} else if id != "none" {
			out[id] = 0.01
		}
	}
	return out, nil
}

func TestSearchMemoryByMeaning(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.Memory.Add("Minha irmã se chama Ana", "família", "owner", memory.High)
	ta.Memory.Add("Academia às terças", "rotina", "owner", memory.High)
	ta.Memory.AddFor("Ana gosta de chocolate", "gostos", "owner", memory.High, "ana")

	_, out := ta.do(t, "GET", "/api/memory/search?q=academia", nil)
	if out["meaning"] != false || len(out["facts"].([]any)) != 1 {
		t.Fatalf("words only: %v", out)
	}
	fc := &fakeChooser{}
	old := meaningJudge
	meaningJudge = func(*App) (chooser, bool) { return fc, true }
	defer func() { meaningJudge = old }()
	_, out = ta.do(t, "GET", "/api/memory/search?q=quem%20é%20da%20família", nil)
	facts := out["facts"].([]any)
	if out["meaning"] != true || len(facts) != 1 || facts[0].(map[string]any)["text"] != "Minha irmã se chama Ana" || facts[0].(map[string]any)["by"] != "meaning" {
		t.Fatalf("%v", out)
	}
	for _, text := range fc.options {
		if text == "Ana gosta de chocolate" {
			t.Fatal("another person's fact reached the search")
		}
	}
}
