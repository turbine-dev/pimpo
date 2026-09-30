package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
)

// Phones and other computers open Pimpo through an address the owner
// exposes, such as `tailscale serve`. Each paired device gets its own
// token, kept only as a hash, so one lost phone can be cut off without
// touching the others.

type Device struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Hash     string    `json:"hash"`
	Created  time.Time `json:"created"`
	LastSeen time.Time `json:"last_seen,omitzero"`
	// Person is who the device belongs to; empty is the owner (devices
	// paired before people had their own logins). It acts only for them.
	Person string `json:"person,omitempty"`
	// Shares are what this phone gives Pimpo (location, camera,
	// shortcuts), chosen on the phone itself.
	Shares []string `json:"shares,omitempty"`
	// Session marks a sign-in with a passkey rather than a paired device;
	// it expires sooner when unused.
	Session bool `json:"session,omitempty"`
	// KeyHash is the phone's key for automations (iOS Shortcuts, Tasker):
	// it can only report phone events, never open the app.
	KeyHash string `json:"key_hash,omitempty"`
	// Passkey is the passkey that opened this session; removing the
	// passkey signs it out.
	Passkey string `json:"passkey,omitempty"`
	// Invite marks a link made for someone else that nobody opened yet:
	// it opens nothing by itself, works once and only until Expires.
	Invite  bool      `json:"invite,omitempty"`
	Expires time.Time `json:"expires,omitzero"`
	// MiniApp marks a session the Telegram Mini App opened: it lasts
	// until Expires and opens only the Mini App's routes.
	MiniApp bool `json:"mini_app,omitempty"`
}

const devicesKey = "devices"

// deviceIdle is how long a paired device may go unused before it has to
// be paired again.
const deviceIdle = 180 * 24 * time.Hour

// inviteTTL is how long a link for someone else waits to be opened.
const inviteTTL = 15 * time.Minute

var devicesMu sync.Mutex

func hashToken(t string) string { s := sha256.Sum256([]byte(t)); return hex.EncodeToString(s[:]) }

// newToken is a fresh secret and a separate id to name it by, so the id
// shown in lists and events gives nothing of the secret away.
func newToken() (token, id string) {
	b := make([]byte, 28)
	rand.Read(b)
	return hex.EncodeToString(b[:24]), hex.EncodeToString(b[24:])
}

// expired says whether a device no longer opens anything: an invite past
// its time, or a device unused for too long.
func (d Device) expired(now time.Time) bool {
	if d.Invite || d.MiniApp {
		return now.After(d.Expires)
	}
	last := d.LastSeen
	if last.IsZero() {
		last = d.Created
	}
	idle := deviceIdle
	if d.Session {
		idle = passkeyIdle
	}
	return now.Sub(last) > idle
}

func (a *App) devices(ctx context.Context) []Device {
	raw, _ := a.Events.Get(ctx, devicesKey)
	var list []Device
	json.Unmarshal([]byte(raw), &list)
	return list
}

func (a *App) saveDevices(ctx context.Context, list []Device) error {
	b, _ := json.Marshal(list)
	return a.Events.Put(ctx, devicesKey, string(b))
}

// deviceValid is the server's check for device tokens: whose device it is.
// A device of someone no longer in the house opens nothing.
func (a *App) deviceValid(token string) (string, bool) {
	ctx := context.Background()
	h := hashToken(token)
	devicesMu.Lock()
	defer devicesMu.Unlock()
	list := a.devices(ctx)
	for i, d := range list {
		if subtle.ConstantTimeCompare([]byte(d.Hash), []byte(h)) == 1 {
			// A device unused for months is signed out: a phone left in a
			// drawer should not open Pimpo forever. An invite only opens
			// a session of its own, at /auth.
			if d.Invite || d.expired(time.Now()) || !a.inHouse(ctx, d.Person) {
				return "", false
			}
			person := people.Norm(d.Person)
			if time.Since(d.LastSeen) > time.Hour {
				list[i].LastSeen = time.Now()
				a.saveDevices(ctx, list)
			}
			return person, true
		}
	}
	return "", false
}

// inHouse says whether a device's person still lives here.
func (a *App) inHouse(ctx context.Context, person string) bool {
	person = people.Norm(person)
	if person == people.OwnerID || a.People == nil {
		return true
	}
	_, err := a.People.Get(ctx, person)
	return err == nil
}

