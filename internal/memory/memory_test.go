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
