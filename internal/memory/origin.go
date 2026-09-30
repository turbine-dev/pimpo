package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Origin is where a fact came from: a conversation, an exploration, a
// routine, an email, an import, the person typing it, or a preference
// learned from their choices. A fact keeps every origin it was found in,
// so a person can take back everything one source gave.
type Origin struct {
	Kind string `json:"kind"`
	// Ref is the source's id: a chat, exploration, routine or job id, an
	// email's message id, the tool an import came from.
	Ref string `json:"ref,omitempty"`
	// Turn is the part of the source: a chat turn or a routine run.
	Turn string `json:"turn,omitempty"`
	// Sender is who sent the email a fact was read in.
	Sender string `json:"sender,omitempty"`
	// Label is a short human name: a chat's title, a routine's name, an
	// email's subject.
	Label string `json:"label,omitempty"`
}

const (
	FromConversation = "conversation"
	FromExploration  = "exploration"
	FromRoutine      = "routine"
	FromJob          = "job"
	FromEmail        = "email"
	FromImport       = "import"
	FromTyped        = "typed"
	FromLearned      = "learned"
	FromUnknown      = "unknown"
)

type originKey struct{}

// WithOrigin marks a request with where facts noted while answering it
// come from, such as the email that asked.
func WithOrigin(ctx context.Context, o Origin) context.Context {
	return context.WithValue(ctx, originKey{}, o)
}

// OriginOf is the origin a request was marked with.
func OriginOf(ctx context.Context) (Origin, bool) {
	o, ok := ctx.Value(originKey{}).(Origin)
	return o, ok
}

// Key names the source an origin belongs to: one conversation, one
// routine, every email from one sender, one import tool.
func (o Origin) Key() string {
	switch o.Kind {
	case FromEmail:
		if o.Sender != "" {
			return FromEmail + ":" + strings.ToLower(o.Sender)
		}
	case FromTyped, FromLearned, FromUnknown:
		return o.Kind
	}
	return o.Kind + ":" + o.Ref
}

// clean keeps an origin to one short line per field: labels come from
// chat titles and email subjects and end up in the Markdown files.
func (o Origin) clean() Origin {
	one := func(s string, n int) string {
		s = strings.Join(strings.Fields(s), " ")
		if r := []rune(s); len(r) > n {
			s = string(r[:n-1]) + "…"
		}
		return s
	}
	o.Kind, o.Ref, o.Turn = one(o.Kind, 20), one(o.Ref, 200), one(o.Turn, 100)
	o.Sender, o.Label = one(o.Sender, 200), one(o.Label, 80)
	if o.Kind == "" {
		o.Kind = FromUnknown
	}
	return o
}

// legacy is the origin of a fact saved before facts kept origins, read
// from its source where that says it plainly.
func legacy(source string) Origin {
	switch {
	case strings.HasPrefix(source, "exploration:"):
		return Origin{Kind: FromExploration, Ref: strings.TrimPrefix(source, "exploration:")}
	case strings.HasPrefix(source, "import:"):
		from := strings.TrimPrefix(source, "import:")
		return Origin{Kind: FromImport, Ref: from, Label: from}
	case strings.HasPrefix(source, "aprendido:"):
		return Origin{Kind: FromLearned}
	}
	return Origin{Kind: FromUnknown}
}

// From is where a fact came from; an old fact without origins has one
// read from its source, or "unknown".
func (f Fact) From() []Origin {
	if len(f.Origins) > 0 {
		return f.Origins
	}
	return []Origin{legacy(f.Source)}
}

// Has says whether the fact came, at least in part, from the source.
func (f Fact) Has(key string) bool {
	for _, o := range f.From() {
		if o.Key() == key {
			return true
		}
	}
	return false
}

// withOrigins adds origins a fact did not have yet.
func withOrigins(have []Origin, more ...Origin) []Origin {
	out := append([]Origin{}, have...)
	for _, o := range more {
		dup := false
		for _, h := range out {
			dup = dup || h == o
		}
		if !dup {
			out = append(out, o)
		}
	}
	return out
}

// AuthoredBy is a fact the person put in memory and so may take back by
// its source: their own, or a house fact they shared. Stricter than
// Authored: the owner does not take back a member's house fact this way.
func AuthoredBy(f Fact, person string) bool {
	if f.Person == "casa" {
		return SharedBy(f) == owned(person)
	}
	return f.Person == owned(person)
}

// Source is one source of a person's facts, with the facts it gave.
type Source struct {
	Key    string   `json:"key"`
	Origin Origin   `json:"origin"`
	Facts  []Fact   `json:"facts"`
	Topics []string `json:"topics"`
}

// SourcesOf groups the person's own facts by where they came from; a fact
// from two sources is in both.
func SourcesOf(facts []Fact, person string) []Source {
	var out []Source
	at := map[string]int{}
	for _, f := range facts {
		if !AuthoredBy(f, person) {
			continue
		}
		for _, o := range f.From() {
			k := o.Key()
			i, ok := at[k]
			if !ok {
				i = len(out)
				at[k] = i
				out = append(out, Source{Key: k, Origin: o, Facts: []Fact{}, Topics: []string{}})
			}
			s := &out[i]
			if s.Origin.Label == "" && o.Label != "" {
				s.Origin.Label = o.Label
			}
			s.Facts = append(s.Facts, f)
			if !contains(s.Topics, f.Topic) {
				s.Topics = append(s.Topics, f.Topic)
			}
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ForgetSource removes every fact the person authored that came, even in
// part, from the source, in one versioned change History can undo. It
// returns what went.
func (m *Memory) ForgetSource(key, person, message string) ([]Fact, error) {
	if key == "" {
		return nil, errors.New("which source?")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	facts, err := m.load()
	if err != nil {
		return nil, err
	}
	kept := []Fact{}
	gone := []Fact{}
	for _, f := range facts {
		if AuthoredBy(f, person) && f.Has(key) {
			gone = append(gone, f)
		} else {
			kept = append(kept, f)
		}
	}
	if len(gone) == 0 {
		return nil, fmt.Errorf("no facts from that source")
	}
	return gone, m.save(kept, message)
}

// Merge is one duplicate folded into the fact kept.
type Merge struct{ Keep, Drop string }

// Fold removes duplicates in one versioned change; each kept fact takes
// the origins of the ones folded into it, so forgetting either source
// still forgets it.
func (m *Memory) Fold(merges []Merge, message string) error {
	if len(merges) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	facts, err := m.load()
	if err != nil {
		return err
	}
	byID := map[string]int{}
	for i, f := range facts {
		byID[f.ID] = i
	}
	gone := map[string]bool{}
	for _, mg := range merges {
		k, okK := byID[mg.Keep]
		d, okD := byID[mg.Drop]
		if !okK || !okD || gone[mg.Keep] {
			continue
		}
		facts[k].Origins = withOrigins(facts[k].From(), facts[d].From()...)
		gone[mg.Drop] = true
	}
	kept := []Fact{}
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
