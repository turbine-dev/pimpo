package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/connector"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/store"
)

// A paired phone is part of the agent: it reports arriving at and leaving
// a place, photos (of a bill, a receipt, a document) and shortcuts, and
// routines watch them through phone.arrivals, phone.photos and
// phone.shortcuts. Each is something the owner turns on on the phone
// itself; a phone that does not share its location cannot report one.
// Automations on the phone (iOS Shortcuts, Tasker) use the phone's own
// key, which only reports events and cannot open the app.

const (
	shareLocation  = "location"
	shareCamera    = "camera"
	shareShortcuts = "shortcuts"

	placesKey     = "phone.places"
	phoneKeep     = 14 * 24 * time.Hour
	maxPhoto      = 12 << 20
	defaultRadius = 150.0
)

var phoneShares = []string{shareLocation, shareCamera, shareShortcuts}

type Place struct {
	Name string `json:"name"`
	// Lat and Lon are optional: a place named only is reported by an
	// automation on the phone, not by the app.
	Lat    float64 `json:"lat,omitempty"`
	Lon    float64 `json:"lon,omitempty"`
	Radius float64 `json:"radius,omitempty"`
}

func (p Place) located() bool { return p.Lat != 0 || p.Lon != 0 }

func (a *App) places(ctx context.Context) []Place {
	raw, _ := a.Events.Get(ctx, placesKey)
	list := []Place{}
	json.Unmarshal([]byte(raw), &list)
	return list
}

func (a *App) placeNamed(ctx context.Context, name string) (Place, bool) {
	for _, p := range a.places(ctx) {
		if strings.EqualFold(p.Name, strings.TrimSpace(name)) {
			return p, true
		}
	}
	return Place{}, false
}

// meters between two points, by the haversine formula.
func meters(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	rad := math.Pi / 180
	dlat, dlon := (lat2-lat1)*rad, (lon2-lon1)*rad
	h := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return 2 * r * math.Asin(math.Sqrt(h))
}

// phoneOf is the paired phone a request comes from: by its login token,
// or by its automation key when keys is true.
func (a *App) phoneOf(r *http.Request, keys bool) (Device, bool) {
	tok := server.TokenOf(r)
	if tok == "" {
		return Device{}, false
	}
	h := hashToken(tok)
	for _, d := range a.devices(r.Context()) {
		if subtle.ConstantTimeCompare([]byte(d.Hash), []byte(h)) == 1 || (keys && d.KeyHash != "" && subtle.ConstantTimeCompare([]byte(d.KeyHash), []byte(h)) == 1) {
			return d, true
		}
	}
	return Device{}, false
}

func (a *App) updateDevice(ctx context.Context, id string, f func(*Device)) error {
	devicesMu.Lock()
	defer devicesMu.Unlock()
	list := a.devices(ctx)
	for i := range list {
		if list[i].ID == id {
			f(&list[i])
			return a.saveDevices(ctx, list)
		}
	}
	return server.StatusError{Status: 404, Msg: "no such device"}
}

