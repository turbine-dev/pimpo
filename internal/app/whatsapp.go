package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/owner"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/connector"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/whatsapp"
)

// WhatsApp is a second channel through the official Cloud API: the owner
// registers a business number, Meta posts messages to our webhook, and we
// answer through the Graph API.

func (a *App) whatsappRoutes() {
	a.Server.HandlePublic("GET /webhook/whatsapp", a.whatsappVerify)
	a.Server.HandlePublic("POST /webhook/whatsapp", a.whatsappWebhook)
}

// wa builds a client from the stored settings, or nil when not set up.
func (a *App) wa(ctx context.Context) *whatsapp.Client {
	tok, err := a.Vault.Get(ctx, "whatsapp.token")
	phone, _ := a.Events.Get(ctx, "whatsapp.phone_id")
	if err != nil || tok == "" || phone == "" {
		return nil
	}
	return &whatsapp.Client{Token: tok, PhoneID: phone, BaseURL: a.WhatsAppAPI}
}

func (a *App) ownerWhatsApp(ctx context.Context) string {
	id, _ := a.Events.Get(ctx, "whatsapp.owner")
	return id
}

// whatsappVerify answers Meta's subscription check with the challenge
// when the verify token matches.
func (a *App) whatsappVerify(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	want, _ := a.Vault.Get(r.Context(), "whatsapp.verify_token")
	if q.Get("hub.mode") != "subscribe" || want == "" || subtle.ConstantTimeCompare([]byte(q.Get("hub.verify_token")), []byte(want)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	io.WriteString(w, q.Get("hub.challenge"))
}

func (a *App) whatsappWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	secret, _ := a.Vault.Get(ctx, "whatsapp.app_secret")
	if err := whatsapp.Verify(body, r.Header.Get("X-Hub-Signature-256"), secret); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	msgs, err := whatsapp.Parse(body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// Meta retries anything slower than a few seconds; answer first.
	w.WriteHeader(http.StatusOK)
	bg := context.WithoutCancel(ctx)
	for _, m := range msgs {
		if waSeen.fresh(m, time.Now()) {
			a.whatsappMessage(bg, m)
		}
	}
}

// A signed webhook body stays valid forever, so Meta's retries and
// anyone replaying a captured body would repeat a message (an approval,
// a request). Each message id is handled once; old messages are dropped.
const (
	waRecent = 2048
	waMaxAge = 24 * time.Hour
)

type recentIDs struct {
	mu    sync.Mutex
	seen  map[string]bool
	order []string
}

var waSeen recentIDs

// fresh says whether m is new: it has an id not seen among the recent
// ones and is not older than waMaxAge. It remembers m.
func (r *recentIDs) fresh(m whatsapp.Inbound, now time.Time) bool {
	if !m.Time.IsZero() && now.Sub(m.Time) > waMaxAge {
		return false
	}
	if m.ID == "" {
		// Meta always sends one; without it a replay cannot be told apart.
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seen[m.ID] {
		return false
	}
	if r.seen == nil {
		r.seen = map[string]bool{}
	}
	if len(r.order) >= waRecent {
		delete(r.seen, r.order[0])
		r.order = r.order[1:]
	}
	r.seen[m.ID] = true
	r.order = append(r.order, m.ID)
	return true
}

func (a *App) whatsappMessage(ctx context.Context, m whatsapp.Inbound) {
	c := a.wa(ctx)
	if c == nil {
		return
	}
	reply := func(text string) { c.Send(ctx, m.From, text) }
	text := strings.TrimSpace(m.Text)
	if code, ok := pairingCode(text); ok {
		a.whatsappPair(ctx, m, code, reply)
		return
	}
	var person string
	if a.ownerWhatsApp(ctx) == m.From {
		person = people.OwnerID
	} else if p, ok := a.People.ByWhatsApp(ctx, m.From); ok {
		person = p.ID
	} else {
		// Strangers get nothing, as on Telegram.
		return
	}
	pctx := people.With(ctx, person)
	h := handler{a}
	if m.Button != "" {
		action, id, _ := strings.Cut(m.Button, ":")
		out, err := h.Button(pctx, action, id)
		if err != nil {
			out = "⚠️ " + err.Error()
		}
		if out != "" {
			reply(out)
		}
		return
	}
	if text == "" {
		return
	}
	if m.ReplyTo != "" {
		// Words in reply to a question are checked against its options.
		if out, ok := h.Reply(pctx, waSent.choices(m.ReplyTo), text); ok {
			reply(out)
			return
		}
	}
	out, err := h.Request(owner.Via(pctx, "whatsapp"), text)
	if err != nil {
		out = i18n.T(ctx, "msg.start.failed", "error", err)
	}
	reply(out)
}

func pairingCode(text string) (string, bool) {
	for _, prefix := range []string{"/start ", "pimpo ", "Pimpo ", "zodim ", "Zodim "} {
		if code, ok := strings.CutPrefix(text, prefix); ok {
			return strings.TrimSpace(code), true
		}
	}
	return "", false
}

// whatsappPair links the number to the owner or an invited person. Wrong
// codes get no answer, so guessing teaches nothing, and they count
// against the number.
func (a *App) whatsappPair(ctx context.Context, m whatsapp.Inbound, code string, reply func(string)) {
	sender := "whatsapp:" + m.From
	if a.Channel.Blocked(sender) {
		return
	}
	_, taken := a.People.ByWhatsApp(ctx, m.From)
	if taken || a.ownerWhatsApp(ctx) == m.From {
		return
	}
	if a.ownerWhatsApp(ctx) == "" && a.Channel.ClaimOwner(code) {
		a.Events.Put(ctx, "whatsapp.owner", m.From)
		a.Events.Append(ctx, "whatsapp.paired", "human:owner", map[string]string{"person": people.OwnerID})
		reply(i18n.T(ctx, "msg.pair.whatsapp"))
		return
	}
	p, err := a.People.PairWhatsApp(ctx, code, m.From)
	if err == nil {
		a.Events.Append(ctx, "whatsapp.paired", "human:"+p.ID, map[string]string{"person": p.ID})
		reply(i18n.T(ctx, "msg.pair.whatsappPerson", "name", p.Name))
		return
	}
	if errors.Is(err, people.ErrUnknown) {
		a.Channel.Wrong(sender)
	}
}

// mirrorWhatsApp sends a notice to the person on WhatsApp too. WhatsApp
// shows three buttons at most: the less common "rest of this run" goes,
// and so does "always" when "for this routine", the narrower lasting
// answer, takes its place; a question with more options comes as a list.
func (a *App) mirrorWhatsApp(ctx context.Context, n explore.Notice) {
	c := a.wa(ctx)
	if c == nil {
		return
	}
	p, err := a.People.Get(ctx, people.Norm(n.To))
	if err != nil || p.WhatsApp == "" {
		return
	}
	grant := slices.ContainsFunc(n.Actions, func(act explore.Action) bool { return strings.HasPrefix(act.Data, "grant:") })
	var buttons []whatsapp.Button
	for _, act := range n.Actions {
		if strings.HasPrefix(act.Data, "batch:") || (grant && strings.HasPrefix(act.Data, "always:")) {
			continue
		}
		buttons = append(buttons, whatsapp.Button{ID: act.Data, Title: act.Label})
	}
	// Not a health signal: a failed send is usually about that message
	// (the 24-hour window, a blocked number), not WhatsApp being down.
	var id string
	if len(buttons) > 3 && questionOf(n.Actions) != "" {
		id, _ = c.SendList(ctx, p.WhatsApp, n.Text, i18n.T(ctx, "msg.answer.choose"), buttons)
	} else {
		id, _ = c.SendID(ctx, p.WhatsApp, n.Text, buttons...)
	}
	if questionOf(n.Actions) != "" {
		waSent.remember(id, n.Actions)
	}
}

// sentChoices are the questions sent on WhatsApp by message id, so a
// reply in words to one is checked against its options.
type sentChoices struct {
	mu    sync.Mutex
	byID  map[string][]explore.Action
	order []string
}

var waSent sentChoices

func (s *sentChoices) remember(id string, choices []explore.Action) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byID == nil {
		s.byID = map[string][]explore.Action{}
	}
	s.byID[id] = choices
	s.order = append(s.order, id)
	for len(s.order) > waRecent {
		delete(s.byID, s.order[0])
		s.order = s.order[1:]
	}
}

func (s *sentChoices) choices(id string) []explore.Action {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.byID[id]
}

// whatsappCap is whatsapp.send (to whoever the run works for) and
// whatsapp.send_to (anyone, always approved first).
type whatsappCap struct{ a *App }

func (whatsappCap) Capabilities() []string { return []string{"whatsapp.send", "whatsapp.send_to"} }

func (w whatsappCap) Call(ctx context.Context, name, _ string, args any) (any, error) {
	var in struct {
		To   string `json:"to"`
		Text string `json:"text"`
	}
	if err := connector.Args(args, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Text) == "" {
		return nil, errors.New("text is empty")
	}
	c := w.a.wa(ctx)
	if c == nil {
		return nil, errors.New("WhatsApp is not set up; open Connections")
	}
	to := whatsapp.Normalize(in.To)
	if name == "whatsapp.send" {
		p, err := w.a.People.Get(ctx, people.From(ctx))
		if err != nil || p.WhatsApp == "" {
			return nil, errors.New("WhatsApp is not paired yet; open Connections to pair it")
		}
		to = p.WhatsApp
	} else if len(to) < 8 {
		return nil, errors.New("to must be a phone number with country code")
	}
	if err := c.Send(ctx, to, in.Text); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func (a *App) putWhatsApp(ctx context.Context, req map[string]string) error {
	tok, phone, secret := strings.TrimSpace(req["token"]), strings.TrimSpace(req["phone_id"]), strings.TrimSpace(req["app_secret"])
	if tok == "" || phone == "" || secret == "" {
		return server.StatusError{Status: 400, Msg: "access token, phone number id and app secret are required"}
	}
	if err := a.Vault.Set(ctx, "whatsapp.token", tok); err != nil {
		return err
	}
	if err := a.Vault.Set(ctx, "whatsapp.app_secret", secret); err != nil {
		return err
	}
	if _, err := a.Vault.Get(ctx, "whatsapp.verify_token"); err != nil {
		a.Vault.Set(ctx, "whatsapp.verify_token", importID()+importID())
	}
	return a.Events.Put(ctx, "whatsapp.phone_id", phone)
}
