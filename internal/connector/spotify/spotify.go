// Package spotify controls the owner's Spotify: what is playing, play
// something, pause, volume and devices. It signs in with PKCE, with the
// owner's own app's client id and no secret; control needs Premium.
package spotify

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/connector"
)

// Store keeps the client id and the refresh token (the vault).
type Store interface {
	Get(ctx context.Context, name string) (string, error)
	Set(ctx context.Context, name, value string) error
}

const scopes = "user-read-playback-state user-modify-playback-state user-read-currently-playing"

type Spotify struct {
	Store Store
	// Accounts and API replace Spotify's addresses; tests only.
	Accounts, API string
	HTTP          *http.Client

	mu      sync.Mutex
	pending map[string]pending
	access  string
	expires time.Time
}

type pending struct {
	verifier, redirect string
	created            time.Time
}

func (s *Spotify) urls() (string, string) {
	acc, api := s.Accounts, s.API
	if acc == "" {
		acc = "https://accounts.spotify.com"
	}
	if api == "" {
		api = "https://api.spotify.com/v1"
	}
	return acc, api
}

func (s *Spotify) client() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func random(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Begin keeps the client id and returns the address to sign in at.
func (s *Spotify) Begin(ctx context.Context, clientID, redirect string) (string, error) {
	clientID = strings.TrimSpace(clientID)
	if len(clientID) < 16 {
		return "", errors.New("paste the Client ID of your app from developer.spotify.com")
	}
	if err := s.Store.Set(ctx, "spotify.client_id", clientID); err != nil {
		return "", err
	}
	state, verifier := random(16), random(48)
	sum := sha256.Sum256([]byte(verifier))
	s.mu.Lock()
	if s.pending == nil {
		s.pending = map[string]pending{}
	}
	s.pending[state] = pending{verifier, redirect, time.Now()}
	s.mu.Unlock()
	acc, _ := s.urls()
	q := url.Values{"client_id": {clientID}, "response_type": {"code"}, "redirect_uri": {redirect}, "scope": {scopes}, "state": {state},
		"code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}}
	return acc + "/authorize?" + q.Encode(), nil
}

type token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

func (s *Spotify) post(ctx context.Context, form url.Values) (token, error) {
	acc, _ := s.urls()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, acc+"/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client().Do(req)
	if err != nil {
		return token{}, fmt.Errorf("Spotify is unreachable: %w", err)
	}
	defer resp.Body.Close()
	var t token
	json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&t)
	if resp.StatusCode != 200 || t.AccessToken == "" {
		return t, fmt.Errorf("Spotify refused: %s", strings.TrimSpace(t.Error+" "+t.Description))
	}
	return t, nil
}

// Finish trades the code Spotify sent back for tokens.
func (s *Spotify) Finish(ctx context.Context, state, code string) error {
	s.mu.Lock()
	p, ok := s.pending[state]
	delete(s.pending, state)
	s.mu.Unlock()
	if !ok || time.Since(p.created) > 15*time.Minute {
		return errors.New("this sign-in link expired; start again from Connections")
	}
	id, _ := s.Store.Get(ctx, "spotify.client_id")
	t, err := s.post(ctx, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {p.redirect}, "client_id": {id}, "code_verifier": {p.verifier}})
	if err != nil {
		return err
	}
	s.keep(ctx, t)
	return nil
}

func (s *Spotify) keep(ctx context.Context, t token) {
	if t.RefreshToken != "" {
		s.Store.Set(ctx, "spotify.refresh", t.RefreshToken)
	}
	s.mu.Lock()
	s.access, s.expires = t.AccessToken, time.Now().Add(time.Duration(t.ExpiresIn)*time.Second)
	s.mu.Unlock()
}

// Connected says whether a sign-in is kept.
func (s *Spotify) Connected(ctx context.Context) bool {
	r, _ := s.Store.Get(ctx, "spotify.refresh")
	return r != ""
}

func (s *Spotify) token(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.access != "" && time.Until(s.expires) > time.Minute {
		t := s.access
		s.mu.Unlock()
		return t, nil
	}
	s.mu.Unlock()
	refresh, _ := s.Store.Get(ctx, "spotify.refresh")
	if refresh == "" {
		return "", errors.New("Spotify is not connected; sign in from Connections")
	}
	id, _ := s.Store.Get(ctx, "spotify.client_id")
	t, err := s.post(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {id}})
	if err != nil {
		return "", err
	}
	s.keep(ctx, t)
	return t.AccessToken, nil
}

