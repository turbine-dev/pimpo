// Package server serves the local web UI and its JSON API. It listens on
// loopback by default; every API call needs the session cookie that the
// one-time login link sets.
package server

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/people"
)

//go:embed all:dist
var dist embed.FS

type Server struct {
	Events *event.Store
	// token is the owner's master token; SetToken rotates it.
	tokenMu sync.RWMutex
	token   string
	// Device names the person a paired device's token belongs to; nil
	// accepts only the owner's session token.
	Device func(token string) (person string, ok bool)
	// Visible says whether a person may see an event whole; others get
	// only its type, enough to refresh a screen. nil shows nothing whole.
	Visible func(person string, e event.Event) bool
	// Allow says whether a person may use a route, by its pattern. nil
	// lets only the owner in.
	Allow func(pattern, person string) bool
	// Exchange trades a one-time sign-in link for the session it opens;
	// nil, or not ok, signs in with the link's token itself.
	Exchange func(token string) (session string, ok bool)
	// TrustedOrigin says whether a page at another address is still this
	// Pimpo, such as its public https address behind a proxy that does not
	// pass the Host on.
	TrustedOrigin func(origin string) bool
	mux           *http.ServeMux
	patterns      []string
	failures      limiter
	attempts      limiter
}

// limiter counts failed sign-ins by address. Tokens are random 192-bit
// values nobody can guess, so this is about not letting anyone hammer the
// login: past the limit, wrong attempts wait and get "too many".
type limiter struct {
	mu   sync.Mutex
	seen map[string][]time.Time
}

const (
	failWindow = 10 * time.Minute
	failLimit  = 20
)

