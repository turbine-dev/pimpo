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

	"github.com/denerFernandes/zodim/internal/event"
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
	s.Events.Append(context.Background(), "routine.ran", "routine:brief", map[string]string{"outcome": "ok"})
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
