package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
)

// Passkeys let each person sign in on a computer or phone with the lock of
// the device (Touch ID, Face ID, Windows Hello, a security key) instead of
// a link. A person adds one while signed in; signing in with it opens a
// session of theirs like a paired device, which expires when unused and
// is revoked like any device. Pimpo keeps only the public keys.
//
// A passkey belongs to a web address: it works where Pimpo is opened by
// name (localhost on this computer, or an https address such as the
// Tailscale one), not by a bare IP address.

const (
	passkeysKey    = "passkeys"
	passkeyHandles = "passkey.handles"
	passkeyIdle    = 30 * 24 * time.Hour
	ceremonyTTL    = 5 * time.Minute
)

type storedPasskey struct {
	ID         string              `json:"id"`
	Person     string              `json:"person"`
	Name       string              `json:"name"`
	RPID       string              `json:"rp_id"`
	Created    time.Time           `json:"created"`
	LastUsed   time.Time           `json:"last_used,omitzero"`
	Credential webauthn.Credential `json:"credential"`
}

var passkeysMu sync.Mutex

func (a *App) passkeys(ctx context.Context) []storedPasskey {
	raw, _ := a.Events.Get(ctx, passkeysKey)
	var list []storedPasskey
	json.Unmarshal([]byte(raw), &list)
	return list
}

func (a *App) savePasskeys(ctx context.Context, list []storedPasskey) {
	b, _ := json.Marshal(list)
	a.Events.Put(ctx, passkeysKey, string(b))
}

// handleOf is the random user handle a person's passkeys carry, so the
// authenticator never holds their id or name as the account key.
func (a *App) handleOf(ctx context.Context, person string) []byte {
	raw, _ := a.Events.Get(ctx, passkeyHandles)
	m := map[string]string{}
	json.Unmarshal([]byte(raw), &m)
	if h, ok := m[person]; ok {
		b, _ := hex.DecodeString(h)
		return b
	}
	b := make([]byte, 32)
	rand.Read(b)
	m[person] = hex.EncodeToString(b)
	out, _ := json.Marshal(m)
	a.Events.Put(ctx, passkeyHandles, string(out))
	return b
}

func (a *App) personOfHandle(ctx context.Context, handle []byte) (string, bool) {
	raw, _ := a.Events.Get(ctx, passkeyHandles)
	m := map[string]string{}
	json.Unmarshal([]byte(raw), &m)
	h := hex.EncodeToString(handle)
	for p, v := range m {
		if v == h {
			return p, true
		}
	}
	return "", false
}

// passkeyUser is a person as WebAuthn sees them.
type passkeyUser struct {
	handle []byte
	name   string
	creds  []webauthn.Credential
}

func (u passkeyUser) WebAuthnID() []byte                         { return u.handle }
func (u passkeyUser) WebAuthnName() string                       { return u.name }
func (u passkeyUser) WebAuthnDisplayName() string                { return u.name }
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func (a *App) passkeyUser(ctx context.Context, person, rpID string) passkeyUser {
	name := "Pimpo"
	if acc, ok := a.admin(ctx); ok && person == people.OwnerID {
		name = acc.Name
	}
	if person != people.OwnerID {
		if p, err := a.People.Get(ctx, person); err == nil {
			name = p.Name
		}
	}
	u := passkeyUser{handle: a.handleOf(ctx, person), name: name}
	for _, k := range a.passkeys(ctx) {
		if k.Person == person && k.RPID == rpID {
			u.creds = append(u.creds, k.Credential)
		}
	}
	return u
}

var errPasskeyAddress = errors.New("passkeys work where Pimpo is opened by name: on this computer at localhost, or at an https address; not at a bare IP address")

