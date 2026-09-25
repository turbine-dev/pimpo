package memory

import (
	"fmt"
	"strings"
	"unicode"
)

// Pair is two facts that may say the same thing.
type Pair struct{ A, B Fact }

var stop = map[string]bool{}

func init() {
	for _, w := range strings.Fields("a o as os de da do das dos e em no na nos nas um uma que para por com se ao the of and in on to is are for with my at it") {
		stop[w] = true
	}
}

func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(w) > 1 && !stop[w] {
			out[w] = true
		}
	}
	return out
}

// Similar finds pairs of facts about the same person whose words mostly
// overlap: the candidates a judge then confirms or rejects.
func Similar(facts []Fact, limit int) []Pair {
	sets := make([]map[string]bool, len(facts))
	for i, f := range facts {
		sets[i] = words(f.Text)
	}
	var out []Pair
	for i := range facts {
		for j := i + 1; j < len(facts); j++ {
			if facts[i].Person != facts[j].Person || len(sets[i]) == 0 || len(sets[j]) == 0 {
				continue
			}
			both, small := 0, min(len(sets[i]), len(sets[j]))
			for w := range sets[i] {
				if sets[j][w] {
					both++
				}
			}
			union := len(sets[i]) + len(sets[j]) - both
			if float64(both)/float64(union) >= 0.5 || (small >= 2 && both == small) {
				out = append(out, Pair{facts[i], facts[j]})
				if len(out) == limit {
					return out
				}
			}
		}
	}
	return out
}

// Keeper picks which of two facts saying the same thing to keep: never a
// low-trust one over what the owner confirmed, else the newer wording.
func Keeper(p Pair) (keep, drop Fact) {
	switch {
	case p.A.Trust == High && p.B.Trust != High:
		return p.A, p.B
	case p.B.Trust == High && p.A.Trust != High:
		return p.B, p.A
	case p.B.Created.After(p.A.Created):
		return p.B, p.A
	}
	return p.A, p.B
}

// Drop removes duplicates in one versioned change, which History can undo.
func (m *Memory) Drop(ids []string, message string) error {
	if len(ids) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	facts, err := m.load()
	if err != nil {
		return err
	}
	gone := map[string]bool{}
	for _, id := range ids {
		gone[id] = true
	}
	kept := facts[:0]
	for _, f := range facts {
		if !gone[f.ID] {
			kept = append(kept, f)
		}
	}
	if len(kept) == len(facts) {
		return fmt.Errorf("none of those facts exist")
	}
	return m.save(kept, message)
}
