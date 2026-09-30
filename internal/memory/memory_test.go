package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFactsHistoryAndRestore(t *testing.T) {
	m, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	boss, _ := m.Add("Minha chefe é a Ana (ana@acme.com)", "trabalho", "owner", High)
	m.Add("Prefiro resumos curtos", "preferências", "owner", High)
	rumor, _ := m.Add("A reunião de sexta foi cancelada", "trabalho", "email:INBOX/9", Low)
	if got, _ := m.Instructions(); len(got) != 2 {
		t.Fatalf("instructions must exclude low-trust facts: %+v", got)
	}
	md, _ := os.ReadFile(filepath.Join(m.Dir, "trabalho.md"))
	if !strings.Contains(string(md), "Ana") || !strings.Contains(string(md), "não confirmado") {
		t.Fatalf("markdown:\n%s", md)
	}
	hist, _ := m.History(10)
	if len(hist) != 3 {
		t.Fatalf("history %d", len(hist))
	}
	m.Remove(boss.ID)
	if got, _ := m.Search("chefe"); len(got) != 0 {
		t.Fatal("removed fact still found")
	}
	if err := m.Restore(hist[0].Hash); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Search("chefe ana"); len(got) != 1 {
		t.Fatal("restore did not bring the fact back")
	}
	m.Confirm(rumor.ID)
	if got, _ := m.Instructions(); len(got) != 3 {
		t.Fatalf("confirmed fact not trusted: %d", len(got))
	}
}

func TestLowTrustNeverDowngradesAndReopens(t *testing.T) {
	dir := t.TempDir()
	m, _ := Open(dir)
	m.Add("Moro em São Paulo", "pessoal", "owner", High)
	f, _ := m.Add("moro em são paulo", "pessoal", "web:example.com", Low)
	if f.Trust != High {
		t.Fatal("a low-trust copy downgraded an owner fact")
	}
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := again.List(); len(got) != 1 {
		t.Fatalf("reopened %d facts", len(got))
	}
}

// Restoring a version brings back only the restorer's facts: a member's
// facts stay as they are now.
func TestRestoreForLeavesOthersFactsAlone(t *testing.T) {
	m, _ := Open(t.TempDir())
	m.Add("owner old", "geral", "owner", High)
	m.AddFor("ana old", "geral", "ana", High, "ana")
	v, _ := m.History(1)
	m.Add("owner new", "geral", "owner", High)
	anaNew, _ := m.AddFor("ana new", "geral", "ana", High, "ana")
	if err := m.RestoreFor(v[0].Hash, "owner"); err != nil {
		t.Fatal(err)
	}
	facts, _ := m.List()
	var texts []string
	for _, f := range facts {
		texts = append(texts, f.Text)
	}
	got := strings.Join(texts, ",")
	if strings.Contains(got, "owner new") || !strings.Contains(got, "owner old") || !strings.Contains(got, "ana new") || !strings.Contains(got, "ana old") {
		t.Fatalf("after restore: %s", got)
	}
	if _, ok := m.Get(anaNew.ID); !ok {
		t.Fatal("ana's newer fact was lost")
	}
}

// A member's house fact is theirs to take back, and never the owner's word.
func TestSharedByAMemberIsNotTheOwnersWord(t *testing.T) {
	m, _ := Open(t.TempDir())
	f, _ := m.AddFor("wifi is on the fridge", "casa", "ana", High, "casa")
	o, _ := m.AddFor("dinner at 8", "casa", "owner", High, "casa")
	if SharedBy(f) != "ana" || OwnersWord(f) || !Authored(f, "ana") || Authored(f, "bia") {
		t.Fatalf("member's house fact: %+v", f)
	}
	if SharedBy(o) != "" || !OwnersWord(o) || Authored(o, "ana") {
		t.Fatalf("owner's house fact: %+v", o)
	}
}