// relyingParty is the WebAuthn configuration for the address the browser
// shows. The origin must be secure (https, or localhost) and match the
// address the request came to.
func (a *App) relyingParty(r *http.Request) (*webauthn.WebAuthn, string, error) {
	o, err := url.Parse(r.Header.Get("Origin"))
	if err != nil || o.Host == "" {
		return nil, "", errPasskeyAddress
	}
	host := o.Hostname()
	if net.ParseIP(host) != nil || (o.Scheme != "https" && host != "localhost") {
		return nil, "", errPasskeyAddress
	}
	reqHost := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		reqHost = h
	}
	public, _ := a.Events.Get(r.Context(), "public_url")
	pu, _ := url.Parse(public)
	if !strings.EqualFold(reqHost, host) && (pu == nil || !strings.EqualFold(pu.Hostname(), host)) && !(host == "localhost" && isLoopback(reqHost)) {
		return nil, "", errPasskeyAddress
	}
	w, err := webauthn.New(&webauthn.Config{RPID: host, RPDisplayName: "Pimpo", RPOrigins: []string{o.Scheme + "://" + o.Host}})
	return w, host, err
}

func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

// ceremonies hold what the browser must answer, for a few minutes.
type ceremony struct {
	session webauthn.SessionData
	person  string // for adding a passkey; empty for signing in
	name    string
	rpID    string
	expires time.Time
}

var ceremonies = struct {
	sync.Mutex
	m map[string]ceremony
}{m: map[string]ceremony{}}

func putCeremony(c ceremony) string {
	ceremonies.Lock()
	defer ceremonies.Unlock()
	now := time.Now()
	for k, v := range ceremonies.m {
		if now.After(v.expires) {
			delete(ceremonies.m, k)
		}
	}
	b := make([]byte, 16)
	rand.Read(b)
	key := hex.EncodeToString(b)
	c.expires = now.Add(ceremonyTTL)
	ceremonies.m[key] = c
	return key
}

func takeCeremony(key string) (ceremony, bool) {
	ceremonies.Lock()
	defer ceremonies.Unlock()
	c, ok := ceremonies.m[key]
	delete(ceremonies.m, key)
	return c, ok && time.Now().Before(c.expires)
}

type passkeyView struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Address  string    `json:"address"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used,omitzero"`
}