// redeem spends an invite: the link opens one session, with a token of
// its own, and is no use afterwards. The person sees the new device in
// their activity, so they notice one they did not add.
func (a *App) redeem(invite string) (string, bool) {
	ctx := context.Background()
	h := hashToken(invite)
	devicesMu.Lock()
	list := a.devices(ctx)
	now := time.Now()
	var found *Device
	kept := list[:0]
	for _, d := range list {
		if d.Invite && d.expired(now) {
			continue
		}
		kept = append(kept, d)
		if d.Invite && subtle.ConstantTimeCompare([]byte(d.Hash), []byte(h)) == 1 {
			found = &kept[len(kept)-1]
		}
	}
	if found == nil || !a.inHouse(ctx, found.Person) {
		if len(kept) != len(list) {
			a.saveDevices(ctx, kept)
		}
		devicesMu.Unlock()
		return "", false
	}
	token, _ := newToken()
	found.Hash, found.Invite, found.Expires, found.Created, found.LastSeen = hashToken(token), false, time.Time{}, now, now
	d := *found
	err := a.saveDevices(ctx, kept)
	devicesMu.Unlock()
	if err != nil {
		return "", false
	}
	a.Events.Append(ctx, "device.added", "device:"+d.ID, map[string]string{"id": d.ID, "name": d.Name, "person": people.Norm(d.Person)})
	return token, true
}

func (a *App) pairingRoutes() {
	a.Server.Device = a.deviceValid
	a.Server.Exchange = a.redeem
	a.Server.TrustedOrigin = a.trustedOrigin
	a.Server.Handle("GET /api/pairing", a.getPairing)
	a.Server.Handle("POST /api/pairing", a.setPairing)
	a.Server.Handle("DELETE /api/devices/{id}", a.revokeDevice)
	a.myDevicesRoutes()
}

// trustedOrigin is the public address the owner gave, which a proxy in
// front may not pass on as the Host.
func (a *App) trustedOrigin(origin string) bool {
	base, _ := a.Events.Get(context.Background(), "public_url")
	return base != "" && strings.EqualFold(strings.TrimSuffix(base, "/"), origin)
}

type deviceView struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Person   string    `json:"person"`
	Created  time.Time `json:"created"`
	LastSeen time.Time `json:"last_seen,omitzero"`
	// Pending is an invite nobody opened yet.
	Pending bool `json:"pending,omitempty"`
}

func (a *App) getPairing(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	base, _ := a.Events.Get(r.Context(), "public_url")
	views := []deviceView{}
	now := time.Now()
	for _, d := range a.devices(r.Context()) {
		if d.Invite && d.expired(now) {
			continue
		}
		views = append(views, deviceView{d.ID, d.Name, people.Norm(d.Person), d.Created, d.LastSeen, d.Invite})
	}
	server.WriteJSON(w, 200, map[string]any{"base": base, "devices": views})
}

