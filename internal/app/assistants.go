package app

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/server"
)

// Assistants are named roles for the agent: instructions for the job and
// the capabilities it may use, enforced by the host, not by the prompt.
// They belong to the owner; people in the house are People.

type Assistant struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Emoji        string   `json:"emoji"`
	Instructions string   `json:"instructions"`
	Capabilities []string `json:"capabilities"`
}

const assistantsKey = "assistants"

var assistantsMu sync.Mutex

func (a *App) assistants(ctx context.Context) []Assistant {
	list := []Assistant{}
	if raw, _ := a.Events.Get(ctx, assistantsKey); raw != "" {
		json.Unmarshal([]byte(raw), &list)
	}
	return list
}

func (a *App) assistant(ctx context.Context, id string) (Assistant, bool) {
	for _, as := range a.assistants(ctx) {
		if as.ID == id {
			return as, true
		}
	}
	return Assistant{}, false
}

func (as Assistant) role() *explore.Assistant {
	return &explore.Assistant{Name: as.Name, Instructions: as.Instructions, Capabilities: as.Capabilities}
}

func (a *App) assistantRoutes() {
	a.Server.Handle("GET /api/assistants", func(w http.ResponseWriter, r *http.Request) { server.WriteJSON(w, 200, a.assistants(r.Context())) })
	a.Server.Handle("PUT /api/assistants/{id}", a.putAssistant)
	a.Server.Handle("DELETE /api/assistants/{id}", a.deleteAssistant)
	a.Server.Handle("GET /api/capabilities", func(w http.ResponseWriter, r *http.Request) {
		type spec struct {
			Name      string `json:"name"`
			Risk      string `json:"risk"`
			Signature string `json:"signature"`
			Returns   string `json:"returns"`
		}
		out := []spec{}
		for _, n := range capability.Names() {
			c := capability.Catalog[n]
			out = append(out, spec{n, c.Risk.String(), c.Signature, c.Returns})
		}
		server.WriteJSON(w, 200, out)
	})
}

var assistantID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

func (a *App) putAssistant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ownerOnly(w, r) {
		return
	}
	var as Assistant
	if err := server.Decode(r, &as); err != nil {
		server.WriteError(w, err)
		return
	}
	bad := func(msg string) { server.WriteError(w, server.StatusError{Status: 400, Msg: msg}) }
	as.ID = r.PathValue("id")
	as.Name = strings.TrimSpace(as.Name)
	as.Instructions = strings.TrimSpace(as.Instructions)
	switch {
	case !assistantID.MatchString(as.ID):
		bad("the id must be lowercase letters, digits and dashes")
		return
	case as.Name == "" || len([]rune(as.Name)) > 40:
		bad("give the assistant a name of up to 40 characters")
		return
	case len([]rune(as.Instructions)) > 2000:
		bad("keep the instructions under 2000 characters")
		return
	case len([]rune(as.Emoji)) > 4:
		bad("use a single emoji")
		return
	}
	seen := map[string]bool{}
	caps := []string{}
	for _, c := range as.Capabilities {
		if _, ok := capability.Catalog[c]; !ok {
			bad("unknown capability " + c)
			return
		}
		if !seen[c] {
			seen[c] = true
			caps = append(caps, c)
		}
	}
	sort.Strings(caps)
	as.Capabilities = caps
	assistantsMu.Lock()
	list := a.assistants(ctx)
	replaced := false
	for i := range list {
		if list[i].ID == as.ID {
			list[i], replaced = as, true
		}
	}
	if !replaced {
		list = append(list, as)
	}
	b, _ := json.Marshal(list)
	err := a.Events.Put(ctx, assistantsKey, string(b))
	assistantsMu.Unlock()
	if err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(ctx, "assistant.saved", actor(ctx), map[string]any{"id": as.ID, "capabilities": as.Capabilities})
	server.WriteJSON(w, 200, as)
}

func (a *App) deleteAssistant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ownerOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	assistantsMu.Lock()
	list := a.assistants(ctx)
	kept := list[:0]
	for _, as := range list {
		if as.ID != id {
			kept = append(kept, as)
		}
	}
	found := len(kept) != len(list)
	b, _ := json.Marshal(kept)
	a.Events.Put(ctx, assistantsKey, string(b))
	assistantsMu.Unlock()
	if !found {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such assistant"})
		return
	}
	a.Events.Append(ctx, "assistant.removed", actor(ctx), map[string]string{"id": id})
	server.WriteJSON(w, 200, map[string]string{"deleted": id})
}
