package app

import (
	"context"
	"net/http"
	"strings"

	"github.com/turbine-dev/pimpo/internal/memory"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
)

// AttachMemory gives the app the owner's memory, stored under dir.
func (a *App) AttachMemory(dir string) error {
	m, err := memory.Open(dir)
	if err != nil {
		return err
	}
	a.Memory = m
	a.Explore.Memory = m
	a.Explore.Recall = func(ctx context.Context, q, person string) ([]memory.Fact, error) {
		res, _, err := a.SearchMeaning(ctx, q, person)
		out := make([]memory.Fact, 0, len(res))
		for _, f := range res {
			out = append(out, f.Fact)
		}
		return out, err
	}
	return nil
}

func (a *App) memoryRoutes() {
	s := a.Server
	s.Handle("GET /api/memory", a.getMemory)
	s.Handle("POST /api/memory", a.addFact)
	s.Handle("DELETE /api/memory/{id}", a.removeFact)
	s.Handle("POST /api/memory/{id}/confirm", a.confirmFact)
	s.Handle("POST /api/memory-versions/{hash}/restore", a.restoreMemory)
	s.Handle("GET /api/memory/sources", a.memorySources)
	s.Handle("POST /api/memory/sources/forget", a.forgetSource)
}

func (a *App) needMemory(w http.ResponseWriter) bool {
	if a.Memory == nil {
		server.WriteError(w, server.StatusError{Status: 503, Msg: "memory is not available"})
		return false
	}
	return true
}

func (a *App) getMemory(w http.ResponseWriter, r *http.Request) {
	if !a.needMemory(w) {
		return
	}
	all, err := a.Memory.List()
	if err != nil {
		server.WriteError(w, err)
		return
	}
	me := people.From(r.Context())
	facts := []memory.Fact{}
	for _, f := range all {
		if memory.Visible(f, me) {
			facts = append(facts, withSources(f, me))
		}
	}
	// The history is the owner's: versions about others' facts say only
	// whose they were, and are left out.
	hist := []memory.Version{}
	if me == people.OwnerID {
		all, _ := a.Memory.History(60)
		for _, v := range all {
			if !strings.Contains(v.Message, memory.ForPrefix) && len(hist) < 30 {
				hist = append(hist, v)
			}
		}
	}
	server.WriteJSON(w, 200, map[string]any{"facts": facts, "history": hist})
}

