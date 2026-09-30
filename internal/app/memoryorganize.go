package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/judge"
	"github.com/turbine-dev/pimpo/internal/memory"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
)

// Memory keeps itself tidy: every night near-duplicate facts are checked
// by the judge and merged in one change the owner can undo, and the owner
// can search it by meaning.

type merge struct {
	Kept    string `json:"kept"`
	Dropped string `json:"dropped"`
}

type organized struct {
	At      time.Time `json:"at,omitzero"`
	Checked int       `json:"checked"`
	Merged  []merge   `json:"merged"`
	Error   string    `json:"error,omitempty"`
}

const organizedKey = "memory.organized"

// sameFact and its threshold were checked on labeled pairs with Jev
// (tools/jev/calibrate_memory.py): rewordings scored 0.70 and up,
// pairs differing in a detail 0.29 and below.
const sameFact = "Do `item.a` and `item.b` record the same fact, possibly in different words? " +
	"Answer no if they differ in a date, day, place, name, number or other detail, or if one contradicts or updates the other."

const sameFactThreshold = 0.6

var organizeMu sync.Mutex

// organizedKeyFor keeps each person's last result apart: what was merged
// quotes their facts.
func organizedKeyFor(person string) string {
	if person = people.Norm(person); person != people.OwnerID {
		return organizedKey + "." + person
	}
	return organizedKey
}

// organizeMemory tidies one person's facts: their own and, for the owner,
// the house's. Nobody's facts are compared with anyone else's.
func (a *App) organizeMemory(ctx context.Context, person string) (organized, error) {
	organizeMu.Lock()
	defer organizeMu.Unlock()
	person = people.Norm(person)
	ctx = people.With(ctx, person)
	res := organized{At: time.Now(), Merged: []merge{}}
	if a.Memory == nil {
		return res, errors.New("memory is not available")
	}
	all, err := a.Memory.List()
	if err != nil {
		return res, err
	}
	var facts []memory.Fact
	for _, f := range all {
		if memory.Mine(f, person) {
			facts = append(facts, f)
		}
	}
	pairs := memory.Similar(facts, 40)
	res.Checked = len(pairs)
	gone := map[string]bool{}
	var drop []memory.Merge
	for _, p := range pairs {
		if gone[p.A.ID] || gone[p.B.ID] {
			continue
		}
		ans, err := a.judge(ctx, sameFact, map[string]string{"a": p.A.Text, "b": p.B.Text})
		if err != nil {
			res.Error = err.Error()
			break
		}
		if ans.P < sameFactThreshold {
			continue
		}
		keep, lose := memory.Keeper(p)
		gone[lose.ID] = true
		drop = append(drop, memory.Merge{Keep: keep.ID, Drop: lose.ID})
		res.Merged = append(res.Merged, merge{Kept: keep.Text, Dropped: lose.Text})
	}
	if len(drop) > 0 {
		msg := fmt.Sprintf("organize: merged %d duplicate facts", len(drop))
		if person != people.OwnerID {
			msg += " (" + memory.ForPrefix + person + ")"
		}
		// The kept fact keeps both sources.
		if err := a.Memory.Fold(drop, msg); err != nil {
			return res, err
		}
	}
	b, _ := json.Marshal(res)
	a.Events.Put(ctx, organizedKeyFor(person), string(b))
	a.Events.Append(ctx, "memory.organized", "system", map[string]any{"checked": res.Checked, "merged": len(res.Merged), "person": person})
	return res, nil
}

// organizeEveryone tidies each person's memory on its own.
func (a *App) organizeEveryone(ctx context.Context) {
	list, _ := a.People.List(ctx)
	for _, p := range list {
		a.organizeMemory(ctx, p.ID)
	}
}

// organizeLoop tidies memory once a night, after 3 in the owner's zone.
func (a *App) organizeLoop(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
		now := time.Now().In(loadZone(a.Settings(ctx).Zone))
		var last organized
		if raw, _ := a.Events.Get(ctx, organizedKey); raw != "" {
			json.Unmarshal([]byte(raw), &last)
		}
		if a.lab(ctx, "memory_organize") && now.Hour() >= 3 && last.At.In(now.Location()).Format("2006-01-02") != now.Format("2006-01-02") {
			a.organizeEveryone(ctx)
		}
	}
}