func (a *App) passkeyRoutes() {
	// A person's own passkeys; nobody sees anyone else's.
	a.Server.Handle("GET /api/passkeys", func(w http.ResponseWriter, r *http.Request) {
		out := []passkeyView{}
		for _, k := range a.passkeys(r.Context()) {
			if mine(r.Context(), k.Person) {
				out = append(out, passkeyView{k.ID, k.Name, k.RPID, k.Created, k.LastUsed})
			}
		}
		server.WriteJSON(w, 200, out)
	})
	a.Server.Handle("POST /api/passkeys/begin", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var req struct {
			Name string `json:"name"`
		}
		server.Decode(r, &req)
		wa, rpID, err := a.relyingParty(r)
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
			return
		}
		person := people.Norm(people.From(ctx))
		u := a.passkeyUser(ctx, person, rpID)
		var exclude []protocol.CredentialDescriptor
		for _, c := range u.creds {
			exclude = append(exclude, c.Descriptor())
		}
		opts, session, err := wa.BeginRegistration(u,
			webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
			webauthn.WithExclusions(exclude))
		if err != nil {
			server.WriteError(w, err)
			return
		}
		name := clip(strings.TrimSpace(req.Name), 60)
		if name == "" {
			name = "Passkey"
		}
		key := putCeremony(ceremony{session: *session, person: person, name: name, rpID: rpID})
		server.WriteJSON(w, 200, map[string]any{"key": key, "options": opts})
	})
	a.Server.Handle("POST /api/passkeys/finish", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		c, ok := takeCeremony(r.URL.Query().Get("key"))
		if !ok || c.person != people.Norm(people.From(ctx)) {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "that took too long; try again"})
			return
		}
		wa, rpID, err := a.relyingParty(r)
		if err != nil || rpID != c.rpID {
			server.WriteError(w, server.StatusError{Status: 400, Msg: errPasskeyAddress.Error()})
			return
		}
		cred, err := wa.FinishRegistration(a.passkeyUser(ctx, c.person, rpID), c.session, r)
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "the passkey was not accepted: " + err.Error()})
			return
		}
		k := storedPasskey{ID: base64.RawURLEncoding.EncodeToString(cred.ID), Person: c.person, Name: c.name, RPID: rpID, Created: time.Now().UTC(), Credential: *cred}
		passkeysMu.Lock()
		a.savePasskeys(ctx, append(a.passkeys(ctx), k))
		passkeysMu.Unlock()
		a.Events.Append(ctx, "passkey.added", actor(ctx), map[string]string{"id": k.ID, "name": k.Name, "person": c.person})
		server.WriteJSON(w, 200, passkeyView{k.ID, k.Name, k.RPID, k.Created, k.LastUsed})
	})
	a.Server.Handle("DELETE /api/passkeys/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		passkeysMu.Lock()
		list := a.passkeys(ctx)
		kept := list[:0]
		found := false
		for _, k := range list {
			if k.ID == r.PathValue("id") && mine(ctx, k.Person) {
				found = true
				continue
			}
			kept = append(kept, k)
		}
		a.savePasskeys(ctx, kept)
		passkeysMu.Unlock()
		if !found {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such passkey"})
			return
		}
		a.Events.Append(ctx, "passkey.removed", actor(ctx), map[string]string{"id": r.PathValue("id"), "person": people.From(ctx)})
		server.WriteJSON(w, 200, map[string]string{"removed": r.PathValue("id")})
	})

	// Signing in: the browser offers the passkeys it has for this address.
	a.Server.HandlePublic("POST /auth/passkey/begin", func(w http.ResponseWriter, r *http.Request) {
		wa, rpID, err := a.relyingParty(r)
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
			return
		}
		opts, session, err := wa.BeginDiscoverableLogin()
		if err != nil {
			server.WriteError(w, err)
			return
		}
		server.WriteJSON(w, 200, map[string]any{"key": putCeremony(ceremony{session: *session, rpID: rpID}), "options": opts})
	})
	a.Server.HandlePublic("POST /auth/passkey/finish", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		c, ok := takeCeremony(r.URL.Query().Get("key"))
		if !ok || c.person != "" {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "that took too long; try again"})
			return
		}
		wa, rpID, err := a.relyingParty(r)
		if err != nil || rpID != c.rpID {
			server.WriteError(w, server.StatusError{Status: 400, Msg: errPasskeyAddress.Error()})
			return
		}
		var who string
		user, cred, err := wa.FinishPasskeyLogin(func(rawID, handle []byte) (webauthn.User, error) {
			p, ok := a.personOfHandle(ctx, handle)
			if !ok {
				return nil, errors.New("unknown passkey")
			}
			if p != people.OwnerID {
				if _, err := a.People.Get(ctx, p); err != nil {
					return nil, errors.New("that person is no longer in the house")
				}
			}
			who = p
			return a.passkeyUser(ctx, p, rpID), nil
		}, c.session, r)
		if err != nil || user == nil {
			server.WriteJSON(w, 401, map[string]string{"error": "that passkey does not open this Pimpo"})
			return
		}
		id := base64.RawURLEncoding.EncodeToString(cred.ID)
		name := "Passkey"
		passkeysMu.Lock()
		list := a.passkeys(ctx)
		for i := range list {
			if list[i].ID == id && list[i].Person == who {
				list[i].Credential.Authenticator.SignCount = cred.Authenticator.SignCount
				list[i].LastUsed = time.Now().UTC()
				name = list[i].Name
			}
		}
		a.savePasskeys(ctx, list)
		passkeysMu.Unlock()
		token := a.newSession(ctx, who, "Passkey · "+name)
		server.SetSession(w, r, token)
		a.Events.Append(ctx, "passkey.signed_in", "human:"+who, map[string]string{"id": id, "person": who})
		server.WriteJSON(w, 200, map[string]string{"state": "signed_in"})
	})
}

// newSession opens a session for a person, kept like a paired device so it
// shows in the list and is revoked the same way; it expires sooner.
func (a *App) newSession(ctx context.Context, person, name string) string {
	b := make([]byte, 24)
	rand.Read(b)
	token := hex.EncodeToString(b)
	devicesMu.Lock()
	defer devicesMu.Unlock()
	now := time.Now()
	list := append(a.devices(ctx), Device{ID: hex.EncodeToString(b[:4]), Name: name, Hash: hashToken(token), Created: now, LastSeen: now, Person: personField(person), Session: true})
	a.saveDevices(ctx, list)
	return token
}