func (a *App) addFact(w http.ResponseWriter, r *http.Request) {
	if !a.needMemory(w) {
		return
	}
	var req struct {
		Text  string `json:"text"`
		Topic string `json:"topic"`
		// Shared, when true, keeps the fact for everyone in the house;
		// otherwise it is the person's own.
		Shared bool `json:"shared"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	ctx := r.Context()
	me := people.From(ctx)
	person := me
	if req.Shared {
		if err := a.mayShare(ctx, me); err != nil {
			server.WriteError(w, err)
			return
		}
		person = people.Household
	}
	f, err := a.Memory.AddFrom(req.Text, req.Topic, me, memory.High, person, memory.Origin{Kind: memory.FromTyped})
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	a.Events.Append(r.Context(), "memory.changed", actor(r.Context()), map[string]string{"added": f.ID})
	server.WriteJSON(w, 200, f)
}

// sharedLimit is how many house facts one member may keep: everyone's
// runs read them, so one person cannot fill the house's memory.
const sharedLimit = 20

// mayShare says whether a person may add a fact for the whole house. A
// guest keeps only their own; a member's are attributed to them, never
// read as the owner's word, and limited in number.
func (a *App) mayShare(ctx context.Context, me string) error {
	if me == people.OwnerID {
		return nil
	}
	if a.People.Role(ctx, me) != people.Member {
		return server.StatusError{Status: 403, Msg: "a guest keeps facts only for themselves"}
	}
	all, err := a.Memory.List()
	if err != nil {
		return err
	}
	n := 0
	for _, f := range all {
		if memory.SharedBy(f) == me {
			n++
		}
	}
	if n >= sharedLimit {
		return server.StatusError{Status: 409, Msg: "you already share many facts with the house; remove one first"}
	}
	return nil
}

func (a *App) removeFact(w http.ResponseWriter, r *http.Request) {
	if !a.needMemory(w) {
		return
	}
	// The author of a house fact may take it back.
	if f, ok := a.Memory.Get(r.PathValue("id")); !ok || !memory.Authored(f, people.From(r.Context())) {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such fact"})
		return
	}
	if err := a.Memory.Remove(r.PathValue("id")); err != nil {
		server.WriteError(w, server.StatusError{Status: 404, Msg: err.Error()})
		return
	}
	a.Events.Append(r.Context(), "memory.changed", actor(r.Context()), map[string]string{"removed": r.PathValue("id")})
	server.WriteJSON(w, 200, map[string]string{"state": "removed"})
}

func (a *App) confirmFact(w http.ResponseWriter, r *http.Request) {
	if !a.needMemory(w) {
		return
	}
	if f, ok := a.Memory.Get(r.PathValue("id")); !ok || !memory.Mine(f, people.From(r.Context())) {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such fact"})
		return
	}
	if err := a.Memory.Confirm(r.PathValue("id")); err != nil {
		server.WriteError(w, server.StatusError{Status: 404, Msg: err.Error()})
		return
	}
	a.Events.Append(r.Context(), "memory.changed", actor(r.Context()), map[string]string{"confirmed": r.PathValue("id")})
	server.WriteJSON(w, 200, map[string]string{"state": "confirmed"})
}

func (a *App) restoreMemory(w http.ResponseWriter, r *http.Request) {
	if !a.needMemory(w) {
		return
	}
	// Only the restorer's facts go back; everyone else's stay as they are.
	if err := a.Memory.RestoreFor(r.PathValue("hash"), people.From(r.Context())); err != nil {
		server.WriteError(w, server.StatusError{Status: 404, Msg: err.Error()})
		return
	}
	a.Events.Append(r.Context(), "memory.changed", "human:owner", map[string]string{"restored": r.PathValue("hash")})
	server.WriteJSON(w, 200, map[string]string{"state": "restored"})
}

// withSources gives every fact its origins; where a house fact someone
// else shared came from is theirs, so only its kind shows.
func withSources(f memory.Fact, me string) memory.Fact {
	from := f.From()
	if !memory.AuthoredBy(f, me) {
		kinds := make([]memory.Origin, 0, len(from))
		for _, o := range from {
			kinds = append(kinds, memory.Origin{Kind: o.Kind})
		}
		from = kinds
	}
	f.Origins = from
	return f
}

// memorySources groups the person's own facts by where they came from.
func (a *App) memorySources(w http.ResponseWriter, r *http.Request) {
	if !a.needMemory(w) {
		return
	}
	all, err := a.Memory.List()
	if err != nil {
		server.WriteError(w, err)
		return
	}
	me := people.From(r.Context())
	sources := memory.SourcesOf(all, me)
	for i := range sources {
		for j, f := range sources[i].Facts {
			sources[i].Facts[j] = withSources(f, me)
		}
	}
	if sources == nil {
		sources = []memory.Source{}
	}
	server.WriteJSON(w, 200, map[string]any{"sources": sources})
}

// forgetSource removes every fact of the person's that came from one
// source, as one change in the memory's history.
func (a *App) forgetSource(w http.ResponseWriter, r *http.Request) {
	if !a.needMemory(w) {
		return
	}
	var req struct {
		Key string `json:"key"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	ctx := r.Context()
	me := people.From(ctx)
	kind, _, _ := strings.Cut(req.Key, ":")
	msg := "forget source: " + kind
	if me != people.OwnerID {
		msg += " (" + memory.ForPrefix + me + ")"
	}
	gone, err := a.Memory.ForgetSource(req.Key, me, msg)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 404, Msg: err.Error()})
		return
	}
	ids := make([]string, 0, len(gone))
	for _, f := range gone {
		ids = append(ids, f.ID)
	}
	a.Events.Append(ctx, "memory.changed", actor(ctx), map[string]any{"forgot_source": kind, "removed": ids})
	server.WriteJSON(w, 200, map[string]any{"removed": gone})
}
