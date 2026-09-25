// Package oauth signs the owner into Google with their own OAuth client
// (installed-app flow with PKCE). Tokens stay in the vault; Zodim never
// routes them through any server of its own.
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Scopes: full mail access over IMAP/SMTP, read-only calendars, and the
// Drive files Zodim creates itself (its backups).
var Scopes = []string{"https://mail.google.com/", "https://www.googleapis.com/auth/calendar.readonly", DriveScope, "openid", "email"}

const DriveScope = "https://www.googleapis.com/auth/drive.file"

type Store interface {
	Get(ctx context.Context, name string) (string, error)
	Set(ctx context.Context, name, value string) error
}

type Google struct {
	Store Store
	// AuthURL and TokenURL are Google's; tests replace them.
	AuthURL  string
	TokenURL string
	HTTP     *http.Client

	mu      sync.Mutex
	pending map[string]pending
	access  string
	expires time.Time
}

type pending struct {
	verifier string
	redirect string
	created  time.Time
}

func (g *Google) urls() (string, string) {
	a, t := g.AuthURL, g.TokenURL
	if a == "" {
		a = "https://accounts.google.com/o/oauth2/v2/auth"
	}
	if t == "" {
		t = "https://oauth2.googleapis.com/token"
	}
	return a, t
}

func random(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Begin stores the owner's client and returns the URL to open.
func (g *Google) Begin(ctx context.Context, clientID, secret, redirect string) (string, error) {
	if clientID == "" || secret == "" {
		return "", errors.New("client ID and secret are required")
	}
	if err := g.Store.Set(ctx, "google.client_id", clientID); err != nil {
		return "", err
	}
	if err := g.Store.Set(ctx, "google.client_secret", secret); err != nil {
		return "", err
	}
	state, verifier := random(16), random(48)
	sum := sha256.Sum256([]byte(verifier))
	g.mu.Lock()
	if g.pending == nil {
		g.pending = map[string]pending{}
	}
	g.pending[state] = pending{verifier: verifier, redirect: redirect, created: time.Now()}
	g.mu.Unlock()
	auth, _ := g.urls()
	q := url.Values{
		"client_id": {clientID}, "redirect_uri": {redirect}, "response_type": {"code"},
		"scope": {strings.Join(Scopes, " ")}, "state": {state}, "access_type": {"offline"}, "prompt": {"consent"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
	}
	return auth + "?" + q.Encode(), nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

// Finish exchanges the code Google sent back for tokens and returns the
// signed-in email address.
func (g *Google) Finish(ctx context.Context, state, code string) (string, error) {
	g.mu.Lock()
	p, ok := g.pending[state]
	delete(g.pending, state)
	g.mu.Unlock()
	if !ok || time.Since(p.created) > 15*time.Minute {
		return "", errors.New("this sign-in link expired; start again from Connections")
	}
	id, _ := g.Store.Get(ctx, "google.client_id")
	secret, _ := g.Store.Get(ctx, "google.client_secret")
	tok, err := g.post(ctx, url.Values{"code": {code}, "client_id": {id}, "client_secret": {secret}, "redirect_uri": {p.redirect},
		"grant_type": {"authorization_code"}, "code_verifier": {p.verifier}})
	if err != nil {
		return "", err
	}
	if tok.RefreshToken == "" {
		return "", errors.New("Google did not grant offline access; remove Zodim's access in your Google account and try again")
	}
	if err := g.Store.Set(ctx, "google.refresh", tok.RefreshToken); err != nil {
		return "", err
	}
	g.mu.Lock()
	g.access, g.expires = tok.AccessToken, time.Now().Add(time.Duration(tok.ExpiresIn)*time.Second)
	g.mu.Unlock()
	g.Store.Set(ctx, "google.scopes", tok.Scope)
	email := emailFromIDToken(tok.IDToken)
	if email != "" {
		g.Store.Set(ctx, "google.email", email)
	}
	return email, nil
}

// Granted reports whether the owner allowed scope when signing in.
func (g *Google) Granted(ctx context.Context, scope string) bool {
	s, _ := g.Store.Get(ctx, "google.scopes")
	for _, f := range strings.Fields(s) {
		if f == scope {
			return true
		}
	}
	return false
}

// Token returns a valid access token, refreshing it when it is about to expire.
func (g *Google) Token(ctx context.Context) (string, error) {
	g.mu.Lock()
	if g.access != "" && time.Until(g.expires) > time.Minute {
		t := g.access
		g.mu.Unlock()
		return t, nil
	}
	g.mu.Unlock()
	refresh, err := g.Store.Get(ctx, "google.refresh")
	if err != nil || refresh == "" {
		return "", errors.New("Google is not connected; sign in from Connections")
	}
	id, _ := g.Store.Get(ctx, "google.client_id")
	secret, _ := g.Store.Get(ctx, "google.client_secret")
	tok, err := g.post(ctx, url.Values{"refresh_token": {refresh}, "client_id": {id}, "client_secret": {secret}, "grant_type": {"refresh_token"}})
	if err != nil {
		return "", err
	}
	g.mu.Lock()
	g.access, g.expires = tok.AccessToken, time.Now().Add(time.Duration(tok.ExpiresIn)*time.Second)
	g.mu.Unlock()
	return tok.AccessToken, nil
}

func (g *Google) post(ctx context.Context, form url.Values) (tokenResponse, error) {
	_, tokenURL := g.urls()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := g.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return tokenResponse{}, errors.New("Google is unreachable")
	}
	defer resp.Body.Close()
	var tok tokenResponse
	json.NewDecoder(resp.Body).Decode(&tok)
	if resp.StatusCode != 200 || tok.Error != "" {
		return tokenResponse{}, fmt.Errorf("Google refused: %s %s", tok.Error, tok.Description)
	}
	return tok, nil
}

// emailFromIDToken reads the email claim; the token came straight from
// Google over TLS in exchange for our own code, so it is not re-verified.
func emailFromIDToken(t string) string {
	parts := strings.Split(t, ".")
	if len(parts) != 3 {
		return ""
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Email string `json:"email"`
	}
	json.Unmarshal(b, &claims)
	return claims.Email
}
