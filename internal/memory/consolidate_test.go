package memory

import (
	"testing"
	"time"
)

func TestSimilarAndKeeper(t *testing.T) {
	now := time.Now()
	facts := []Fact{
		{ID: "1", Text: "Vou à academia às terças", Trust: Low, Created: now.Add(-48 * time.Hour)},
		{ID: "2", Text: "Academia às terças", Trust: High, Created: now.Add(-72 * time.Hour)},
		{ID: "3", Text: "Minha irmã se chama Ana", Trust: High, Created: now},
		{ID: "4", Text: "Academia às terças", Trust: High, Person: "ana", Created: now},
		{ID: "5", Text: "A irmã se chama Ana Paula", Trust: High, Created: now.Add(time.Hour)},
	}
	pairs := Similar(facts, 10)
	got := map[string]bool{}
	for _, p := range pairs {
		got[p.A.ID+p.B.ID] = true
	}
	if !got["12"] || !got["35"] || got["24"] || got["14"] || len(pairs) != 2 {
		t.Fatalf("%v", got)
	}
	if keep, drop := Keeper(pairs[0]); keep.ID != "2" || drop.ID != "1" {
		t.Fatalf("a low-trust copy won: kept %s", keep.ID)
	}
	if keep, _ := Keeper(Pair{facts[2], facts[4]}); keep.ID != "5" {
		t.Fatalf("the newer wording should stay, kept %s", keep.ID)
	}
}

func TestDropIsOneUndoableChange(t *testing.T) {
	m, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := m.Add("Academia às terças", "rotina", "owner", High)
	b, _ := m.Add("Vou à academia às terças", "rotina", "email", Low)
	before, _ := m.History(1)
	if err := m.Drop([]string{b.ID}, "consolidate: 1 duplicate"); err != nil {
		t.Fatal(err)
	}
	left, _ := m.List()
	if len(left) != 1 || left[0].ID != a.ID {
		t.Fatalf("%+v", left)
	}
	if err := m.Restore(before[0].Hash); err != nil {
		t.Fatal(err)
	}
	if all, _ := m.List(); len(all) != 2 {
		t.Fatalf("undo: %+v", all)
	}
}
