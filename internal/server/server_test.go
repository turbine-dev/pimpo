package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/people"
)

func newTest(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	store, err := event.Open(filepath.Join(t.TempDir(), "v.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	s := New(store, "tok")
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	return s, ts
}

func TestAPIRequiresTheSession(t *testing.T) {
	_, ts := newTest(t)
	resp, _ := http.Get(ts.URL + "/api/events")
	if resp.StatusCode != 401 {
		t.Fatalf("anonymous got %d", resp.StatusCode)
	}
	resp, _ = http.Get(ts.URL + "/auth?token=wrong")
	if resp.StatusCode != 401 {
		t.Fatalf("bad token login got %d", resp.StatusCode)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, _ = client.Get(ts.URL + "/auth?token=tok")
	if resp.StatusCode != 303 || len(resp.Cookies()) != 1 || !resp.Cookies()[0].HttpOnly {
		t.Fatalf("login %d %+v", resp.StatusCode, resp.Cookies())
	}
	req, _ := http.NewRequest("GET", ts.URL+"/api/events", nil)
	req.AddCookie(resp.Cookies()[0])
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 200 {
		t.Fatalf("with cookie got %d", resp.StatusCode)
	}
}

func TestEventsAndVerify(t *testing.T) {
	s, ts := newTest(t)
	s.Visible = func(person string, e event.Event) bool { return person == "owner" && e.Actor == "routine:brief" }
	s.Events.Append(context.Background(), "routine.ran", "routine:brief", map[string]string{"outcome": "ok"})
	s.Events.Append(context.Background(), "routine.ran", "routine:someone-elses", map[string]string{"outcome": "ok"})
	get := func(path string, v any) {
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer tok")
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("%s: %v %v", path, err, resp)
		}
		json.NewDecoder(resp.Body).Decode(v)
	}
	var evs []event.Event
	get("/api/events?types=routine.ran", &evs)
	if len(evs) != 1 {
		t.Fatalf("events %+v", evs)
	}
	var v map[string]any
	get("/api/events/verify", &v)
	if v["intact"] != true {
		t.Fatalf("verify %+v", v)
	}
}

func TestUIFallsBackToIndex(t *testing.T) {
	_, ts := newTest(t)
	resp, _ := http.Get(ts.URL + "/receipts/42")
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("spa route %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	resp, _ = http.Get(ts.URL + "/api/nope")
	if resp.StatusCode != 404 {
		t.Fatalf("unknown api %d", resp.StatusCode)
	}
}

func TestWebSocketStreamsNewEvents(t *testing.T) {
	s, ts := newTest(t)
	ctx := context.Background()
	c, _, err := websocket.Dial(ctx, strings.Replace(ts.URL, "http", "ws", 1)+"/api/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer tok"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	// The subscription starts when the handler runs; retry the append until it is seen.
	got := make(chan event.Event, 1)
	go func() {
		_, b, err := c.Read(ctx)
		if err == nil {
			var e event.Event
			json.Unmarshal(b, &e)
			got <- e
		}
	}()
	for {
		s.Events.Append(ctx, "ping", "system", map[string]int{})
		select {
		case e := <-got:
			if e.Type != "ping" {
				t.Fatalf("got %+v", e)
			}
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// A device acts for its person, who reaches only the routes opened to them.
func TestDevicesActForTheirPerson(t *testing.T) {
	s, ts := newTest(t)
	s.Device = func(tok string) (string, bool) { return "ana", tok == "ana-token" }
	s.Allow = func(pattern, person string) bool { return pattern == "GET /api/mine" && person == "ana" }
	var seen string
	s.Handle("GET /api/mine", func(w http.ResponseWriter, r *http.Request) { seen = people.From(r.Context()) })
	s.Handle("GET /api/admin", func(w http.ResponseWriter, r *http.Request) { seen = "admin" })
	call := func(tok, path string) int {
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if call("ana-token", "/api/mine") != 200 || seen != "ana" {
		t.Fatalf("Ana's request acted for %q", seen)
	}
	if call("ana-token", "/api/admin") != 403 || seen == "admin" {
		t.Fatal("Ana reached a route not opened to her")
	}
	if call("tok", "/api/admin") != 200 || seen != "admin" {
		t.Fatal("the owner was kept out")
	}
	if call("wrong", "/api/mine") != 401 {
		t.Fatal("a stranger got in")
	}
}

// The session cookie is Secure over https and only left without it on
// plain http, where browsers would drop a Secure cookie.
func TestSessionCookieIsSecureOverHTTPS(t *testing.T) {
	for _, c := range []struct {
		proto  string
		secure bool
	}{{"https", true}, {"", false}} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/auth?token=tok", nil)
		if c.proto != "" {
			r.Header.Set("X-Forwarded-Proto", c.proto)
		}
		SetSession(w, r, "tok")
		got := w.Result().Cookies()
		if len(got) != 1 || got[0].Secure != c.secure || !got[0].HttpOnly {
			t.Fatalf("proto %q: %+v", c.proto, got)
		}
	}
}
