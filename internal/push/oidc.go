// Package push checks what Google Pub/Sub and GitHub push to Pimpo's
// public address before anything acts on it, and talks to the Gmail API
// that tells a push what arrived.
package push

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// GoogleCerts is where Google publishes the keys that sign its ID tokens.
const GoogleCerts = "https://www.googleapis.com/oauth2/v3/certs"

// Verifier checks the ID token (a JWT signed by Google) that a Pub/Sub
// push subscription with authentication sends in its Authorization header.
type Verifier struct {
	// CertsURL replaces GoogleCerts; tests only.
	CertsURL string
	HTTP     *http.Client
	Now      func() time.Time

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

// certsLife is how long fetched keys are trusted before fetching again;
// an unknown key id fetches again at most once a minute.
const certsLife = time.Hour

var errToken = errors.New("the push token is not valid")

func (v *Verifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

// Verify checks the token's signature against Google's keys, that Google
// issued it, that it has not expired, that it was made for audience and
// that it identifies the service account email, verified.
func (v *Verifier) Verify(ctx context.Context, token, audience, email string) error {
	if audience == "" || email == "" {
		return errors.New("push is not set up")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errToken
	}
	var head struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if decode(parts[0], &head) != nil || head.Alg != "RS256" || head.Kid == "" {
		return errToken
	}
	key, err := v.key(ctx, head.Kid)
	if err != nil {
		return err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return errToken
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig) != nil {
		return errToken
	}
	var c struct {
		Iss      string `json:"iss"`
		Aud      string `json:"aud"`
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
		Exp      int64  `json:"exp"`
		Iat      int64  `json:"iat"`
	}
	if decode(parts[1], &c) != nil {
		return errToken
	}
	now := v.now().Unix()
	switch {
	case c.Iss != "accounts.google.com" && c.Iss != "https://accounts.google.com":
		return errToken
	case c.Exp < now || c.Iat > now+300:
		return errors.New("the push token expired")
	case c.Aud != audience:
		return errors.New("the push token was made for another address")
	case !strings.EqualFold(c.Email, email) || !c.Verified:
		return errors.New("the push token names another service account")
	}
	return nil
}

func decode(part string, out any) error {
	b, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	age := v.now().Sub(v.fetched)
	if k := v.keys[kid]; k != nil && age < certsLife {
		return k, nil
	}
	if v.keys != nil && age < time.Minute {
		return nil, errToken
	}
	keys, err := v.fetch(ctx)
	if err != nil {
		return nil, err
	}
	v.keys, v.fetched = keys, v.now()
	if k := keys[kid]; k != nil {
		return k, nil
	}
	return nil, errToken
}

func (v *Verifier) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	u := v.CertsURL
	if u == "" {
		u = GoogleCerts
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	client := v.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("Google's keys are unreachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Google's keys answered %d", resp.StatusCode)
	}
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&set); err != nil {
		return nil, err
	}
	out := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if k.Kty != "RSA" || err1 != nil || err2 != nil || len(e) > 4 {
			continue
		}
		out[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	return out, nil
}
