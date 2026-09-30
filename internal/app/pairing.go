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
	// KeyHash is the phone's key for automations (iOS Shortcuts, Tasker):
	// it can only report phone events, never open the app.
	KeyHash string `json:"key_hash,omitempty"`
}

const devicesKey = "devices"

// deviceIdle is how long a paired device may go unused before it has to
// be paired again.
const deviceIdle = 180 * 24 * time.Hour

var devicesMu sync.Mutex

func hashToken(t string) string { s := sha256.Sum256([]byte(t)); return hex.EncodeToString(s[:]) }

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
			// drawer should not open Pimpo forever.
			last := d.LastSeen
			if last.IsZero() {
				last = d.Created
			}
			if time.Since(last) > deviceIdle {
				return "", false
			}
			person := people.Norm(d.Person)
			if person != people.OwnerID && a.People != nil {
				if _, err := a.People.Get(ctx, person); err != nil {
					return "", false
				}
			}
			if time.Since(d.LastSeen) > time.Hour {
				list[i].LastSeen = time.Now()
				a.saveDevices(ctx, list)
			}
			return person, true
		}
	}
	return "", false
}

func (a *App) pairingRoutes() {
	a.Server.Device = a.deviceValid
	a.Server.Handle("GET /api/pairing", a.getPairing)
	a.Server.Handle("POST /api/pairing", a.setPairing)
	a.Server.Handle("DELETE /api/devices/{id}", a.revokeDevice)
}

type deviceView struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Person   string    `json:"person"`
	Created  time.Time `json:"created"`
	LastSeen time.Time `json:"last_seen,omitzero"`
}

func (a *App) getPairing(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	base, _ := a.Events.Get(r.Context(), "public_url")
	views := []deviceView{}
	for _, d := range a.devices(r.Context()) {
		views = append(views, deviceView{d.ID, d.Name, people.Norm(d.Person), d.Created, d.LastSeen})
	}
	server.WriteJSON(w, 200, map[string]any{"base": base, "devices": views})
}

// setPairing saves the public address and, when a device name is given,
// pairs a new device: the answer carries its link once and never again.
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
		b := make([]byte, 24)
		rand.Read(b)
		token := hex.EncodeToString(b)
		id := hex.EncodeToString(b[:4])
		devicesMu.Lock()
		list := append(a.devices(ctx), Device{ID: id, Name: name, Hash: hashToken(token), Created: time.Now(), Person: personField(person)})
		err := a.saveDevices(ctx, list)
		devicesMu.Unlock()
		if err != nil {
			server.WriteError(w, err)
			return
		}
		a.Events.Append(ctx, "device.paired", "human:owner", map[string]string{"id": id, "name": name, "person": person})
		out["id"], out["link"] = id, primary+"/auth?token="+url.QueryEscape(token)
		// With both, the phone tries the home address first and falls back
		// to the public link; the token works on either.
		if home != "" && home != primary {
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
	ctx := r.Context()
	id := r.PathValue("id")
	devicesMu.Lock()
	list := a.devices(ctx)
	kept := list[:0]
	for _, d := range list {
		if d.ID != id {
			kept = append(kept, d)
		}
	}
	found := len(kept) != len(list)
	err := a.saveDevices(ctx, kept)
	devicesMu.Unlock()
	if !found {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such device"})
		return
	}
	if err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(ctx, "device.revoked", "human:owner", map[string]string{"id": id})
	server.WriteJSON(w, 200, map[string]string{"revoked": id})
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
}

func private(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()))
}
