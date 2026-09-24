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
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/denerFernandes/vigia/internal/event"
)

//go:embed all:dist
var dist embed.FS

type Server struct {
	Events *event.Store
	Token  string
	// Device accepts tokens given to paired devices, which can be revoked
	// one by one; nil accepts only the session token.
	Device func(token string) bool
	mux    *http.ServeMux
	api    map[string]http.HandlerFunc
}

const cookie = "vigia_session"

func New(events *event.Store, token string) *Server {
	s := &Server{Events: events, Token: token, mux: http.NewServeMux()}
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
	s.mux.Handle(pattern, s.auth(h))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	s.mux.ServeHTTP(w, r)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.valid(r.URL.Query().Get("token")) {
		http.Error(w, "invalid or expired link", http.StatusUnauthorized)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookie, Value: r.URL.Query().Get("token"), Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 60 * 60 * 24 * 365})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) valid(t string) bool {
	if t == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(t), []byte(s.Token)) == 1 || (s.Device != nil && s.Device(t))
}

func (s *Server) auth(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if c, err := r.Cookie(cookie); err == nil && tok == "" {
			tok = c.Value
		}
		if !s.valid(tok) {
			WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "open the login link Vigia printed at startup"})
			return
		}
		h(w, r)
	})
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

func Decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err := dec.Decode(v); err != nil {
		return StatusError{http.StatusBadRequest, "invalid request body: " + err.Error()}
	}
	return nil
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
	if evs == nil {
		evs = []event.Event{}
	}
	WriteJSON(w, 200, evs)
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
	sub := s.Events.Subscribe(ctx)
	for {
		select {
		case e, ok := <-sub:
			if !ok {
				return
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