func (a *App) phoneRoutes() {
	a.Server.Handle("GET /api/phone", func(w http.ResponseWriter, r *http.Request) {
		out := map[string]any{"places": a.places(r.Context()), "shares": phoneShares}
		if d, ok := a.phoneOf(r, false); ok {
			out["device"] = map[string]any{"id": d.ID, "name": d.Name, "shares": nonNil(d.Shares), "has_key": d.KeyHash != ""}
		}
		server.WriteJSON(w, 200, out)
	})
	// Only the phone itself chooses what it shares.
	a.Server.Handle("POST /api/phone/shares", func(w http.ResponseWriter, r *http.Request) {
		d, ok := a.phoneOf(r, false)
		if !ok {
			server.WriteError(w, server.StatusError{Status: 403, Msg: "open this on the paired phone"})
			return
		}
		var req struct{ Shares []string }
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		shares := []string{}
		for _, s := range req.Shares {
			if slices.Contains(phoneShares, s) && !slices.Contains(shares, s) {
				shares = append(shares, s)
			}
		}
		if err := a.updateDevice(r.Context(), d.ID, func(d *Device) { d.Shares = shares }); err != nil {
			server.WriteError(w, err)
			return
		}
		a.Events.Append(r.Context(), "phone.shares", "human:owner", map[string]any{"device": d.ID, "shares": shares})
		server.WriteJSON(w, 200, map[string]any{"shares": shares})
	})
	a.Server.Handle("POST /api/phone/key", func(w http.ResponseWriter, r *http.Request) {
		d, ok := a.phoneOf(r, false)
		if !ok {
			server.WriteError(w, server.StatusError{Status: 403, Msg: "open this on the paired phone"})
			return
		}
		b := make([]byte, 24)
		rand.Read(b)
		key := "pk_" + hex.EncodeToString(b)
		if err := a.updateDevice(r.Context(), d.ID, func(d *Device) { d.KeyHash = hashToken(key) }); err != nil {
			server.WriteError(w, err)
			return
		}
		a.Events.Append(r.Context(), "phone.key", "human:owner", map[string]string{"device": d.ID})
		server.WriteJSON(w, 200, map[string]string{"key": key})
	})
	a.Server.Handle("POST /api/phone/places", func(w http.ResponseWriter, r *http.Request) {
		if !ownerOnly(w, r) {
			return
		}
		var p Place
		if err := server.Decode(r, &p); err != nil {
			server.WriteError(w, err)
			return
		}
		p.Name = clip(strings.TrimSpace(p.Name), 40)
		if p.Name == "" || math.Abs(p.Lat) > 90 || math.Abs(p.Lon) > 180 {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "a place needs a name, and a valid position if any"})
			return
		}
		if p.located() && p.Radius == 0 {
			p.Radius = defaultRadius
		}
		p.Radius = min(max(p.Radius, 0), 5000)
		list := slices.DeleteFunc(a.places(r.Context()), func(x Place) bool { return strings.EqualFold(x.Name, p.Name) })
		list = append(list, p)
		b, _ := json.Marshal(list)
		a.Events.Put(r.Context(), placesKey, string(b))
		server.WriteJSON(w, 200, list)
	})
	a.Server.Handle("DELETE /api/phone/places/{name}", func(w http.ResponseWriter, r *http.Request) {
		if !ownerOnly(w, r) {
			return
		}
		list := slices.DeleteFunc(a.places(r.Context()), func(x Place) bool { return strings.EqualFold(x.Name, r.PathValue("name")) })
		b, _ := json.Marshal(list)
		a.Events.Put(r.Context(), placesKey, string(b))
		server.WriteJSON(w, 200, list)
	})

	// Events from the phone: the app, or an automation with the key.
	a.Server.HandlePublic("POST /api/phone/location", a.phoneEvent(shareLocation, a.phoneLocation))
	a.Server.HandlePublic("POST /api/phone/arrived", a.phoneEvent(shareLocation, a.phonePlace("arrived")))
	a.Server.HandlePublic("POST /api/phone/left", a.phoneEvent(shareLocation, a.phonePlace("left")))
	a.Server.HandlePublic("POST /api/phone/photo", a.phoneEvent(shareCamera, a.phonePhoto))
	a.Server.HandlePublic("POST /api/phone/shortcut", a.phoneEvent(shareShortcuts, a.phoneShortcut))
	a.Server.Handle("GET /api/phone/photos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !validID(id) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeFile(w, r, filepath.Join(a.Home, "phone", "photos", id+".jpg"))
	})
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func validID(id string) bool {
	if len(id) == 0 || len(id) > 40 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'f' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

type phoneHandler func(w http.ResponseWriter, r *http.Request, d Device) (map[string]any, error)

// phoneEvent accepts a report only from a paired phone that shares it.
func (a *App) phoneEvent(share string, h phoneHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := a.phoneOf(r, true)
		if !ok {
			server.WriteJSON(w, 401, map[string]string{"error": "use a paired phone, or its key for automations"})
			return
		}
		if !slices.Contains(d.Shares, share) {
			server.WriteError(w, server.StatusError{Status: 403, Msg: "this phone does not share " + share + "; turn it on in Pimpo on the phone"})
			return
		}
		out, err := h(w, r, d)
		if err != nil {
			server.WriteError(w, err)
			return
		}
		if out == nil {
			out = map[string]any{"ok": true}
		}
		go a.pokePhoneWatchers(context.WithoutCancel(r.Context()))
		server.WriteJSON(w, 200, out)
	}
}

