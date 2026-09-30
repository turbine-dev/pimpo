package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/push"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/store"
)

// Push triggers: a routine that watches Gmail or Slack hears of new
// items as they happen instead of checking every few minutes.
//
// Gmail tells a Pub/Sub topic of the owner's Google Cloud project when a
// mailbox changes, and a push subscription posts that to /push/gmail on
// Pimpo's public address. The post carries a Google-signed ID token,
// checked against Google's keys, the configured audience and service
// account; it names only a mailbox and a history id. Pimpo then asks
// Gmail itself what reached the inbox, with that person's own token, and
// checks that person's watching routines exactly as a poll would, so a
// forged or repeated push can at most cause one extra check. A watch
// lasts 7 days and is renewed daily; while it is not live, routines poll.

const (
	pushGmailKey  = "push.gmail"
	gmailWatchKey = "gmail.push.watch"
	gmailPeople   = "gmail.push.people"
	pushMax       = 64 << 10
	// gmailRenew is how often a live watch is renewed; Google asks for
	// at least every 7 days and suggests daily.
	gmailRenew = 24 * time.Hour
)

// gmailPushConfig is the owner's Google Cloud setup for Gmail push.
type gmailPushConfig struct {
	// Topic is projects/<project>/topics/<topic>, where Gmail publishes.
	Topic string `json:"topic"`
	// Account is the service account the push subscription signs as.
	Account string `json:"account"`
	// Audience is what the subscription puts in its tokens; empty means
	// the push address itself.
	Audience string `json:"audience,omitempty"`
}

// gmailWatch is one person's live Gmail push.
type gmailWatch struct {
	Email   string    `json:"email"`
	History uint64    `json:"history"`
	Expires time.Time `json:"expires"`
	Renewed time.Time `json:"renewed"`
	Error   string    `json:"error,omitempty"`
}

var (
	topicPattern   = regexp.MustCompile(`^projects/[a-z][a-z0-9-]{4,28}[a-z0-9]/topics/[A-Za-z][\w.~+%-]{2,254}$`)
	accountPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	// gmailMu keeps one person's pushes and renewals from racing.
	gmailMu sync.Mutex
)

func (a *App) pushRoutes() {
	a.Server.Handle("GET /api/push/gmail", func(w http.ResponseWriter, r *http.Request) {
		server.WriteJSON(w, 200, a.gmailPushView(r.Context()))
	})
	a.Server.Handle("PUT /api/push/gmail", a.setGmailPush)
	a.Server.Handle("GET /api/routines/{id}/push", func(w http.ResponseWriter, r *http.Request) {
		rt, err := a.myRoutine(r.Context(), r.PathValue("id"))
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such routine"})
			return
		}
		server.WriteJSON(w, 200, a.pushStatus(r.Context(), rt))
	})
	a.Server.Handle("POST /api/routines/{id}/push/{action}", a.setRoutinePush)
	a.Server.HandlePublic("POST /push/gmail", a.gmailPushed)
	a.githubRoutes()
}

func (a *App) gmailPushConfig(ctx context.Context) gmailPushConfig {
	var c gmailPushConfig
	if raw, _ := a.Events.Get(ctx, pushGmailKey); raw != "" {
		json.Unmarshal([]byte(raw), &c)
	}
	return c
}

// gmailEndpoint is the public address Pub/Sub posts to, when there is one.
func (a *App) gmailEndpoint(ctx context.Context) string {
	if pub, _ := a.Events.Get(ctx, "public_url"); pub != "" {
		return strings.TrimRight(pub, "/") + "/push/gmail"
	}
	return ""
}

func (a *App) gmailAudience(ctx context.Context, c gmailPushConfig) string {
	if c.Audience != "" {
		return c.Audience
	}
	return a.gmailEndpoint(ctx)
}

func (c gmailPushConfig) ready() bool { return c.Topic != "" && c.Account != "" }

func (a *App) gmailPushView(ctx context.Context) map[string]any {
	c := a.gmailPushConfig(ctx)
	return map[string]any{"topic": c.Topic, "account": c.Account, "audience": c.Audience, "endpoint": a.gmailEndpoint(ctx), "ready": c.ready() && a.gmailAudience(ctx, c) != ""}
}

