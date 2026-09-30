package models

import (
	"context"
	"sort"
	"strings"
	"time"
)

// Discovery keeps each provider's list for a day (a minute for servers on
// this computer, whose models change as the owner pulls them); fresh asks
// again at once.
const (
	listFor  = 24 * time.Hour
	localFor = time.Minute
	// NewFor is how long a model the provider just started offering is
	// marked new.
	NewFor = 14 * 24 * time.Hour
)

// Discover returns the provider's current list, from the cache unless it
// is older than a day or fresh is set.
func (c *Client) Discover(ctx context.Context, e Endpoint, fresh bool) ([]Model, error) {
	ttl := listFor
	if e.Provider == "ollama" || e.Provider == "lmstudio" {
		ttl = localFor
	}
	key := cacheKey(e.Provider+"|"+e.Base, map[string]string{"key": e.Key})
	c.mu.Lock()
	hit, ok := c.lists[key]
	c.mu.Unlock()
	if ok && !fresh && time.Since(hit.at) < ttl {
		return append([]Model(nil), hit.models...), nil
	}
	if fresh && e.Provider == "openrouter" {
		c.mu.Lock()
		c.fetched = time.Time{}
		c.mu.Unlock()
	}
	list, err := c.List(ctx, e)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.lists == nil {
		c.lists = map[string]listed{}
	}
	c.lists[key] = listed{list, time.Now()}
	c.mu.Unlock()
	return append([]Model(nil), list...), nil
}

// Owned is one of the owner's models with the price the owner set.
type Owned struct {
	ID       string
	PriceIn  float64
	PriceOut float64
}

// Merge joins a provider's live list with the owner's own models:
//   - the owner's models keep the owner's price, which wins over any found;
//   - models the provider did not list before are marked new (unpriced
//     unless the provider's catalog gave a price, as OpenRouter's does);
//   - the owner's models the provider no longer lists are kept, marked
//     retired, and returned as retired.
//
// seen is when each id was first listed; nil means this is the first
// look, so nothing is new yet. The updated record is returned.
func Merge(live []Model, mine []Owned, seen map[string]time.Time, now time.Time) (out []Model, retired []string, seenOut map[string]time.Time) {
	first := seen == nil
	seenOut = map[string]time.Time{}
	for id, t := range seen {
		seenOut[id] = t
	}
	owned := map[string]Owned{}
	for _, o := range mine {
		owned[o.ID] = o
	}
	listed := map[string]bool{}
	for _, m := range live {
		listed[m.ID] = true
		at, known := seenOut[m.ID]
		switch {
		case first:
			seenOut[m.ID] = time.Time{}
		case !known:
			seenOut[m.ID], at = now, now
		}
		m.New = !first && !at.IsZero() && now.Sub(at) < NewFor
		if o, ok := owned[m.ID]; ok {
			m.Mine, m.Priced, m.PriceIn, m.PriceOut = true, true, o.PriceIn, o.PriceOut
			m.Free = o.PriceIn == 0 && o.PriceOut == 0
		}
		out = append(out, m)
	}
	for _, o := range mine {
		if listed[o.ID] {
			continue
		}
		retired = append(retired, o.ID)
		out = append(out, Model{ID: o.ID, Name: o.ID, PriceIn: o.PriceIn, PriceOut: o.PriceOut, Priced: true, Mine: true, Retired: true})
	}
	sort.Strings(retired)
	// Retired first, then new, then the rest by name, so what needs a
	// decision is on top.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Retired != out[j].Retired {
			return out[i].Retired
		}
		if out[i].New != out[j].New {
			return out[i].New
		}
		return strings.ToLower(out[i].ID) < strings.ToLower(out[j].ID)
	})
	return out, retired, seenOut
}