func newPhoneID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// phoneLocation turns a position into arriving at or leaving the places
// that have one. The position itself is not kept.
func (a *App) phoneLocation(_ http.ResponseWriter, r *http.Request, d Device) (map[string]any, error) {
	ctx := r.Context()
	var req struct{ Lat, Lon, Accuracy float64 }
	if err := server.Decode(r, &req); err != nil {
		return nil, err
	}
	if req.Accuracy > 500 {
		return map[string]any{"ignored": "too imprecise"}, nil
	}
	key := "phone.at." + d.ID
	raw, _ := a.Events.Get(ctx, key)
	var at []string
	json.Unmarshal([]byte(raw), &at)
	var now []string
	changed := []string{}
	for _, p := range a.places(ctx) {
		if !p.located() {
			continue
		}
		inside := meters(req.Lat, req.Lon, p.Lat, p.Lon) <= p.Radius+min(req.Accuracy, 100)
		was := slices.Contains(at, p.Name)
		if inside {
			now = append(now, p.Name)
		}
		switch {
		case inside && !was:
			a.recordPlace(ctx, d, "arrived", p.Name)
			changed = append(changed, "arrived "+p.Name)
		case !inside && was:
			// Leaving needs a clear distance, so a jittery fix at the edge
			// does not come and go.
			if meters(req.Lat, req.Lon, p.Lat, p.Lon) > p.Radius*1.5 {
				a.recordPlace(ctx, d, "left", p.Name)
				changed = append(changed, "left "+p.Name)
			} else {
				now = append(now, p.Name)
			}
		}
	}
	b, _ := json.Marshal(now)
	a.Events.Put(ctx, key, string(b))
	return map[string]any{"at": nonNil(now), "changed": changed}, nil
}

func (a *App) phonePlace(kind string) phoneHandler {
	return func(_ http.ResponseWriter, r *http.Request, d Device) (map[string]any, error) {
		var req struct{ Place string }
		if err := server.Decode(r, &req); err != nil {
			return nil, err
		}
		p, ok := a.placeNamed(r.Context(), req.Place)
		if !ok {
			return nil, server.StatusError{Status: 404, Msg: "no place named " + req.Place + "; add it in Pimpo › Celular first"}
		}
		a.recordPlace(r.Context(), d, kind, p.Name)
		return nil, nil
	}
}

func (a *App) recordPlace(ctx context.Context, d Device, kind, place string) {
	a.Events.Append(ctx, "phone."+kind, "device:"+d.ID, map[string]string{"id": newPhoneID(), "place": place, "device": d.Name})
}

func (a *App) phonePhoto(_ http.ResponseWriter, r *http.Request, d Device) (map[string]any, error) {
	ctx := r.Context()
	if a.Home == "" {
		return nil, errors.New("photos need a data folder")
	}
	caption := clip(r.URL.Query().Get("caption"), 200)
	var img []byte
	var err error
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		r.Body = http.MaxBytesReader(nil, r.Body, maxPhoto+1<<20)
		if err := r.ParseMultipartForm(maxPhoto); err != nil {
			return nil, server.StatusError{Status: 400, Msg: "send the photo as a file under 12 MB"}
		}
		f, _, ferr := r.FormFile("photo")
		if ferr != nil {
			return nil, server.StatusError{Status: 400, Msg: "no photo in the form"}
		}
		defer f.Close()
		img, err = io.ReadAll(io.LimitReader(f, maxPhoto+1))
		if c := r.FormValue("caption"); c != "" {
			caption = clip(c, 200)
		}
	} else {
		img, err = io.ReadAll(io.LimitReader(r.Body, maxPhoto+1))
	}
	if err != nil || len(img) == 0 || len(img) > maxPhoto {
		return nil, server.StatusError{Status: 400, Msg: "send a photo under 12 MB"}
	}
	if ct := http.DetectContentType(img); !strings.HasPrefix(ct, "image/") {
		return nil, server.StatusError{Status: 400, Msg: "that is not a photo"}
	}
	id := newPhoneID()
	dir := filepath.Join(a.Home, "phone", "photos")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, id+".jpg"), img, 0o600); err != nil {
		return nil, err
	}
	text, note := "", ""
	if a.Channel != nil && a.Channel.ReadPhoto != nil {
		rctx, cancel := context.WithTimeout(ctx, time.Minute)
		t, err := a.Channel.ReadPhoto(rctx, img)
		cancel()
		if err != nil {
			note = err.Error()
		}
		text = clip(t, 4000)
	}
	a.Events.Append(ctx, "phone.photo", "device:"+d.ID, map[string]string{"id": id, "device": d.Name, "caption": caption, "text": text, "note": note})
	return map[string]any{"id": id, "text": text, "note": note}, nil
}

