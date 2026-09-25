package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

type memStore struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memStore) Get(_ context.Context, k string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[k], nil
}
func (s *memStore) Set(_ context.Context, k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k] = v
	return nil
}

func TestSignInAndRefresh(t *testing.T) {
	refreshes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "the-code" || r.Form.Get("code_verifier") == "" {
				w.WriteHeader(400)
				w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			claims, _ := json.Marshal(map[string]string{"email": "eu@gmail.com"})
			id := "h." + base64.RawURLEncoding.EncodeToString(claims) + ".s"
			w.Write([]byte(`{"access_token":"a1","refresh_token":"r1","expires_in":30,"scope":"https://mail.google.com/ https://www.googleapis.com/auth/drive.file","id_token":"` + id + `"}`))
		case "refresh_token":
			refreshes++
			w.Write([]byte(`{"access_token":"a2","expires_in":3600}`))
		}
	}))
	defer srv.Close()
	store := &memStore{m: map[string]string{}}
	g := &Google{Store: store, AuthURL: srv.URL + "/auth", TokenURL: srv.URL + "/token"}
	ctx := context.Background()
	link, err := g.Begin(ctx, "cid", "secret", "http://127.0.0.1:7788/oauth/google")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(link)
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("access_type") != "offline" || !strings.Contains(q.Get("scope"), "mail.google.com") || !strings.Contains(q.Get("scope"), DriveScope) {
		t.Fatalf("auth url %s", link)
	}
	if _, err := g.Finish(ctx, "wrong-state", "the-code"); err == nil {
		t.Fatal("accepted an unknown state")
	}
	email, err := g.Finish(ctx, q.Get("state"), "the-code")
	if err != nil || email != "eu@gmail.com" || store.m["google.refresh"] != "r1" {
		t.Fatalf("finish %q %v %v", email, err, store.m)
	}
	if !g.Granted(ctx, DriveScope) || g.Granted(ctx, "https://www.googleapis.com/auth/drive") {
		t.Fatal("granted scopes misread")
	}
	// The first access token expires in 30s, so Token refreshes it.
	tok, err := g.Token(ctx)
	if err != nil || tok != "a2" || refreshes != 1 {
		t.Fatalf("token %q %v refreshes %d", tok, err, refreshes)
	}
	tok, _ = g.Token(ctx)
	if tok != "a2" || refreshes != 1 {
		t.Fatal("a fresh token was refreshed again")
	}
}
