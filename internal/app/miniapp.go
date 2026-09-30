package app

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/telegram"
)

// The Telegram Mini App is a small dashboard Telegram opens inside the
// chat with Pimpo's bot: what waits for the person, their routines, what
// was spent and their widgets. Telegram signs who opened it (initData);
// Pimpo checks that signature with the bot token, finds the person who
// paired that Telegram account and opens a short session for them alone.
// The page keeps that token in memory: in Telegram Web it runs in a frame
// of another site, where the Strict session cookie is never sent.

const (
	// initDataAge is how long after Telegram signed initData it still
	// opens a session.
	initDataAge = time.Hour
	// miniAppLife is how long a Mini App session lasts.
	miniAppLife = time.Hour
	// miniAppKeep is how many Mini App sessions a person keeps at once.
	miniAppKeep = 5
	menuKey     = "telegram.menu"
)

// miniAppRoutes are all a Mini App session opens, on top of what the
// person may use anyway: the page's own few screens, nothing else.
var miniAppRoutes = routeSet(
	"GET /api/state",
	"GET /api/approvals", "POST /api/approvals/{id}/{answer}",
	"GET /api/questions", "POST /api/questions/{id}/answer",
	"GET /api/routines", "POST /api/routines/{id}/{action}",
	"GET /api/cost",
	"GET /api/dashboards", "GET /api/dashboards/{id}/widgets", "GET /api/widgets", "GET /api/widgets/{id}", "POST /api/widgets/{id}/refresh",
)

func (a *App) miniAppRoutes() {
	a.Server.Narrow = a.narrow
	a.Server.HandlePublic("POST /api/tg/session", a.miniAppSession)
	a.Channel.MiniApp = a.miniAppURL
	a.Channel.Paired = func(ctx context.Context) { go a.syncMiniApp(context.WithoutCancel(ctx)) }
}

// narrow keeps a Mini App session to the Mini App's routes.
func (a *App) narrow(token, pattern string) bool {
	if token == "" {
		return true
	}
	h := hashToken(token)
	devicesMu.Lock()
	defer devicesMu.Unlock()
	for _, d := range a.devices(context.Background()) {
		if d.Hash == h {
			return !d.MiniApp || miniAppRoutes[pattern]
		}
	}
	return true
}

// miniAppURL is the Mini App's address. Telegram opens only https pages,
// so without a public https address (Tailscale Funnel, or the one the
// owner gave) there is no Mini App to offer.
func (a *App) miniAppURL(ctx context.Context) string {
	base, _ := a.Events.Get(ctx, "public_url")
	base = strings.TrimSuffix(strings.TrimSpace(base), "/")
	if !strings.HasPrefix(strings.ToLower(base), "https://") || len(base) <= len("https://") {
		return ""
	}
	return base + server.MiniAppPath
}

// telegramPerson is who paired the Telegram account id. People pair only
// in their own private chat with the bot, whose id is their account's.
func (a *App) telegramPerson(ctx context.Context, user int64) (string, bool) {
	if user == 0 {
		return "", false
	}
	if chat, _ := a.Channel.Chat(ctx); chat == user {
		return people.OwnerID, true
	}
	if p, ok := a.People.ByChat(ctx, user); ok {
		return p.ID, true
	}
	return "", false
}

