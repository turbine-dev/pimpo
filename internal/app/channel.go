package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
)

// A generic channel, for bridges nobody built into Pimpo (Matrix, Signal,
// SMS, a smart speaker): notices go out as signed webhooks, and requests
// and button taps come back through the API with a device token.

var webhookClient = &http.Client{Timeout: 10 * time.Second}

func (a *App) channelRoutes() {
	a.Server.Handle("PUT /api/channel/webhook", a.setChannelWebhook)
	a.Server.Handle("POST /api/channel/message", a.channelMessage)
	a.Server.Handle("POST /api/channel/button", a.channelButton)
}

func (a *App) setChannelWebhook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if req.URL != "" && (err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && private(u.Hostname())))) {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "use an https address, or http on this machine or network"})
		return
	}
	ctx := r.Context()
	secret, _ := a.Vault.Get(ctx, "channel.webhook_secret")
	if secret == "" {
		secret = importID() + importID() + importID()
		a.Vault.Set(ctx, "channel.webhook_secret", secret)
	}
	a.Events.Put(ctx, "channel.webhook", strings.TrimSpace(req.URL))
	server.WriteJSON(w, 200, map[string]string{"url": req.URL, "secret": secret})
}

// mirrorWebhook posts a notice to the configured bridge, signed with
// HMAC-SHA256 in X-Pimpo-Signature so the bridge can tell it is Pimpo.
// The bridge is the owner's: notices for anyone else never reach it.
func (a *App) mirrorWebhook(ctx context.Context, n explore.Notice) {
	if people.Norm(n.To) != people.OwnerID {
		return
	}
	target, _ := a.Events.Get(ctx, "channel.webhook")
	secret, _ := a.Vault.Get(ctx, "channel.webhook_secret")
	if target == "" || secret == "" {
		return
	}
	actions := []map[string]string{}
	for _, act := range n.Actions {
		actions = append(actions, map[string]string{"label": act.Label, "data": act.Data})
	}
	body, _ := json.Marshal(map[string]any{"to": people.Norm(n.To), "text": n.Text, "actions": actions, "sent": time.Now().UTC()})
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	req, err := http.NewRequestWithContext(context.WithoutCancel(ctx), "POST", target, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pimpo-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	if resp, err := webhookClient.Do(req); err == nil {
		resp.Body.Close()
	}
}

type channelRequest struct {
	Person string `json:"person"`
	Text   string `json:"text"`
	Action string `json:"action"`
	ID     string `json:"id"`
	// Data is the button's data as the notice carried it, "approve:ab12".
	Data string `json:"data"`
}

// channelPerson is who a bridge request acts for. The bridge was set up
// by the owner with the owner's token, so it speaks only for the owner:
// acting as someone else would reach their chats and approvals.
func (a *App) channelPerson(ctx context.Context, id string) (context.Context, error) {
	if id == "" || id == people.OwnerID {
		return people.With(ctx, people.OwnerID), nil
	}
	return ctx, server.StatusError{Status: 403, Msg: "the channel speaks only for the administrator"}
}

func (a *App) channelMessage(w http.ResponseWriter, r *http.Request) {
	var req channelRequest
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	ctx, err := a.channelPerson(r.Context(), req.Person)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	reply, err := handler{a}.Request(context.WithoutCancel(ctx), req.Text)
	respond(w)(map[string]string{"reply": reply}, err)
}

func (a *App) channelButton(w http.ResponseWriter, r *http.Request) {
	var req channelRequest
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	if req.Data != "" {
		req.Action, req.ID, _ = strings.Cut(req.Data, ":")
	}
	ctx, err := a.channelPerson(r.Context(), req.Person)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	reply, err := handler{a}.Button(context.WithoutCancel(ctx), req.Action, req.ID)
	respond(w)(map[string]string{"reply": reply}, err)
}
