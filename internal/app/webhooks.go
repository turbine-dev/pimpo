package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/store"
)

// Webhooks start a routine when another service calls its address: an
// iPhone Shortcut, IFTTT or Zapier, GitHub, a form. Each routine that has
// one gets a secret address of its own; what was sent reaches the routine
// as event.webhook. A paused routine does not start, a call may carry at
// most 256 KB, and a routine starts at most 30 times a minute this way.

const (
	webhookMax     = 256 << 10
	webhookPerMin  = 30
	webhookKeyName = "webhook."
)

var hookCalls = struct {
	sync.Mutex
	at map[string][]time.Time
}{at: map[string][]time.Time{}}

// hookAllowed counts a call and says whether the routine is under its
// limit.
func hookAllowed(id string, now time.Time) bool {
	hookCalls.Lock()
	defer hookCalls.Unlock()
	recent := hookCalls.at[id][:0]
	for _, t := range hookCalls.at[id] {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= webhookPerMin {
		hookCalls.at[id] = recent
		return false
	}
	hookCalls.at[id] = append(recent, now)
	return true
}

func (a *App) webhookToken(ctx context.Context, id string) string {
	t, _ := a.Vault.Get(ctx, webhookKeyName+id)
	return t
}

// webhookURLs are the addresses the hook answers on: this computer, the
// home network, and the public address when there is one.
func (a *App) webhookURLs(r *http.Request, id, token string) map[string]string {
	if token == "" {
		return nil
	}
	path := "/hook/" + id + "/" + token
	out := map[string]string{"local": "http://" + r.Host + path}
	if lan := a.lanURL(); lan != "" {
		out["lan"] = strings.TrimRight(lan, "/") + path
	}
	if pub, _ := a.Events.Get(r.Context(), "public_url"); pub != "" {
		out["public"] = strings.TrimRight(pub, "/") + path
	}
	return out
}

func (a *App) webhookRoutes() {
	a.Server.Handle("GET /api/routines/{id}/webhook", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, err := a.myRoutine(r.Context(), id); err != nil {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such routine"})
			return
		}
		server.WriteJSON(w, 200, map[string]any{"urls": a.webhookURLs(r, id, a.webhookToken(r.Context(), id))})
	})
	// on, off or rotate: a new address replaces the old one at once.
	a.Server.Handle("POST /api/routines/{id}/webhook/{action}", func(w http.ResponseWriter, r *http.Request) {
		ctx, id := r.Context(), r.PathValue("id")
		if _, err := a.myRoutine(ctx, id); err != nil {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such routine"})
			return
		}
		switch r.PathValue("action") {
		case "on", "rotate":
			b := make([]byte, 24)
			rand.Read(b)
			if err := a.Vault.Set(ctx, webhookKeyName+id, hex.EncodeToString(b)); err != nil {
				server.WriteError(w, err)
				return
			}
		case "off":
			a.Vault.Delete(ctx, webhookKeyName+id)
		default:
			server.WriteError(w, server.StatusError{Status: 404, Msg: "unknown action"})
			return
		}
		a.Events.Append(ctx, "routine.webhook", actor(ctx), map[string]string{"routine": id, "action": r.PathValue("action")})
		server.WriteJSON(w, 200, map[string]any{"urls": a.webhookURLs(r, id, a.webhookToken(ctx, id))})
	})
	a.Server.HandlePublic("/hook/{id}/{token}", a.webhook)
}

func (a *App) webhook(w http.ResponseWriter, r *http.Request) {
	ctx, id := r.Context(), r.PathValue("id")
	want := a.webhookToken(ctx, id)
	if want == "" || subtle.ConstantTimeCompare([]byte(r.PathValue("token")), []byte(want)) != 1 {
		// Nothing says whether the routine exists.
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodGet && r.Method != http.MethodPut {
		server.WriteError(w, server.StatusError{Status: 405, Msg: "use POST or GET"})
		return
	}
	// The webhook's token is its credential; the routine may be anyone's.
	rt, err := a.Store.Routine(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if rt.State != store.RoutineActive {
		server.WriteError(w, server.StatusError{Status: 409, Msg: "the routine is paused"})
		return
	}
	if !hookAllowed(id, time.Now()) {
		server.WriteError(w, server.StatusError{Status: 429, Msg: "too many calls; wait a minute"})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, webhookMax+1))
	if err != nil || len(raw) > webhookMax {
		server.WriteError(w, server.StatusError{Status: 413, Msg: "at most 256 KB"})
		return
	}
	var body any = string(raw)
	var parsed any
	if len(raw) > 0 && json.Unmarshal(raw, &parsed) == nil {
		body = parsed
	} else if strings.Contains(r.Header.Get("Content-Type"), "x-www-form-urlencoded") {
		// The body was read above, so the form is parsed from the copy.
		if vals, err := url.ParseQuery(string(raw)); err == nil {
			form := map[string]string{}
			for k, v := range vals {
				form[k] = strings.Join(v, ",")
			}
			body = form
		}
	}
	query := map[string]string{}
	for k, v := range r.URL.Query() {
		query[k] = strings.Join(v, ",")
	}
	event := map[string]any{"webhook": map[string]any{"method": r.Method, "query": query, "body": body, "received": time.Now().Format(time.RFC3339)}}
	a.Events.Append(ctx, "routine.webhook.called", "system", map[string]any{"routine": id, "bytes": len(raw)})
	go a.Scheduler.RunWith(context.WithoutCancel(ctx), id, "webhook", event)
	server.WriteJSON(w, 202, map[string]any{"ok": true, "routine": id})
}

func (a *App) lanURL() string {
	if a.LAN == nil {
		return ""
	}
	return a.LAN.URL()
}