// miniAppSession trades Telegram's signed initData for a session of the
// person who paired that account. Anyone else gets nothing.
func (a *App) miniAppSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		InitData string `json:"init_data"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "send init_data"})
		return
	}
	token, _ := a.Vault.Get(ctx, "telegram.token")
	if token == "" {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "Telegram is not set up"})
		return
	}
	u, err := telegram.VerifyInitData(req.InitData, token, initDataAge, time.Now())
	if err != nil {
		if a.Server.Attempt(r) {
			server.WriteError(w, server.StatusError{Status: 429, Msg: "too many wrong sign-ins from here; wait a few minutes"})
			return
		}
		server.WriteError(w, server.StatusError{Status: 401, Msg: err.Error()})
		return
	}
	person, ok := a.telegramPerson(ctx, u.ID)
	if !ok {
		server.WriteError(w, server.StatusError{Status: 403, Msg: "this Telegram account is not paired with Pimpo"})
		return
	}
	session, id, expires := a.newMiniAppSession(ctx, person)
	a.Events.Append(ctx, "telegram.miniapp.opened", "human:"+person, map[string]string{"id": id, "person": person})
	pctx := people.With(ctx, person)
	server.WriteJSON(w, 200, map[string]any{"token": session, "expires": expires.UTC().Format(time.RFC3339), "person": person, "role": a.roleOf(pctx), "name": a.nameOf(pctx)})
}

// newMiniAppSession opens a short session for person, dropping their
// Mini App sessions that ran out and all but the newest few.
func (a *App) newMiniAppSession(ctx context.Context, person string) (token, id string, expires time.Time) {
	token, id = newToken()
	now := time.Now()
	expires = now.Add(miniAppLife)
	devicesMu.Lock()
	defer devicesMu.Unlock()
	list := a.devices(ctx)
	kept := list[:0]
	var theirs []int
	for _, d := range list {
		if d.MiniApp && d.expired(now) {
			continue
		}
		if d.MiniApp && people.Norm(d.Person) == person {
			theirs = append(theirs, len(kept))
		}
		kept = append(kept, d)
	}
	if extra := len(theirs) - (miniAppKeep - 1); extra > 0 {
		sort.Slice(theirs, func(i, j int) bool { return kept[theirs[i]].Created.Before(kept[theirs[j]].Created) })
		drop := map[int]bool{}
		for _, i := range theirs[:extra] {
			drop[i] = true
		}
		left := kept[:0]
		for i, d := range kept {
			if !drop[i] {
				left = append(left, d)
			}
		}
		kept = left
	}
	kept = append(kept, Device{ID: id, Name: "Telegram Mini App", Hash: hashToken(token), Created: now, LastSeen: now, Person: personField(person), Session: true, MiniApp: true, Expires: expires})
	a.saveDevices(ctx, kept)
	return token, id, expires
}

var menuMu sync.Mutex

// syncMiniApp gives each paired chat a menu button that opens the Mini
// App, or takes it away when there is no https address any more. It
// remembers what each chat has, so Telegram is asked only for changes.
func (a *App) syncMiniApp(ctx context.Context) {
	bot, ok := a.bot(ctx).(telegram.Bot)
	if !ok {
		return
	}
	menuMu.Lock()
	defer menuMu.Unlock()
	url := a.miniAppURL(ctx)
	want := map[string]string{}
	if chat, _ := a.Channel.Chat(ctx); chat != 0 {
		want[strconv.FormatInt(chat, 10)] = url
	}
	if list, err := a.People.List(ctx); err == nil {
		for _, p := range list {
			if p.Chat != 0 {
				want[strconv.FormatInt(p.Chat, 10)] = url
			}
		}
	}
	had := map[string]string{}
	if raw, _ := a.Events.Get(ctx, menuKey); raw != "" {
		json.Unmarshal([]byte(raw), &had)
	}
	for chat := range had {
		if _, still := want[chat]; !still {
			want[chat] = ""
		}
	}
	now := map[string]string{}
	for chat, u := range want {
		prev, known := had[chat]
		if !known && u == "" {
			continue
		}
		if known && prev == u {
			if u != "" {
				now[chat] = u
			}
			continue
		}
		id, _ := strconv.ParseInt(chat, 10, 64)
		if err := bot.SetMenuButton(ctx, id, i18n.T(ctx, "msg.miniapp.button"), u); err != nil {
			if known {
				now[chat] = prev
			}
			continue
		}
		if u != "" {
			now[chat] = u
		}
	}
	b, _ := json.Marshal(now)
	a.Events.Put(ctx, menuKey, string(b))
}
