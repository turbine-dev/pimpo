package company

import (
	"testing"

	"github.com/turbine-dev/pimpo/internal/capability"
)

func TestTriggersPutADecisionAtItsLevel(t *testing.T) {
	l := SuggestedLevels()
	for _, c := range []struct {
		name string
		m    Matter
		want int
	}{
		{"nothing", Matter{Text: "which font"}, 1},
		{"irreversible", Matter{Capability: "gmail.send", Risk: capability.Irreversible}, 2},
		{"$50", Matter{AmountUSD: 50}, 3},
		{"architecture", Matter{Kind: "Architecture"}, 3},
		{"$150", Matter{AmountUSD: 150}, 4},
		{"a contract", Matter{Kind: "contract"}, 4},
		{"public", Matter{Public: true}, 4},
		{"the highest wins", Matter{Capability: "gmail.send", Risk: capability.Irreversible, AmountUSD: 500}, 4},
	} {
		if got, why := l.Classify(c.m); got != c.want {
			t.Errorf("%s = %d (%s), want %d", c.name, got, why, c.want)
		}
	}
	l.List[3].When.Words = []string{"preço"}
	if got, why := l.Classify(Matter{Text: "Mudar o PRECO da camiseta?"}); got != 4 || why != "mentions preço" {
		t.Fatalf("a word = %d %q", got, why)
	}
	if n, _ := (Levels{}).Classify(Matter{AmountUSD: 1e6}); n != 0 {
		t.Fatal("a company without levels gave one")
	}
}

func TestLevelsEndWithTheCEO(t *testing.T) {
	for _, bad := range []Levels{
		{List: []Level{{Level: 1, Name: "Only", Decides: LevelBoss}}},
		{List: []Level{{Level: 2, Name: "Two", Decides: LevelCEO}}},
		{List: []Level{{Level: 1, Name: "X", Decides: "oracle"}}},
		{List: []Level{{Level: 1, Name: "X", Decides: LevelCEO}}, Unsure: 0.2},
	} {
		if bad.check() == nil {
			t.Errorf("%+v passed", bad)
		}
	}
	if err := SuggestedLevels().check(); err != nil {
		t.Fatal(err)
	}
}

func TestWhoDecidesAtEachLevel(t *testing.T) {
	o := shop(t, newStore(t))
	for decides, want := range map[string]string{LevelSelf: "clara", LevelBoss: "bia", LevelHead: "bia", LevelCEO: CEO} {
		if m, ok := o.DeciderAt("clara", Level{Decides: decides}); !ok || m.ID != want {
			t.Errorf("%s: %s", decides, m.ID)
		}
	}
}
