package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"slices"
	"time"

	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
)

// Home-screen widgets on a phone read the widget feed with the phone's
// widget key (wk_…). The key is made on the phone, is kept only as a hash,
// reads only the widgets pinned to that phone, and dies with the device:
// it cannot open the app, change anything or read another widget.

const maxPins = 20

// widgetPhone is the paired phone whose widget key a request carries.
func (a *App) widgetPhone(r *http.Request) (Device, bool) {
	tok := server.TokenOf(r)
	if len(tok) < 4 || tok[:3] != "wk_" {
		return Device{}, false
	}
	h := hashToken(tok)
	for _, d := range a.devices(r.Context()) {
		if d.WidgetKeyHash != "" && subtle.ConstantTimeCompare([]byte(d.WidgetKeyHash), []byte(h)) == 1 {
			if d.Invite || d.expired(time.Now()) || !a.inHouse(r.Context(), d.Person) {
				return Device{}, false
			}
			return d, true
		}
	}
	return Device{}, false
}

func (a *App) widgetFeedRoutes() {
	// The phone pins a widget to its home screen, from the app on that
	// phone. The answer carries a fresh widget key, which replaces the old
	// one, for the phone's own widgets to read the feed with.
	a.Server.Handle("POST /api/phone/widgets", func(w http.ResponseWriter, r *http.Request) {
		d, ok := a.phoneOf(r, false)
		if !ok {
			server.WriteError(w, server.StatusError{Status: 403, Msg: "open this on the paired phone"})
			return
		}
		var req struct {
			Pin   string `json:"pin"`
			Unpin string `json:"unpin"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		ctx := people.With(r.Context(), people.Norm(d.Person))
		pins := slices.DeleteFunc(slices.Clone(d.Pins), func(id string) bool { return id == req.Unpin })
		if req.Pin != "" && !slices.Contains(pins, req.Pin) {
			if _, ok := a.anyWidget(ctx, req.Pin, false); !ok {
				server.WriteError(w, server.StatusError{Status: 404, Msg: "no such widget"})
				return
			}
			if len(pins) >= maxPins {
				server.WriteError(w, server.StatusError{Status: 400, Msg: "a phone keeps at most 20 widgets"})
				return
			}
			pins = append(pins, req.Pin)
		}
		b := make([]byte, 24)
		rand.Read(b)
		key := "wk_" + hex.EncodeToString(b)
		if err := a.updateDevice(r.Context(), d.ID, func(d *Device) { d.Pins = pins; d.WidgetKeyHash = hashToken(key) }); err != nil {
			server.WriteError(w, err)
			return
		}
		a.Events.Append(r.Context(), "phone.widgets", actor(r.Context()), map[string]any{"device": d.ID, "person": people.Norm(d.Person), "pins": len(pins)})
		server.WriteJSON(w, 200, map[string]any{"key": key, "pins": nonNil(pins)})
	})

	// The feed: the latest snapshot of each widget pinned to the phone, in
	// its person's name. A widget since removed, or no longer shared with
	// them, simply drops out.
	a.Server.HandlePublic("GET /api/widgets/feed", func(w http.ResponseWriter, r *http.Request) {
		d, ok := a.widgetPhone(r)
		if !ok {
			server.WriteJSON(w, 401, map[string]string{"error": "use the phone's widget key"})
			return
		}
		ctx := people.With(r.Context(), people.Norm(d.Person))
		out := []widgetView{}
		for _, id := range d.Pins {
			if v, ok := a.anyWidget(ctx, id, true); ok {
				out = append(out, v)
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		server.WriteJSON(w, 200, map[string]any{"at": time.Now().UTC(), "widgets": out})
	})
}