// setPairing saves the public address and, when a device name is given,
// pairs a new device: the answer carries its link once and never again.
// A link for someone else is an invite: it works once, within minutes,
// and opens a session the owner never holds.
func (a *App) setPairing(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	ctx := r.Context()
	var req struct {
		Base   string `json:"base"`
		Device string `json:"device"`
		// Person is whose device this is; empty is the owner's.
		Person string `json:"person"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	person := people.Norm(req.Person)
	if person != people.OwnerID {
		if _, err := a.People.Get(ctx, person); err != nil {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such person"})
			return
		}
	}
	base := strings.TrimSpace(req.Base)
	if base != "" {
		u, err := url.Parse(base)
		if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && private(u.Hostname()))) {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "use an https address; the token must not travel in the clear"})
			return
		}
		base = u.Scheme + "://" + u.Host
	}
	if err := a.Events.Put(ctx, "public_url", base); err != nil {
		server.WriteError(w, err)
		return
	}
	go a.syncMiniApp(context.WithoutCancel(ctx))
	out := map[string]string{"base": base}
	home := ""
	if a.LAN != nil {
		home = a.LAN.URL()
	}
	primary := base
	if primary == "" {
		primary = home
	}
	if name := strings.TrimSpace(req.Device); name != "" && primary != "" {
		token, id := newToken()
		d := Device{ID: id, Name: name, Hash: hashToken(token), Created: time.Now(), Person: personField(person)}
		invite := person != people.OwnerID
		if invite {
			d.Invite, d.Expires = true, d.Created.Add(inviteTTL)
		}
		devicesMu.Lock()
		list := append(a.devices(ctx), d)
		err := a.saveDevices(ctx, list)
		devicesMu.Unlock()
		if err != nil {
			server.WriteError(w, err)
			return
		}
		a.Events.Append(ctx, "device.paired", "human:owner", map[string]string{"id": id, "name": name, "person": person})
		out["id"], out["link"] = id, primary+"/auth?token="+url.QueryEscape(token)
		// With both, the phone tries the home address first and falls back
		// to the public link; the token works on either. An invite works
		// once, at one address, so it carries only one.
		if invite {
			out["expires"] = d.Expires.UTC().Format(time.RFC3339)
		} else if home != "" && home != primary {
			out["home"] = home + "/auth?token=" + url.QueryEscape(token)
			out["link"] += "#home=" + url.QueryEscape(home)
		}
	}
	server.WriteJSON(w, 200, out)
}

// personField keeps the owner's devices as before: no person.
func personField(p string) string {
	if p == people.OwnerID {
		return ""
	}
	return p
}

func (a *App) revokeDevice(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	a.dropDevice(w, r, func(Device) bool { return true })
}

// dropDevice revokes the device in the path when may allows it; any other
// device does not exist for the caller.
func (a *App) dropDevice(w http.ResponseWriter, r *http.Request, may func(Device) bool) {
	ctx := r.Context()
	id := r.PathValue("id")
	devicesMu.Lock()
	list := a.devices(ctx)
	kept := list[:0]
	var gone *Device
	for _, d := range list {
		if d.ID == id && may(d) {
			gone = &d
			continue
		}
		kept = append(kept, d)
	}
	var err error
	if gone != nil {
		err = a.saveDevices(ctx, kept)
	}
	devicesMu.Unlock()
	if gone == nil {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such device"})
		return
	}
	if err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(ctx, "device.revoked", actor(ctx), map[string]string{"id": id, "person": people.Norm(gone.Person)})
	server.WriteJSON(w, 200, map[string]string{"revoked": id})
}

type myDevice struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Created  time.Time `json:"created"`
	LastSeen time.Time `json:"last_seen,omitzero"`
	Session  bool      `json:"session,omitempty"`
	Pending  bool      `json:"pending,omitempty"`
	Current  bool      `json:"current,omitempty"`
}

// myDevices are the devices and passkey sessions of the person asking,
// and nobody else's: each person sees what opens their account and can
// sign any of it out.
func (a *App) myDevicesRoutes() {
	a.Server.Handle("GET /api/me/devices", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		current := hashToken(server.TokenOf(r))
		now := time.Now()
		out := []myDevice{}
		for _, d := range a.devices(ctx) {
			if !mine(ctx, d.Person) || d.expired(now) {
				continue
			}
			out = append(out, myDevice{d.ID, d.Name, d.Created, d.LastSeen, d.Session, d.Invite, d.Hash == current})
		}
		server.WriteJSON(w, 200, out)
	})
	a.Server.Handle("DELETE /api/me/devices/{id}", func(w http.ResponseWriter, r *http.Request) {
		a.dropDevice(w, r, func(d Device) bool { return mine(r.Context(), d.Person) })
	})
}

// forgetDevicesOf revokes every device of someone who left the house.
func (a *App) forgetDevicesOf(ctx context.Context, person string) {
	devicesMu.Lock()
	defer devicesMu.Unlock()
	list := a.devices(ctx)
	kept := list[:0]
	for _, d := range list {
		if people.Norm(d.Person) != people.Norm(person) {
			kept = append(kept, d)
		}
	}
	a.saveDevices(ctx, kept)
	passkeysMu.Lock()
	keys := a.passkeys(ctx)
	left := keys[:0]
	for _, k := range keys {
		if people.Norm(k.Person) != people.Norm(person) {
			left = append(left, k)
		}
	}
	a.savePasskeys(ctx, left)
	passkeysMu.Unlock()
}

func private(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()))
}