// setGmailPush saves the owner's Google Cloud setup; empty fields turn
// Gmail push off for the house.
func (a *App) setGmailPush(w http.ResponseWriter, r *http.Request) {
	var c gmailPushConfig
	if err := server.Decode(r, &c); err != nil {
		server.WriteError(w, err)
		return
	}
	c.Topic, c.Account, c.Audience = strings.TrimSpace(c.Topic), strings.TrimSpace(c.Account), strings.TrimSpace(c.Audience)
	if c.Topic != "" && !topicPattern.MatchString(c.Topic) {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "the topic looks like projects/my-project/topics/pimpo-gmail"})
		return
	}
	if c.Account != "" && !accountPattern.MatchString(c.Account) {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "the service account is an email address"})
		return
	}
	if c.Audience != "" && !strings.HasPrefix(c.Audience, "https://") {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "the audience is an https address"})
		return
	}
	b, _ := json.Marshal(c)
	a.Events.Put(r.Context(), pushGmailKey, string(b))
	a.Events.Append(r.Context(), "push.gmail.configured", actor(r.Context()), map[string]any{"on": c.ready()})
	go a.renewGmailWatches(context.WithoutCancel(r.Context()))
	server.WriteJSON(w, 200, a.gmailPushView(r.Context()))
}

// pushKind is how a routine's watch can hear of things as they happen:
// gmail, slack, or "" when it can only poll.
func pushKind(r store.Routine) string {
	w := r.Watch()
	if w == nil {
		return ""
	}
	switch strings.SplitN(w.Capability, ":", 2)[0] {
	case "gmail.search":
		return "gmail"
	case "slack.messages":
		return "slack"
	}
	return ""
}

func (a *App) setRoutinePush(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rt, err := a.myRoutine(ctx, r.PathValue("id"))
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such routine"})
		return
	}
	if pushKind(rt) == "" {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "only routines that watch Gmail or Slack can use push"})
		return
	}
	on := r.PathValue("action") == "on"
	if !on && r.PathValue("action") != "off" {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "unknown action"})
		return
	}
	s := rt.Settings
	s.Push = on
	if err := a.Store.SetRoutineSettings(ctx, rt.ID, s); err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(ctx, "routine.push", actor(ctx), map[string]any{"routine": rt.ID, "on": on})
	if pushKind(rt) == "gmail" {
		// Starting (or stopping) the watch answers at once; errors show
		// in the status and routines keep polling meanwhile.
		a.syncGmailWatch(people.With(context.WithoutCancel(ctx), rt.Person))
	}
	rt, _ = a.Store.Routine(ctx, rt.ID)
	server.WriteJSON(w, 200, a.pushStatus(ctx, rt))
}

// pushStatus is what the routine's trigger settings show.
func (a *App) pushStatus(ctx context.Context, rt store.Routine) map[string]any {
	kind := pushKind(rt)
	out := map[string]any{"kind": kind, "on": rt.Settings.Push, "live": rt.Settings.Push && a.pushLive(ctx, rt)}
	switch kind {
	case "gmail":
		pctx := people.With(ctx, rt.Person)
		c := a.gmailPushConfig(ctx)
		_, tokErr := a.gmailToken(pctx)
		g := map[string]any{"configured": c.ready() && a.gmailAudience(ctx, c) != "", "signed_in": tokErr == nil}
		if wt, ok := a.gmailWatchOf(pctx); ok {
			g["until"] = wt.Expires.Format(time.RFC3339)
			g["error"] = wt.Error
		}
		out["gmail"] = g
	case "slack":
		out["slack"] = map[string]any{"connected": a.slackUp(), "owner_only": people.Norm(rt.Person) != people.OwnerID}
	}
	return out
}

// pushLive says whether events reach a routine as they happen right now;
// the scheduler polls it only as a safety net then.
func (a *App) pushLive(ctx context.Context, rt store.Routine) bool {
	if !rt.Settings.Push {
		return false
	}
	switch pushKind(rt) {
	case "gmail":
		wt, ok := a.gmailWatchOf(people.With(ctx, rt.Person))
		return ok && wt.Error == "" && time.Now().Before(wt.Expires)
	case "slack":
		return people.Norm(rt.Person) == people.OwnerID && a.slackUp()
	}
	return false
}