func (s *Spotify) do(ctx context.Context, method, path string, body, out any) error {
	tok, err := s.token(ctx)
	if err != nil {
		return err
	}
	_, api := s.urls()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, api+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return fmt.Errorf("Spotify is unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	switch {
	case resp.StatusCode == 204 || (resp.StatusCode/100 == 2 && len(raw) == 0):
		return nil
	case resp.StatusCode == 403:
		return errors.New("Spotify refused: controlling playback needs Spotify Premium")
	case resp.StatusCode == 404:
		return errors.New("no active Spotify device: open Spotify on a phone, computer or speaker first")
	case resp.StatusCode == 401:
		return errors.New("Spotify signed Pimpo out; sign in again from Connections")
	case resp.StatusCode/100 != 2:
		return fmt.Errorf("Spotify answered %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func (s *Spotify) Capabilities() []string {
	return []string{"spotify.now", "spotify.play", "spotify.pause", "spotify.volume", "spotify.devices"}
}

func (s *Spotify) Call(ctx context.Context, name, _ string, args any) (any, error) {
	var in struct {
		Query   string `json:"query"`
		URI     string `json:"uri"`
		Kind    string `json:"kind"`
		Device  string `json:"device"`
		Percent *int   `json:"percent"`
	}
	if err := connector.Args(args, &in); err != nil {
		return nil, err
	}
	device := ""
	if in.Device != "" {
		id, err := s.deviceID(ctx, in.Device)
		if err != nil {
			return nil, err
		}
		device = "?device_id=" + url.QueryEscape(id)
	}
	switch name {
	case "spotify.devices":
		var r struct {
			Devices []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Type     string `json:"type"`
				IsActive bool   `json:"is_active"`
				Volume   int    `json:"volume_percent"`
			} `json:"devices"`
		}
		if err := s.do(ctx, http.MethodGet, "/me/player/devices", nil, &r); err != nil {
			return nil, err
		}
		out := []map[string]any{}
		for _, d := range r.Devices {
			out = append(out, map[string]any{"name": d.Name, "type": d.Type, "active": d.IsActive, "volume": d.Volume})
		}
		return out, nil
	case "spotify.now":
		var r struct {
			IsPlaying bool `json:"is_playing"`
			Device    struct {
				Name string `json:"name"`
			} `json:"device"`
			Item *struct {
				Name    string `json:"name"`
				URI     string `json:"uri"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Show *struct {
					Name string `json:"name"`
				} `json:"show"`
			} `json:"item"`
		}
		if err := s.do(ctx, http.MethodGet, "/me/player?additional_types=episode", nil, &r); err != nil {
			return nil, err
		}
		out := map[string]any{"playing": r.IsPlaying, "device": r.Device.Name}
		if r.Item != nil {
			var by []string
			for _, a := range r.Item.Artists {
				by = append(by, a.Name)
			}
			if r.Item.Show != nil {
				by = append(by, r.Item.Show.Name)
			}
			out["title"], out["by"], out["uri"] = r.Item.Name, strings.Join(by, ", "), r.Item.URI
		}
		return out, nil
	case "spotify.pause":
		if err := s.do(ctx, http.MethodPut, "/me/player/pause"+device, nil, nil); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "undo": map[string]any{"capability": "spotify.play", "args": map[string]any{}}}, nil
	case "spotify.volume":
		if in.Percent == nil || *in.Percent < 0 || *in.Percent > 100 {
			return nil, errors.New("percent is 0 to 100")
		}
		sep := "?"
		if device != "" {
			sep = "&"
		}
		if err := s.do(ctx, http.MethodPut, "/me/player/volume"+device+sep+"volume_percent="+fmt.Sprint(*in.Percent), nil, nil); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "spotify.play":
		uri := strings.TrimSpace(in.URI)
		title := ""
		if uri == "" && strings.TrimSpace(in.Query) != "" {
			var err error
			uri, title, err = s.search(ctx, in.Query, in.Kind)
			if err != nil {
				return nil, err
			}
		}
		var body any
		switch {
		case uri == "":
			// resume
		case strings.HasPrefix(uri, "spotify:track:") || strings.HasPrefix(uri, "spotify:episode:"):
			body = map[string]any{"uris": []string{uri}}
		case strings.HasPrefix(uri, "spotify:"):
			body = map[string]any{"context_uri": uri}
		default:
			return nil, errors.New("uri is a spotify: address, like spotify:playlist:…")
		}
		if err := s.do(ctx, http.MethodPut, "/me/player/play"+device, body, nil); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "playing": firstOf(title, uri), "undo": map[string]any{"capability": "spotify.pause", "args": map[string]any{}}}, nil
	}
	return nil, fmt.Errorf("unknown capability %s", name)
}

// search finds what to play: a track by default, or a playlist, album,
// artist, show or episode.
func (s *Spotify) search(ctx context.Context, q, kind string) (string, string, error) {
	if kind == "" {
		kind = "track"
	}
	switch kind {
	case "track", "playlist", "album", "artist", "show", "episode":
	default:
		return "", "", errors.New("kind is track, playlist, album, artist, show or episode")
	}
	var r map[string]struct {
		Items []*struct {
			Name string `json:"name"`
			URI  string `json:"uri"`
		} `json:"items"`
	}
	if err := s.do(ctx, http.MethodGet, "/search?"+url.Values{"q": {q}, "type": {kind}, "limit": {"3"}}.Encode(), nil, &r); err != nil {
		return "", "", err
	}
	for _, it := range r[kind+"s"].Items {
		if it != nil && it.URI != "" {
			return it.URI, it.Name, nil
		}
	}
	return "", "", fmt.Errorf("nothing on Spotify for %q", q)
}

func (s *Spotify) deviceID(ctx context.Context, name string) (string, error) {
	var r struct {
		Devices []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"devices"`
	}
	if err := s.do(ctx, http.MethodGet, "/me/player/devices", nil, &r); err != nil {
		return "", err
	}
	var names []string
	for _, d := range r.Devices {
		if strings.EqualFold(d.Name, name) {
			return d.ID, nil
		}
		names = append(names, d.Name)
	}
	return "", fmt.Errorf("no Spotify device %q (available: %s)", name, strings.Join(names, ", "))
}

func firstOf(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