func (a *App) phoneShortcut(_ http.ResponseWriter, r *http.Request, d Device) (map[string]any, error) {
	var req struct{ Name, Text string }
	if err := server.Decode(r, &req); err != nil {
		return nil, err
	}
	name := clip(strings.TrimSpace(req.Name), 60)
	if name == "" {
		return nil, server.StatusError{Status: 400, Msg: "a shortcut needs a name"}
	}
	a.Events.Append(r.Context(), "phone.shortcut", "device:"+d.ID, map[string]string{"id": newPhoneID(), "name": name, "text": clip(req.Text, 2000), "device": d.Name})
	return nil, nil
}

// pokePhoneWatchers checks at once the routines that watch the phone, so
// arriving home does not wait for the next poll.
func (a *App) pokePhoneWatchers(ctx context.Context) {
	if a.Scheduler == nil || a.Store == nil {
		return
	}
	routines, err := a.Store.Routines(ctx)
	if err != nil {
		return
	}
	for _, r := range routines {
		if w := r.Watch(); w != nil && r.State == store.RoutineActive && strings.HasPrefix(w.Capability, "phone.") {
			a.Scheduler.Poll(ctx, r.ID)
		}
	}
}

// phoneCap lets routines and the agent read what the phones reported.
type phoneCap struct{ a *App }

func (phoneCap) Capabilities() []string {
	return []string{"phone.arrivals", "phone.photos", "phone.shortcuts"}
}

func (c phoneCap) Call(ctx context.Context, name, _ string, args any) (any, error) {
	var in struct {
		Place string `json:"place"`
		Name  string `json:"name"`
	}
	if err := connector.Args(args, &in); err != nil {
		return nil, err
	}
	types := map[string][]string{
		"phone.arrivals":  {"phone.arrived", "phone.left"},
		"phone.photos":    {"phone.photo"},
		"phone.shortcuts": {"phone.shortcut"},
	}[name]
	evs, err := c.a.Events.List(ctx, event.Query{Types: types, Newest: true, Limit: 200})
	if err != nil {
		return nil, err
	}
	since := time.Now().Add(-phoneKeep)
	out := []map[string]any{}
	for _, e := range evs {
		if e.Time.Before(since) || len(out) == 50 {
			break
		}
		var d map[string]string
		e.Decode(&d)
		item := map[string]any{"id": d["id"], "time": e.Time.Format(time.RFC3339), "device": d["device"]}
		switch name {
		case "phone.arrivals":
			if in.Place != "" && !strings.EqualFold(in.Place, d["place"]) {
				continue
			}
			item["place"], item["kind"] = d["place"], strings.TrimPrefix(e.Type, "phone.")
		case "phone.photos":
			item["caption"], item["text"] = d["caption"], d["text"]
			if d["note"] != "" {
				item["note"] = d["note"]
			}
		case "phone.shortcuts":
			if in.Name != "" && !strings.EqualFold(in.Name, d["name"]) {
				continue
			}
			item["name"], item["text"] = d["name"], d["text"]
		}
		out = append(out, item)
	}
	return out, nil
}
