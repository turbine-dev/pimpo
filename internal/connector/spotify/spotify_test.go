package spotify

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

type mem struct {
	sync.Mutex
	m map[string]string
}

func (s *mem) Get(_ context.Context, k string) (string, error) {
	s.Lock()
	defer s.Unlock()
	return s.m[k], nil
}
func (s *mem) Set(_ context.Context, k, v string) error {
	s.Lock()
	defer s.Unlock()
	s.m[k] = v
	return nil
}

func TestSpotify(t *testing.T) {
	var challenge string
	var played map[string]any
	premium := true
	acc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge || r.Form.Get("code") != "c0de" {
				w.WriteHeader(400)
				w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			w.Write([]byte(`{"access_token":"a1","refresh_token":"r1","expires_in":3600}`))
		case "refresh_token":
			w.Write([]byte(`{"access_token":"a2","expires_in":3600}`))
		}
	}))
	defer acc.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a := r.Header.Get("Authorization"); a != "Bearer a1" && a != "Bearer a2" {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.URL.Path == "/search":
			w.Write([]byte(`{"shows":{"items":[{"name":"Hacker News Podcast","uri":"spotify:show:abc"}]}}`))
		case r.URL.Path == "/me/player/play":
			if !premium {
				w.WriteHeader(403)
				return
			}
			json.NewDecoder(r.Body).Decode(&played)
			w.WriteHeader(204)
		case r.URL.Path == "/me/player":
			w.Write([]byte(`{"is_playing":true,"device":{"name":"Sala"},"item":{"name":"Numb","uri":"spotify:track:1","artists":[{"name":"Linkin Park"}]}}`))
		case r.URL.Path == "/me/player/devices":
			w.Write([]byte(`{"devices":[{"id":"d1","name":"Sala","type":"Speaker","is_active":true,"volume_percent":40}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer api.Close()
	store := &mem{m: map[string]string{}}
	s := &Spotify{Store: store, Accounts: acc.URL, API: api.URL}
	ctx := context.Background()
	link, err := s.Begin(ctx, "0123456789abcdef0123", "http://127.0.0.1:7788/oauth/spotify")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(link)
	challenge = u.Query().Get("code_challenge")
	if u.Query().Get("code_challenge_method") != "S256" || !strings.Contains(u.Query().Get("scope"), "user-modify-playback-state") {
		t.Fatalf("link %s", link)
	}
	if err := s.Finish(ctx, "forged", "c0de"); err == nil {
		t.Fatal("finished with a forged state")
	}
	if err := s.Finish(ctx, u.Query().Get("state"), "c0de"); err != nil || !s.Connected(ctx) || store.m["spotify.refresh"] != "r1" {
		t.Fatalf("finish %v %v", err, store.m)
	}
	out, err := s.Call(ctx, "spotify.play", "", map[string]any{"query": "hacker news", "kind": "show", "device": "sala"})
	if err != nil || played["context_uri"] != "spotify:show:abc" || out.(map[string]any)["playing"] != "Hacker News Podcast" {
		t.Fatalf("%v %v %v", out, err, played)
	}
	now, _ := s.Call(ctx, "spotify.now", "", map[string]any{})
	if n := now.(map[string]any); n["title"] != "Numb" || n["by"] != "Linkin Park" || n["device"] != "Sala" {
		t.Fatalf("now %v", now)
	}
	s.access = "" // expired: refreshes
	premium = false
	if _, err := s.Call(ctx, "spotify.play", "", map[string]any{"uri": "spotify:track:1"}); err == nil || !strings.Contains(err.Error(), "Premium") {
		t.Fatalf("no premium: %v", err)
	}
	if _, err := s.Call(ctx, "spotify.play", "", map[string]any{"device": "Cozinha"}); err == nil || !strings.Contains(err.Error(), "Sala") {
		t.Fatalf("unknown device: %v", err)
	}
	if _, err := s.Begin(ctx, "short", "x"); err == nil {
		t.Fatal("accepted a bad client id")
	}
}
