package company

import (
	"strings"
	"testing"
)

func TestEachMemberReadsItsOwnMemories(t *testing.T) {
	notes := []Note{
		{ID: "old", Title: "From before scopes"},
		{ID: "co", Scope: MemoryCompany},
		{ID: "bia", Scope: MemoryMember, Of: "bia"},
		{ID: "rui", Scope: MemoryMember, Of: "rui"},
		{ID: "parent", Scope: MemoryTask, Of: "t_1"},
		{ID: "other", Scope: MemoryTask, Of: "t_9"},
		{ID: "waiting", Scope: MemoryCompany, Pending: true},
	}
	var got []string
	for _, n := range Visible(notes, "bia", []string{"t_2", "t_1"}) {
		got = append(got, n.ID)
	}
	if want := "old co bia parent"; strings.Join(got, " ") != want {
		t.Fatalf("Bia reads %q, want %q", strings.Join(got, " "), want)
	}
}

func TestMemoryPolicyDefaults(t *testing.T) {
	var p MemoryPolicy
	if sp := p.For(MemoryMember); sp.Write != WriteFree || sp.Auto {
		t.Fatalf("member = %+v", sp)
	}
	if sp := p.For(MemoryCompany); sp.Write != WriteDecide || sp.Decider.Kind != DecidePerson {
		t.Fatalf("company = %+v", sp)
	}
	p.Task = ScopePolicy{Write: WriteDecide, Decider: Decider{Kind: DecideJev}}
	if sp := p.For(MemoryTask); sp.Decider.Kind != DecideJev {
		t.Fatalf("task = %+v", sp)
	}
	if err := (MemoryPolicy{Member: ScopePolicy{Write: "sometimes"}}).check(Org{}); err == nil {
		t.Fatal("an unknown way of writing")
	}
}
