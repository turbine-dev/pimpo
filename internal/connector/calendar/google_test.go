package calendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGoogleCalendarAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/calendarList"):
			w.Write([]byte(`{"items":[{"id":"primary@x","summary":"Trabalho"},{"id":"bdays","summary":"Birthdays"}]}`))
		case strings.Contains(r.URL.Path, "primary@x"):
			w.Write([]byte(`{"items":[{"id":"e1","summary":"Standup","start":{"dateTime":"2026-09-24T12:30:00Z"},"end":{"dateTime":"2026-09-24T12:45:00Z"},"attendees":[{"email":"Ana@Acme.com"}]},{"id":"e2","summary":"Cancelada","status":"cancelled","start":{"dateTime":"2026-09-24T15:00:00Z"},"end":{"dateTime":"2026-09-24T16:00:00Z"}}]}`))
		default:
			w.Write([]byte(`{"items":[{"id":"b1","summary":"Aniversário da Marina","start":{"date":"2026-09-24"},"end":{"date":"2026-09-25"}}]}`))
		}
	}))
	defer srv.Close()
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	g := &Google{Base: srv.URL, Zone: sp, Token: func(context.Context) (string, error) { return "tok", nil }}
	got, err := g.Call(context.Background(), "calendar.events", "", map[string]any{"from": "2026-09-24", "to": "2026-09-25"})
	if err != nil {
		t.Fatal(err)
	}
	evs := got.([]Event)
	if len(evs) != 2 || evs[0].Title != "Aniversário da Marina" || !evs[0].AllDay || evs[1].Start != "2026-09-24T09:30:00-03:00" || evs[1].Attendees[0] != "ana@acme.com" {
		t.Fatalf("events %+v", evs)
	}
}