func clientOf(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// fail records a failed attempt and says whether the address is over the
// limit.
func (l *limiter) fail(addr string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen == nil || len(l.seen) > 10000 {
		l.seen = map[string][]time.Time{}
	}
	kept := l.seen[addr][:0]
	for _, t := range l.seen[addr] {
		if now.Sub(t) < failWindow {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	l.seen[addr] = kept
	return len(kept) > failLimit
}

// refuse answers a request that did not sign in. Only a wrong credential
// counts as a failed sign-in: a page asking without one, as the app does
// before anyone signs in, is not an attempt.
func (s *Server) refuse(w http.ResponseWriter, r *http.Request, tok, msg string) {
	if tok != "" && s.failures.fail(clientOf(r), time.Now()) {
		time.Sleep(time.Second)
		WriteJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many wrong sign-ins from here; wait a few minutes"})
		return
	}
	WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": msg})
}

const cookie = "pimpo_session"

func New(events *event.Store, token string) *Server {
	s := &Server{Events: events, token: token, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /auth", s.login)
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { WriteJSON(w, 200, map[string]string{"status": "ok"}) })
	s.Handle("GET /api/events", s.listEvents)
	s.Handle("GET /api/events/verify", s.verifyEvents)
	s.Handle("GET /api/ws", s.stream)
	ui, _ := fs.Sub(dist, "dist")
	files := http.FileServerFS(ui)
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if _, err := fs.Stat(ui, strings.TrimPrefix(r.URL.Path, "/")); err != nil || r.URL.Path == "/" {
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
	return s
}

// HandlePublic registers a route that carries its own credential, such as
// the per-exploration MCP endpoint.
func (s *Server) HandlePublic(pattern string, h http.HandlerFunc) { s.mux.HandleFunc(pattern, h) }

// Handle registers an authenticated API route.
func (s *Server) Handle(pattern string, h http.HandlerFunc) {
	s.patterns = append(s.patterns, pattern)
	s.mux.Handle(pattern, s.auth(pattern, h))
}

// Patterns are the authenticated routes, for the test that every one of
// them has an access rule.
func (s *Server) Patterns() []string { return append([]string(nil), s.patterns...) }

// SetToken replaces the owner's master token: the old one, and every
// browser signed in with it, stops working at once.
func (s *Server) SetToken(t string) {
	s.tokenMu.Lock()
	s.token = t
	s.tokenMu.Unlock()
}

func (s *Server) masterToken() string {
	s.tokenMu.RLock()
	defer s.tokenMu.RUnlock()
	return s.token
}

// Attempt counts a sign-in attempt that has no credential to check yet,
// such as starting a passkey sign-in, and says whether the address is over
// the limit.
func (s *Server) Attempt(r *http.Request) bool {
	return s.attempts.fail(clientOf(r), time.Now())
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", policy(r.Host))
	s.mux.ServeHTTP(w, r)
}

var plainHost = regexp.MustCompile(`^[A-Za-z0-9.\-]+(:[0-9]+)?$|^\[[0-9A-Fa-f:.]+\](:[0-9]+)?$`)

// policy is what the web app may load: only its own files, its own API and
// live stream, images it draws (QR codes) and audio it plays. Nobody may
// frame it.
func policy(host string) string {
	live := ""
	if plainHost.MatchString(host) {
		live = " ws://" + host + " wss://" + host
	}
	return "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; font-src 'self' data:; connect-src 'self'" + live +
		"; worker-src 'self'; manifest-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
}

// login signs a browser in with a link. A one-time link opens a session of
// its own and is spent; opening it again where that session already
// works just goes on to the app.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("token")
	if s.Exchange != nil {
		if session, ok := s.Exchange(tok); ok {
			SetSession(w, r, session)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}
	if !s.valid(tok) {
		if c, err := r.Cookie(cookie); err == nil && s.valid(c.Value) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		s.refuse(w, r, tok, "invalid or expired link")
		return
	}
	SetSession(w, r, tok)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) valid(t string) bool {
	_, ok := s.who(t)
	return ok
}

// who is the person a token belongs to: the session token is the owner's.
func (s *Server) who(t string) (string, bool) {
	if t == "" {
		return "", false
	}
	if subtle.ConstantTimeCompare([]byte(t), []byte(s.masterToken())) == 1 {
		return people.OwnerID, true
	}
	if s.Device != nil {
		return s.Device(t)
	}
	return "", false
}

func (s *Server) auth(pattern string, h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := TokenOf(r)
		person, ok := s.who(tok)
		if !ok {
			s.refuse(w, r, tok, "open the login link Pimpo printed at startup")
			return
		}
		// Every request acts for the person its token belongs to, and
		// only for them; routes nobody opened to them stay closed.
		// A browser only changes things from this Pimpo's own page: the
		// cookie also rides along on requests other pages make, such as a
		// page on another port of this computer.
		if !safeMethod(r.Method) && !s.SameOrigin(r) {
			WriteJSON(w, http.StatusForbidden, map[string]string{"error": "this request did not come from Pimpo's own page"})
			return
		}
		if person != people.OwnerID && (s.Allow == nil || !s.Allow(pattern, person)) {
			WriteJSON(w, http.StatusForbidden, map[string]string{"error": "only the owner of this Pimpo can do that"})
			return
		}
		h(w, r.WithContext(people.With(r.Context(), person)))
	})
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// SameOrigin says whether a request signed in by the browser's cookie came
// from a page of this Pimpo. Clients that send their token themselves
// (the desktop app, automations, the command line) are not browsers
// carrying a cookie, so they pass. A browser says where a request comes
// from in Origin, or at least in Sec-Fetch-Site.
func (s *Server) SameOrigin(r *http.Request) bool {
	if r.Header.Get("Authorization") != "" {
		return true
	}
	o := r.Header.Get("Origin")
	if o == "" {
		switch r.Header.Get("Sec-Fetch-Site") {
		case "", "same-origin", "none":
			return true
		}
		return false
	}
	u, err := url.Parse(o)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := hostPort(u.Scheme, u.Host)
	for _, h := range []string{r.Host, r.Header.Get("X-Forwarded-Host")} {
		if h != "" && (strings.EqualFold(host, hostPort("http", h)) || strings.EqualFold(host, hostPort("https", h))) {
			return true
		}
	}
	return s.TrustedOrigin != nil && s.TrustedOrigin(u.Scheme+"://"+u.Host)
}

// hostPort drops the scheme's default port, so http://localhost and
// localhost:80 compare equal.
func hostPort(scheme, host string) string {
	if h, p, err := net.SplitHostPort(host); err == nil && (scheme == "http" && p == "80" || scheme == "https" && p == "443") {
		return h
	}
	return host
}

// SetSession signs the browser in with a session token. The cookie is
// Secure whenever the page came over https, directly or through a proxy
// such as Tailscale's; only plain http on this computer or the home
// network goes without, where browsers would otherwise drop it.
func SetSession(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{Name: cookie, Value: token, Path: "/", HttpOnly: true, Secure: overHTTPS(r), SameSite: http.SameSiteStrictMode, MaxAge: 60 * 60 * 24 * 365})
}

func overHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") || strings.HasPrefix(strings.ToLower(r.Header.Get("Origin")), "https://")
}

// TokenOf is the credential a request carries: a bearer token, or the
// login cookie.
func TokenOf(r *http.Request) string {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if c, err := r.Cookie(cookie); err == nil && tok == "" {
		tok = c.Value
	}
	return tok
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	var se StatusError
	if errors.As(err, &se) {
		status = se.Status
	}
	WriteJSON(w, status, map[string]string{"error": err.Error()})
}

type StatusError struct {
	Status int
	Msg    string
}

func (e StatusError) Error() string { return e.Msg }

// Decode reads a JSON body. A browser must say it is JSON: a form or a
// text/plain body is what another page could make it send unasked.
// Clients that send their own token may leave the type out.
func Decode(r *http.Request, v any) error {
	if r.Header.Get("Authorization") == "" && !isJSON(r.Header.Get("Content-Type")) {
		return StatusError{http.StatusUnsupportedMediaType, "send the body as application/json"}
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err := dec.Decode(v); err != nil {
		return StatusError{http.StatusBadRequest, "invalid request body: " + err.Error()}
	}
	return nil
}

func isJSON(ct string) bool {
	t, _, err := mime.ParseMediaType(ct)
	return err == nil && (t == "application/json" || strings.HasSuffix(t, "+json"))
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	q := event.Query{Search: r.URL.Query().Get("q"), Newest: r.URL.Query().Get("order") != "asc", Limit: 200}
	if t := r.URL.Query().Get("types"); t != "" {
		q.Types = strings.Split(t, ",")
	}
	if a, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64); err == nil {
		q.After = a
	}
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 1000 {
		q.Limit = l
	}
	evs, err := s.Events.List(r.Context(), q)
	if err != nil {
		WriteError(w, err)
		return
	}
	me := people.From(r.Context())
	out := []event.Event{}
	for _, e := range evs {
		if s.Visible != nil && s.Visible(me, e) {
			out = append(out, e)
		}
	}
	WriteJSON(w, 200, out)
}

func (s *Server) verifyEvents(w http.ResponseWriter, r *http.Request) {
	bad, err := s.Events.Verify(r.Context())
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, 200, map[string]any{"intact": bad == 0, "first_bad": bad})
}

// stream pushes new events to the UI as they happen.
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		for {
			if _, _, err := c.Read(ctx); err != nil {
				cancel()
				return
			}
		}
	}()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	me := people.From(r.Context())
	sub := s.Events.Subscribe(ctx)
	for {
		select {
		case e, ok := <-sub:
			if !ok {
				return
			}
			if s.Visible == nil || !s.Visible(me, e) {
				// Someone else's event: only that something changed.
				e = event.Event{ID: e.ID, Type: e.Type, Time: e.Time, Data: json.RawMessage(`{}`)}
			}
			b, _ := json.Marshal(e)
			if err := c.Write(ctx, websocket.MessageText, b); err != nil {
				return
			}
		case <-ping.C:
			if err := c.Ping(ctx); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
