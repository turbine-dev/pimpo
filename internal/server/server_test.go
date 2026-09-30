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

// A page elsewhere, such as another port of this computer, cannot make the
// browser change anything with its cookie; Pimpo's own page and clients
// that send their token can.
func TestCrossOriginWritesAreRefused(t *testing.T) {
	s, ts := newTest(t)
	s.Handle("POST /api/thing", func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		if err := Decode(r, &v); err != nil {
			WriteError(w, err)
			return
		}
		WriteJSON(w, 200, v)
	})
	s.TrustedOrigin = func(o string) bool { return o == "https://pimpo.example.ts.net" }
	self := ts.URL
	post := func(origin, site, ctype string, bearer bool) int {
		req, _ := http.NewRequest("POST", ts.URL+"/api/thing", strings.NewReader(`{"a":1}`))
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if site != "" {
			req.Header.Set("Sec-Fetch-Site", site)
		}
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		if bearer {
			req.Header.Set("Authorization", "Bearer tok")
		} else {
			req.AddCookie(&http.Cookie{Name: cookie, Value: "tok"})
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for _, c := range []struct {
		origin, site, ctype string
		bearer              bool
		want                int
	}{
		{self, "same-origin", "application/json", false, 200},
		{"https://pimpo.example.ts.net", "", "application/json", false, 200},
		{"", "", "application/json", false, 200}, // not a browser
		{"http://127.0.0.1:1", "", "application/json", false, 403},
		{"http://127.0.0.1:1", "", "text/plain", false, 403},
		{"null", "", "application/json", false, 403},
		{"", "cross-site", "application/json", false, 403},
		{"", "same-site", "application/json", false, 403},
		{self, "", "text/plain", false, 415},
		{self, "", "", false, 415},
		{self, "", "application/json; charset=utf-8", false, 200},
		{"http://127.0.0.1:1", "cross-site", "", true, 200}, // its own token
	} {
		if got := post(c.origin, c.site, c.ctype, c.bearer); got != c.want {
			t.Errorf("origin %q site %q type %q bearer %v: %d, want %d", c.origin, c.site, c.ctype, c.bearer, got, c.want)
		}
	}
}

// Every answer forbids framing and loading from elsewhere.
func TestSecurityHeaders(t *testing.T) {
	_, ts := newTest(t)
	for _, path := range []string{"/", "/api/health"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		csp := resp.Header.Get("Content-Security-Policy")
		for _, want := range []string{"frame-ancestors 'none'", "default-src 'self'", "script-src 'self'", "object-src 'none'"} {
			if !strings.Contains(csp, want) {
				t.Errorf("%s: policy %q lacks %q", path, csp, want)
			}
		}
		if strings.Contains(csp, "script-src 'self' 'unsafe") {
			t.Errorf("%s: scripts may run inline: %q", path, csp)
		}
		if resp.Header.Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: framing allowed", path)
		}
	}
	// Only the Mini App page may be framed, only by Telegram Web, and only
	// it may load Telegram's script.
	resp, err := http.Get(ts.URL + MiniAppPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	csp := resp.Header.Get("Content-Security-Policy")
	for _, want := range []string{"frame-ancestors https://web.telegram.org", "script-src 'self' https://telegram.org;", "object-src 'none'", "default-src 'self'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("mini app: policy %q lacks %q", csp, want)
		}
	}
	if !strings.HasSuffix(csp, "frame-ancestors https://web.telegram.org") || resp.Header.Get("X-Frame-Options") != "" {
		t.Errorf("mini app cannot be framed by Telegram Web: %q %q", csp, resp.Header.Get("X-Frame-Options"))
	}
	for _, path := range []string{"/tg/app/other", "/tg", "/api/health", "/tg/app.js"} {
		resp, _ := http.Get(ts.URL + path)
		resp.Body.Close()
		if c := resp.Header.Get("Content-Security-Policy"); strings.Contains(c, "telegram") || resp.Header.Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: Telegram's exceptions leaked: %q", path, c)
		}
	}
	if p := policy(`evil.example; script-src *`); strings.Contains(p, "evil") {
		t.Fatalf("a strange Host got into the policy: %s", p)
	}
}

// A one-time link opens a session of its own and is spent; opening it
// again where that session works goes on to the app.
func TestOneTimeLinks(t *testing.T) {
	s, ts := newTest(t)
	spent := false
	s.Exchange = func(tok string) (string, bool) {
		if tok != "invite" || spent {
			return "", false
		}
		spent = true
		return "session", true
	}
	s.Device = func(tok string) (string, bool) { return "ana", tok == "session" }
	s.Allow = func(string, string) bool { return true }
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, _ := client.Get(ts.URL + "/auth?token=invite")
	if resp.StatusCode != 303 || len(resp.Cookies()) != 1 || resp.Cookies()[0].Value != "session" {
		t.Fatalf("first use %d %+v", resp.StatusCode, resp.Cookies())
	}
	session := resp.Cookies()[0]
	if resp, _ := client.Get(ts.URL + "/auth?token=invite"); resp.StatusCode != 401 {
		t.Fatalf("a spent link signed in again: %d", resp.StatusCode)
	}
	req, _ := http.NewRequest("GET", ts.URL+"/auth?token=invite", nil)
	req.AddCookie(session)
	if resp, _ := client.Do(req); resp.StatusCode != 303 || len(resp.Cookies()) != 0 {
		t.Fatalf("reopening with a session: %d", resp.StatusCode)
	}
}

// Rotating the master token signs out the old one at once.
func TestSetToken(t *testing.T) {
	s, ts := newTest(t)
	get := func(tok string) int {
		req, _ := http.NewRequest("GET", ts.URL+"/api/events", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		return resp.StatusCode
	}
	s.SetToken("new")
	if get("tok") != 401 || get("new") != 200 {
		t.Fatal("the rotated token did not replace the old one")
	}
}

// A narrowed token opens only the routes it was given, even the owner's.
func TestNarrowedTokens(t *testing.T) {
	s, ts := newTest(t)
	s.Handle("GET /api/one", func(w http.ResponseWriter, r *http.Request) { WriteJSON(w, 200, "ok") })
	s.Handle("GET /api/two", func(w http.ResponseWriter, r *http.Request) { WriteJSON(w, 200, "ok") })
	s.Narrow = func(token, pattern string) bool { return pattern == "GET /api/one" }
	for path, want := range map[string]int{"/api/one": 200, "/api/two": 403} {
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer tok")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("%s: %d, want %d", path, resp.StatusCode, want)
		}
	}
}
