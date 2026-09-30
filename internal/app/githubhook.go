package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/push"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/store"
)

// A GitHub webhook starts a routine when something happens in a
// repository: a push, a pull request, an issue. Each routine gets its own
// secret, shown once; GitHub signs every delivery with it
// (X-Hub-Signature-256) and Pimpo checks that HMAC over the raw body
// before reading anything. Each delivery id starts the routine once, so
// a redelivered or replayed call repeats nothing. The event reaches the
// routine as event.github: the event type, the action and the payload
// without its API links, as data.

const (
	githubMax       = 1 << 20
	githubKeyName   = "github.hook."
	githubDelivered = "github.deliveries."
	deliveriesKept  = 300
)

var githubMu sync.Mutex

func (a *App) githubSecret(ctx context.Context, id string) string {
	t, _ := a.Vault.Get(ctx, githubKeyName+id)
	return t
}

func (a *App) githubURLs(r *http.Request, id string) map[string]string {
	path := "/github-hook/" + id
	out := map[string]string{"local": "http://" + r.Host + path}
	if pub, _ := a.Events.Get(r.Context(), "public_url"); pub != "" {
		out["public"] = strings.TrimRight(pub, "/") + path
	}
	return out
}

func (a *App) githubRoutes() {
	a.Server.Handle("GET /api/routines/{id}/github", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, err := a.myRoutine(r.Context(), id); err != nil {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such routine"})
			return
		}
		on := a.githubSecret(r.Context(), id) != ""
		out := map[string]any{"on": on}
		if on {
			out["urls"] = a.githubURLs(r, id)
		}
		server.WriteJSON(w, 200, out)
	})
	// on or rotate make a new secret, returned only in this answer; off
	// forgets it.
	a.Server.Handle("POST /api/routines/{id}/github/{action}", func(w http.ResponseWriter, r *http.Request) {
		ctx, id := r.Context(), r.PathValue("id")
		if _, err := a.myRoutine(ctx, id); err != nil {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such routine"})
			return
		}
		out := map[string]any{"on": false}
		switch r.PathValue("action") {
		case "on", "rotate":
			b := make([]byte, 24)
			rand.Read(b)
			secret := hex.EncodeToString(b)
			if err := a.Vault.Set(ctx, githubKeyName+id, secret); err != nil {
				server.WriteError(w, err)
				return
			}
			out = map[string]any{"on": true, "urls": a.githubURLs(r, id), "secret": secret}
		case "off":
			a.Vault.Delete(ctx, githubKeyName+id)
		default:
			server.WriteError(w, server.StatusError{Status: 404, Msg: "unknown action"})
			return
		}
		a.Events.Append(ctx, "routine.github", actor(ctx), map[string]string{"routine": id, "action": r.PathValue("action")})
		server.WriteJSON(w, 200, out)
	})
	a.Server.HandlePublic("POST /github-hook/{id}", a.githubHook)
}

// firstDelivery records a delivery id and says whether it is new.
func (a *App) firstDelivery(ctx context.Context, id, delivery string) bool {
	githubMu.Lock()
	defer githubMu.Unlock()
	var seen []string
	raw, _ := a.Events.Get(ctx, githubDelivered+id)
	json.Unmarshal([]byte(raw), &seen)
	if slices.Contains(seen, delivery) {
		return false
	}
	seen = append(seen, delivery)
	if len(seen) > deliveriesKept {
		seen = seen[len(seen)-deliveriesKept:]
	}
	b, _ := json.Marshal(seen)
	a.Events.Put(ctx, githubDelivered+id, string(b))
	return true
}

func (a *App) githubHook(w http.ResponseWriter, r *http.Request) {
	ctx, id := r.Context(), r.PathValue("id")
	secret := a.githubSecret(ctx, id)
	raw, err := io.ReadAll(io.LimitReader(r.Body, githubMax+1))
	if err != nil || len(raw) > githubMax {
		server.WriteError(w, server.StatusError{Status: 413, Msg: "at most 1 MB"})
		return
	}
	// A wrong signature and a routine without a GitHub webhook look the
	// same: nothing says whether the routine exists.
	if secret == "" || !push.SignatureOK(secret, raw, r.Header.Get("X-Hub-Signature-256")) {
		http.NotFound(w, r)
		return
	}
	rt, err := a.Store.Routine(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	kind, delivery := r.Header.Get("X-GitHub-Event"), r.Header.Get("X-GitHub-Delivery")
	if kind == "" || delivery == "" {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "not a GitHub delivery"})
		return
	}
	if kind == "ping" {
		server.WriteJSON(w, 200, map[string]any{"ok": true})
		return
	}
	if rt.State != store.RoutineActive {
		server.WriteError(w, server.StatusError{Status: 409, Msg: "the routine is paused"})
		return
	}
	if !hookAllowed("github:"+id, time.Now()) {
		server.WriteError(w, server.StatusError{Status: 429, Msg: "too many calls; wait a minute"})
		return
	}
	if !a.firstDelivery(ctx, id, delivery) {
		server.WriteJSON(w, 200, map[string]any{"ok": true, "duplicate": true})
		return
	}
	body := raw
	if strings.Contains(r.Header.Get("Content-Type"), "x-www-form-urlencoded") {
		if vals, err := url.ParseQuery(string(raw)); err == nil {
			body = []byte(vals.Get("payload"))
		}
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "the payload is not JSON"})
		return
	}
	action, _ := payload["action"].(string)
	gh := map[string]any{"event": kind, "delivery": delivery, "action": action, "payload": push.Trim(payload), "received": time.Now().Format(time.RFC3339)}
	a.Events.Append(ctx, "routine.github.called", "system", map[string]any{"routine": id, "event": kind, "bytes": len(raw)})
	go a.Scheduler.RunWith(context.WithoutCancel(ctx), id, "github", map[string]any{"github": gh})
	server.WriteJSON(w, 202, map[string]any{"ok": true, "routine": id})
}