// gmailToken is the access token of whoever ctx acts for, for their own
// mailbox. Only a mailbox signed in with Google has one: today the
// owner's; a member's IMAP mailbox keeps polling.
func (a *App) gmailToken(ctx context.Context) (string, error) {
	if a.GmailToken != nil {
		return a.GmailToken(ctx)
	}
	if auth, _ := a.Events.Get(ctx, personal(ctx, "mail.auth")); auth == "oauth" && a.Google != nil && people.From(ctx) == people.OwnerID {
		return a.Google.Token(ctx)
	}
	return "", errors.New("Gmail push needs the mailbox signed in with Google in Connections")
}

func (a *App) gmail() push.Gmail {
	return push.Gmail{API: a.GmailAPI, Token: a.gmailToken}
}

func (a *App) gmailWatchOf(ctx context.Context) (gmailWatch, bool) {
	var wt gmailWatch
	raw, _ := a.Events.Get(ctx, personal(ctx, gmailWatchKey))
	if raw == "" || json.Unmarshal([]byte(raw), &wt) != nil {
		return wt, false
	}
	return wt, true
}

func (a *App) saveGmailWatch(ctx context.Context, wt gmailWatch) {
	b, _ := json.Marshal(wt)
	a.Events.Put(ctx, personal(ctx, gmailWatchKey), string(b))
	if wt.Email != "" {
		a.Events.Put(ctx, "gmail.push.mailbox."+wt.Email, people.From(ctx))
	}
	a.gmailPeopleEdit(ctx, people.From(ctx), true)
}

func (a *App) dropGmailWatch(ctx context.Context) {
	if wt, ok := a.gmailWatchOf(ctx); ok && wt.Email != "" {
		a.Events.Put(ctx, "gmail.push.mailbox."+wt.Email, "")
	}
	a.Events.Put(ctx, personal(ctx, gmailWatchKey), "")
	a.gmailPeopleEdit(ctx, people.From(ctx), false)
}

// gmailPeopleEdit keeps the list of people with a watch, for renewals.
func (a *App) gmailPeopleEdit(ctx context.Context, person string, in bool) {
	var list []string
	raw, _ := a.Events.Get(ctx, gmailPeople)
	json.Unmarshal([]byte(raw), &list)
	has := slices.Contains(list, person)
	switch {
	case in && !has:
		list = append(list, person)
	case !in && has:
		list = slices.DeleteFunc(list, func(p string) bool { return p == person })
	default:
		return
	}
	b, _ := json.Marshal(list)
	a.Events.Put(ctx, gmailPeople, string(b))
}

// wantsGmailPush says whether any active routine of the person asked for
// Gmail push.
func (a *App) wantsGmailPush(ctx context.Context, person string) bool {
	routines, _ := a.Store.Routines(ctx)
	for _, r := range routines {
		if r.State == store.RoutineActive && r.Settings.Push && pushKind(r) == "gmail" && people.Norm(r.Person) == people.Norm(person) {
			return true
		}
	}
	return false
}

// syncGmailWatch starts, renews or stops the Gmail watch of whoever ctx
// acts for, so it exists exactly while one of their routines wants it.
func (a *App) syncGmailWatch(ctx context.Context) {
	gmailMu.Lock()
	defer gmailMu.Unlock()
	person := people.From(ctx)
	wt, had := a.gmailWatchOf(ctx)
	c := a.gmailPushConfig(ctx)
	if !a.wantsGmailPush(ctx, person) || !c.ready() {
		if had {
			a.gmail().Stop(ctx)
			a.dropGmailWatch(ctx)
		}
		return
	}
	if had && wt.Error == "" && time.Since(wt.Renewed) < gmailRenew && time.Until(wt.Expires) > 2*gmailRenew {
		return
	}
	got, err := a.gmail().Watch(ctx, c.Topic)
	if err != nil {
		wt.Error = err.Error()
		a.saveGmailWatch(ctx, wt)
		a.Events.Append(ctx, "push.gmail.failed", "system", map[string]string{"person": person, "error": err.Error()})
		return
	}
	email, _ := a.Events.Get(ctx, personal(ctx, "mail.user"))
	if email = strings.ToLower(strings.TrimSpace(email)); wt.Email != "" && wt.Email != email {
		a.Events.Put(ctx, "gmail.push.mailbox."+wt.Email, "")
	}
	wt.Email = email
	if wt.History == 0 || !had {
		wt.History = got.HistoryID
	}
	wt.Expires, wt.Renewed, wt.Error = got.Expires, time.Now(), ""
	a.saveGmailWatch(ctx, wt)
	a.Events.Append(ctx, "push.gmail.watching", "system", map[string]any{"person": person, "until": wt.Expires.Format(time.RFC3339)})
}

