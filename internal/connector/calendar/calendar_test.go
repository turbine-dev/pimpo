package calendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.FileServer(http.Dir("testdata")))
	t.Cleanup(srv.Close)
	return srv
}

func titles(evs []Event) string {
	var out []string
	for _, e := range evs {
		out = append(out, e.Start[:min(16, len(e.Start))]+" "+e.Title)
	}
	return strings.Join(out, " | ")
}

func TestEventsAcrossFeeds(t *testing.T) {
	srv := serve(t)
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	c := &Calendar{Zone: sp, Feeds: func(context.Context) ([]string, error) {
		return []string{srv.URL + "/work.ics", srv.URL + "/birthdays.ics"}, nil
	}}
	got, err := c.Call(context.Background(), "calendar.events", "", map[string]any{"from": "2026-09-24", "to": "2026-09-26"})
	if err != nil {
		t.Fatal(err)
	}
	evs := got.([]Event)
	want := "2026-09-24T09:30 Standup | 2026-09-24T14:00 Revisão do contrato | 2026-09-25 Aniversário da Marina"
	if titles(evs) != want {
		t.Fatalf("got  %s\nwant %s", titles(evs), want)
	}
	review := evs[1]
	if review.Location != "Sala 3" || strings.Join(review.Attendees, ",") != "ana@acme.com,bruno@acme.com" || review.Calendar != "Trabalho" {
		t.Fatalf("review %+v", review)
	}
	if !evs[2].AllDay || evs[2].Calendar != "Birthdays" {
		t.Fatalf("birthday %+v", evs[2])
	}
}

func TestBadInputAndBrokenFeeds(t *testing.T) {
	srv := serve(t)
	c := &Calendar{Feeds: func(context.Context) ([]string, error) { return []string{srv.URL + "/missing.ics"}, nil }}
	if _, err := c.Call(context.Background(), "", "", map[string]any{"from": "2026-09-24", "to": "2026-09-25"}); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("missing feed: %v", err)
	}
	if _, err := c.Call(context.Background(), "", "", map[string]any{"from": "soon", "to": "later"}); err == nil {
		t.Fatal("accepted a bad date")
	}
	if _, err := c.Call(context.Background(), "", "", map[string]any{"from": "2026-09-25", "to": "2026-09-24"}); err == nil {
		t.Fatal("accepted an inverted range")
	}
}
