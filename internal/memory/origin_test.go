package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func keys(f Fact) []string {
	var out []string
	for _, o := range f.From() {
		out = append(out, o.Key())
	}
	return out
}

func TestEveryFactKeepsItsSources(t *testing.T) {
	m, _ := Open(t.TempDir())
	chat := Origin{Kind: FromConversation, Ref: "c1", Turn: "e1", Label: "Planos\nde sexta"}
	f, _ := m.AddFrom("Ana é a chefe", "trabalho", "exploration:e1", Low, "", chat)
	if got := f.From(); len(got) != 1 || got[0].Key() != "conversation:c1" || got[0].Label != "Planos de sexta" {
		t.Fatalf("%+v", got)
	}
	// The same fact read again in an email adds that source.
	mail := Origin{Kind: FromEmail, Ref: "<m1@x>", Sender: "Bob@X.com"}
	f, _ = m.AddFrom("ana é a chefe", "trabalho", "exploration:e2", Low, "", mail)
	if k := keys(f); len(k) != 2 || k[1] != "email:bob@x.com" {
		t.Fatalf("%v", k)
	}
	// What the person typed does not become forgettable with a note the
	// agent read somewhere.
	typed, _ := m.AddFrom("Moro em Lisboa", "pessoal", "owner", High, "", Origin{Kind: FromTyped})
	again, _ := m.AddFrom("moro em lisboa", "pessoal", "exploration:e3", Low, "", mail)
	if k := keys(again); len(k) != 1 || k[0] != FromTyped || again.ID != typed.ID {
		t.Fatalf("%v", k)
	}
}

func TestForgetSourceRemovesExactlyItsFacts(t *testing.T) {
	m, _ := Open(t.TempDir())
	chat := Origin{Kind: FromConversation, Ref: "c1"}
	other := Origin{Kind: FromConversation, Ref: "c2"}
	m.AddFrom("A", "t", "x", Low, "", chat)
	m.AddFrom("B", "t", "x", Low, "", chat, Origin{Kind: FromEmail, Sender: "b@x.com"})
	m.AddFrom("C", "t", "x", Low, "", other)
	m.AddFrom("D", "t", "x", Low, "ana", chat) // Ana's, same key
	m.AddFrom("E", "t", "ana", High, "casa", chat)
	m.AddFrom("F", "t", "owner", High, "casa", chat)
	before, _ := m.History(1)

	gone, err := m.ForgetSource("conversation:c1", "owner", "forget source: conversation")
	if err != nil || len(gone) != 3 {
		t.Fatalf("%v %+v", err, gone)
	}
	left := map[string]bool{}
	all, _ := m.List()
	for _, f := range all {
		left[f.Text] = true
	}
	// The owner's A, B and house F go; C, Ana's D and Ana's house E stay.
	if left["A"] || left["B"] || left["F"] || !left["C"] || !left["D"] || !left["E"] {
		t.Fatalf("%v", left)
	}
	if h, _ := m.History(1); h[0].Message != "forget source: conversation" {
		t.Fatalf("history %v", h)
	}
	if _, err := m.ForgetSource("conversation:c1", "owner", "again"); err == nil {
		t.Fatal("forgot nothing without saying so")
	}
	// Ana takes back hers, including the house fact she shared.
	if gone, _ := m.ForgetSource("conversation:c1", "ana", "x"); len(gone) != 2 {
		t.Fatalf("%+v", gone)
	}
	m.Restore(before[0].Hash)
	if all, _ := m.List(); len(all) != 6 {
		t.Fatal("forgetting could not be undone")
	}
}

func TestSourcesAreEachPersonsOwn(t *testing.T) {
	m, _ := Open(t.TempDir())
	m.AddFrom("A", "t", "x", Low, "", Origin{Kind: FromRoutine, Ref: "r1", Label: "Resumo"})
	m.AddFrom("B", "t", "x", Low, "ana", Origin{Kind: FromRoutine, Ref: "r9"})
	m.AddFrom("C", "t", "ana", High, "casa", Origin{Kind: FromTyped})
	all, _ := m.List()
	mine := SourcesOf(all, "owner")
	if len(mine) != 1 || mine[0].Key != "routine:r1" || mine[0].Origin.Label != "Resumo" || len(mine[0].Facts) != 1 {
		t.Fatalf("%+v", mine)
	}
	ana := SourcesOf(all, "ana")
	if len(ana) != 2 {
		t.Fatalf("%+v", ana)
	}
}

func TestFoldKeepsBothSources(t *testing.T) {
	m, _ := Open(t.TempDir())
	a, _ := m.AddFrom("Academia às terças", "t", "owner", High, "", Origin{Kind: FromTyped})
	b, _ := m.AddFrom("Vou à academia às terças", "t", "x", Low, "", Origin{Kind: FromConversation, Ref: "c1"})
	if err := m.Fold([]Merge{{Keep: a.ID, Drop: b.ID}}, "organize"); err != nil {
		t.Fatal(err)
	}
	all, _ := m.List()
	if len(all) != 1 || !all[0].Has(FromTyped) || !all[0].Has("conversation:c1") {
		t.Fatalf("%+v", all)
	}
}

func TestFactsFromBeforeSourcesLoad(t *testing.T) {
	dir := t.TempDir()
	old := `[{"id":"a1","text":"Old one","topic":"geral","source":"owner","trust":"high","created":"2025-01-01T00:00:00Z"},
	{"id":"a2","text":"Noted","topic":"geral","source":"exploration:e7","trust":"low","created":"2025-01-01T00:00:00Z"}]`
	os.WriteFile(filepath.Join(dir, factsFile), []byte(old), 0o600)
	m, _ := Open(dir)
	all, err := m.List()
	if err != nil || len(all) != 2 {
		t.Fatalf("%v %d", err, len(all))
	}
	byID := map[string]Fact{}
	for _, f := range all {
		byID[f.ID] = f
	}
	if k := keys(byID["a1"]); k[0] != FromUnknown {
		t.Fatalf("%v", k)
	}
	if k := keys(byID["a2"]); k[0] != "exploration:e7" {
		t.Fatalf("%v", k)
	}
	if gone, _ := m.ForgetSource(FromUnknown, "owner", "x"); len(gone) != 1 || gone[0].ID != "a1" {
		t.Fatalf("%+v", gone)
	}
}