// renewGmailWatches brings every person's watch in line with what their
// routines want.
func (a *App) renewGmailWatches(ctx context.Context) {
	seen := map[string]bool{}
	var list []string
	raw, _ := a.Events.Get(ctx, gmailPeople)
	json.Unmarshal([]byte(raw), &list)
	routines, _ := a.Store.Routines(ctx)
	for _, r := range routines {
		if r.Settings.Push && pushKind(r) == "gmail" {
			list = append(list, people.Norm(r.Person))
		}
	}
	for _, p := range list {
		if !seen[p] {
			seen[p] = true
			a.syncGmailWatch(people.With(ctx, p))
		}
	}
}

func (a *App) pushLoop(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		a.renewGmailWatches(ctx)
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
}

func (a *App) pushVerifier() *push.Verifier {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.verifier == nil {
		a.verifier = &push.Verifier{CertsURL: a.GoogleCerts}
	}
	return a.verifier
}

// gmailPushed receives Pub/Sub's push. Any 2xx acknowledges it; an error
// makes Pub/Sub try again later.
func (a *App) gmailPushed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := a.gmailPushConfig(ctx)
	aud := a.gmailAudience(ctx, c)
	if !c.ready() || aud == "" {
		http.NotFound(w, r)
		return
	}
	tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if err := a.pushVerifier().Verify(ctx, tok, aud, c.Account); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, pushMax+1))
	if err != nil || len(raw) > pushMax {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		return
	}
	n, err := push.ParseNotification(raw)
	if err != nil {
		// Acknowledged, so a malformed message is not sent again.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := a.gmailChanged(ctx, n); err != nil {
		http.Error(w, "try again", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// gmailChanged asks Gmail what reached the mailbox since the last push
// and checks its person's pushed routines when something did. A push
// that is not newer than the last one changes nothing.
func (a *App) gmailChanged(ctx context.Context, n push.Notification) error {
	person, _ := a.Events.Get(ctx, "gmail.push.mailbox."+n.Email)
	if person == "" {
		return nil
	}
	pctx := people.With(context.WithoutCancel(ctx), person)
	gmailMu.Lock()
	wt, ok := a.gmailWatchOf(pctx)
	if !ok || wt.Email != n.Email || n.HistoryID <= wt.History {
		gmailMu.Unlock()
		return nil
	}
	ids, latest, err := a.gmail().Added(pctx, wt.History)
	if err != nil && !errors.Is(err, push.ErrHistoryGone) {
		gmailMu.Unlock()
		return err
	}
	wt.History = max(n.HistoryID, latest)
	a.saveGmailWatch(pctx, wt)
	gmailMu.Unlock()
	if len(ids) == 0 && err == nil {
		return nil
	}
	a.Events.Append(pctx, "push.gmail.received", "system", map[string]any{"person": person, "messages": len(ids)})
	a.pokePushed(pctx, "gmail")
	return nil
}

// pokePushed checks at once the pushed routines of whoever ctx acts for
// that watch kind, exactly as their next poll would.
func (a *App) pokePushed(ctx context.Context, kind string) {
	routines, err := a.Store.Routines(ctx)
	if err != nil {
		return
	}
	for _, r := range routines {
		if r.State == store.RoutineActive && r.Settings.Push && pushKind(r) == kind && mine(ctx, r.Person) {
			go a.Scheduler.Poll(context.WithoutCancel(ctx), r.ID)
		}
	}
}
