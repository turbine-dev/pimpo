package explore

import (
	"sort"
	"strings"
)

// guideSections splits the user guide at its "## " headings.
func guideSections(guide string) []string {
	var out []string
	for _, part := range strings.Split("\n"+guide, "\n## ")[1:] {
		out = append(out, "## "+strings.TrimSpace(part))
	}
	return out
}

// searchGuide returns the sections that share the most words with the
// query, or the list of headings when the query is empty.
func searchGuide(guide, query string) string {
	sections := guideSections(guide)
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		var heads []string
		for _, s := range sections {
			heads = append(heads, strings.SplitN(s, "\n", 2)[0])
		}
		return strings.Join(heads, "\n")
	}
	type hit struct {
		i, n int
	}
	var hits []hit
	for i, s := range sections {
		low := strings.ToLower(s)
		head, _, _ := strings.Cut(low, "\n")
		n := 0
		for _, w := range words {
			if len(w) > 2 && strings.Contains(low, w) {
				n++
				// A word in the section's heading says more about it.
				if strings.Contains(head, w) {
					n += 2
				}
			}
		}
		if n > 0 {
			hits = append(hits, hit{i, n})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].n > hits[b].n })
	var out []string
	for k, h := range hits {
		if k == 3 {
			break
		}
		out = append(out, sections[h.i])
	}
	if len(out) == 0 {
		return "Nothing in the guide matches; these are its sections:\n" + searchGuide(guide, "")
	}
	return strings.Join(out, "\n\n")
}
