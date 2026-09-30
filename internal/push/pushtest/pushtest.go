// Package pushtest signs tokens like Google does, for tests of push.
package pushtest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
)

// FakeGoogle signs ID tokens like Google and serves its keys.
type FakeGoogle struct {
	Key   *rsa.PrivateKey
	Kid   string
	Certs *httptest.Server
}

func NewFakeGoogle(t *testing.T) *FakeGoogle {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &FakeGoogle{Key: k, Kid: "k1"}
	f.Certs = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": f.Kid, "kty": "RSA", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(k.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes()),
		}}})
	}))
	t.Cleanup(f.Certs.Close)
	return f
}

func (f *FakeGoogle) Sign(claims map[string]any) string {
	enc := func(v any) string { b, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(b) }
	head := enc(map[string]string{"alg": "RS256", "kid": f.Kid, "typ": "JWT"})
	body := enc(claims)
	sum := sha256.Sum256([]byte(head + "." + body))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, f.Key, crypto.SHA256, sum[:])
	return head + "." + body + "." + base64.RawURLEncoding.EncodeToString(sig)
}
