package app

import (
	"net/http"

	"github.com/denerFernandes/vigia/internal/memory"
	"github.com/denerFernandes/vigia/internal/people"
	"github.com/denerFernandes/vigia/internal/server"
)

// AttachMemory gives the app the owner's memory, stored under dir.
func (a *App) AttachMemory(dir string) error {
	m, err := memory.Open(dir)
	if err != nil {
		return err
	}
	a.Memory = m
	a.Explore.Memory = m
	return nil
}

func (a *App) memoryRoutes() {
	s := a.Server
	s.Handle("GET /api/memory", a.getMemory)
	s.Handle("POST /api/memory", a.addFact)
	s.Handle("DELETE /api/memory/{id}", a.removeFact)
	s.Handle("POST /api/memory/{id}/confirm", a.confirmFact)
	s.Handle("POST /api/memory-versions/{hash}/restore", a.restoreMemory)
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
	facts, err := a.Memory.List()
	if err != nil {
		server.WriteError(w, err)
		return
	}
	hist, _ := a.Memory.History(30)
	server.WriteJSON(w, 200, map[string]any{"facts": facts, "history": hist})
}

func (a *App) addFact(w http.ResponseWriter, r *http.Request) {
	if !a.needMemory(w) {
		return
	}
	var req struct {
		Text  string `json:"text"`
		Topic string `json:"topic"`
		// Person keeps the fact for someone in the house, or "casa" for all.
		Person string `json:"person"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	if req.Person != "" && req.Person != people.OwnerID && req.Person != people.Household {
		if _, err := a.People.Get(r.Context(), req.Person); err != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "no such person"})
			return
		}
	}
	f, err := a.Memory.AddFor(req.Text, req.Topic, "owner", memory.High, req.Person)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	a.Events.Append(r.Context(), "memory.changed", "human:owner", map[string]string{"added": f.ID})
	server.WriteJSON(w, 200, f)
}

func (a *App) removeFact(w http.ResponseWriter, r *http.Request) {
	if !a.needMemory(w) {
		return
	}
	if err := a.Memory.Remove(r.PathValue("id")); err != nil {
		server.WriteError(w, server.StatusError{Status: 404, Msg: err.Error()})
		return
	}
	a.Events.Append(r.Context(), "memory.changed", "human:owner", map[string]string{"removed": r.PathValue("id")})
	server.WriteJSON(w, 200, map[string]string{"state": "removed"})
}

func (a *App) confirmFact(w http.ResponseWriter, r *http.Request) {
	if !a.needMemory(w) {
		return
	}
	if err := a.Memory.Confirm(r.PathValue("id")); err != nil {
		server.WriteError(w, server.StatusError{Status: 404, Msg: err.Error()})
		return
	}
	a.Events.Append(r.Context(), "memory.changed", "human:owner", map[string]string{"confirmed": r.PathValue("id")})
	server.WriteJSON(w, 200, map[string]string{"state": "confirmed"})
}

func (a *App) restoreMemory(w http.ResponseWriter, r *http.Request) {
	if !a.needMemory(w) {
		return
	}
	if err := a.Memory.Restore(r.PathValue("hash")); err != nil {
		server.WriteError(w, server.StatusError{Status: 404, Msg: err.Error()})
		return
	}
	a.Events.Append(r.Context(), "memory.changed", "human:owner", map[string]string{"restored": r.PathValue("hash")})
	server.WriteJSON(w, 200, map[string]string{"state": "restored"})
}
