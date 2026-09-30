package store

import (
	"context"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// ChatHit is one place a search found in a person's conversations: the
// message asked or the answer given in one turn.
type ChatHit struct {
	Chat    string `json:"chat"`
	Title   string `json:"title"`
	Turn    string `json:"turn"`
	Snippet string `json:"snippet"`
	At      string `json:"at"`
}

// SearchChats finds q in the titles, messages and answers of one person's
// conversations, newest first. Words may come in any order; text in
// double quotes must appear as written. Case and accents are ignored, so
// "reuniao" finds "Reunião".
func (s *Store) SearchChats(ctx context.Context, person, q string, limit int) ([]ChatHit, error) {
	terms := searchTerms(q)
	out := []ChatHit{}
	if len(terms) == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.title, t.exploration, e.request, COALESCE(e.summary, ''), e.created_at
		FROM chats c JOIN chat_turns t ON t.chat = c.id JOIN explorations e ON e.id = t.exploration
		WHERE c.person = ? ORDER BY e.created_at DESC`, person)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() && len(out) < limit {
		var h ChatHit
		var request, summary string
		if err := rows.Scan(&h.Chat, &h.Title, &h.Turn, &request, &summary, &h.At); err != nil {
			return nil, err
		}
		if !hasAll(fold(h.Title+"\n"+request+"\n"+summary), terms) {
			continue
		}
		h.Snippet = snippet(request, summary, terms)
		out = append(out, h)
	}
	return out, rows.Err()
}

// searchTerms splits a query into folded words and quoted phrases.
func searchTerms(q string) []string {
	var terms []string
	for i, part := range strings.Split(q, `"`) {
		if i%2 == 1 {
			if p := strings.Join(strings.Fields(fold(part)), " "); p != "" {
				terms = append(terms, p)
			}
			continue
		}
		terms = append(terms, strings.Fields(fold(part))...)
	}
	return terms
}

func hasAll(text string, terms []string) bool {
	text = strings.Join(strings.Fields(text), " ")
	for _, t := range terms {
		if !strings.Contains(text, t) {
			return false
		}
	}
	return true
}

// fold lowercases and drops accents.
func fold(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, s)
	if err != nil {
		out = s
	}
	return strings.ToLower(out)
}

// snippet is a short piece around the first term found, from the message
// or else the answer.
func snippet(request, summary string, terms []string) string {
	for _, text := range []string{request, summary} {
		line := strings.Join(strings.Fields(text), " ")
		f := []rune(fold(line))
		r := []rune(line)
		if len(f) != len(r) {
			// Folding changed the length; show the start instead.
			if hasAll(fold(line), terms[:1]) {
				return cut(r, 0)
			}
			continue
		}
		if i := strings.Index(string(f), terms[0]); i >= 0 {
			return cut(r, len([]rune(string(f)[:i])))
		}
	}
	return cut([]rune(strings.Join(strings.Fields(request), " ")), 0)
}

func cut(r []rune, at int) string {
	start := max(0, at-40)
	end := min(len(r), start+160)
	s := string(r[start:end])
	if start > 0 {
		s = "…" + s
	}
	if end < len(r) {
		s += "…"
	}
	return s
}