func (a *App) organizeRoutes() {
	a.Server.Handle("POST /api/memory/organize", func(w http.ResponseWriter, r *http.Request) {
		res, err := a.organizeMemory(r.Context(), people.From(r.Context()))
		if err != nil {
			server.WriteError(w, err)
			return
		}
		server.WriteJSON(w, 200, res)
	})
	a.Server.Handle("GET /api/memory/organized", func(w http.ResponseWriter, r *http.Request) {
		last := organized{Merged: []merge{}}
		if raw, _ := a.Events.Get(r.Context(), organizedKeyFor(people.From(r.Context()))); raw != "" {
			json.Unmarshal([]byte(raw), &last)
		}
		if last.Merged == nil {
			last.Merged = []merge{}
		}
		server.WriteJSON(w, 200, last)
	})
	a.Server.Handle("GET /api/memory/search", a.searchMemory)
}

type chooser interface {
	Choose(ctx context.Context, instructions string, state any, options map[string]string) (map[string]float64, error)
}

// meaningJudge is Jev when it is set up: it can pick among many facts in
// one question. Tests replace it.
var meaningJudge = func(a *App) (chooser, bool) {
	if _, err := a.Vault.Get(context.Background(), "typesafe.key"); err != nil {
		return nil, false
	}
	return judge.Jev{Key: func(ctx context.Context) (string, error) { return a.secret(ctx, "typesafe.key") }}, true
}

type found struct {
	memory.Fact
	Score float64 `json:"score"`
	By    string  `json:"by"` // words or meaning
}

// SearchMeaning returns the facts person may read that relate to query:
// the ones containing its words, then the ones Jev finds by meaning.
func (a *App) SearchMeaning(ctx context.Context, query, person string) ([]found, bool, error) {
	out := []found{}
	seen := map[string]bool{}
	lexical, err := a.Memory.SearchFor(query, person)
	if err != nil {
		return nil, false, err
	}
	for _, f := range lexical {
		out = append(out, found{f, 1, "words"})
		seen[f.ID] = true
	}
	j, ok := meaningJudge(a)
	if !ok || !a.lab(ctx, "meaning_search") {
		return out, false, nil
	}
	all, _ := a.Memory.SearchFor("", person)
	sort.Slice(all, func(i, k int) bool { return all[i].Created.After(all[k].Created) })
	options := map[string]string{"none": "None of these facts relates to the query"}
	byID := map[string]memory.Fact{}
	for _, f := range all {
		if len(byID) == 80 {
			break
		}
		if !seen[f.ID] {
			options[f.ID] = f.Text
			byID[f.ID] = f
		}
	}
	if len(byID) == 0 {
		return out, true, nil
	}
	probs, err := j.Choose(ctx, "Which of these facts about the person best answers or relates to `query`?", map[string]string{"query": query}, options)
	if err != nil {
		return out, true, nil
	}
	var extra []found
	for id, p := range probs {
		if f, ok := byID[id]; ok && p >= 0.15 {
			extra = append(extra, found{f, p, "meaning"})
		}
	}
	sort.Slice(extra, func(i, k int) bool { return extra[i].Score > extra[k].Score })
	return append(out, extra...), true, nil
}

func (a *App) searchMemory(w http.ResponseWriter, r *http.Request) {
	if !a.needMemory(w) {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		server.WriteJSON(w, 200, map[string]any{"facts": []found{}, "meaning": false})
		return
	}
	res, meaning, err := a.SearchMeaning(r.Context(), q, people.From(r.Context()))
	if err != nil {
		server.WriteError(w, err)
		return
	}
	for i := range res {
		res[i].Fact = withSources(res[i].Fact, people.From(r.Context()))
	}
	server.WriteJSON(w, 200, map[string]any{"facts": res, "meaning": meaning})
}
