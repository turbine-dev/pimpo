package app

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// authenticator is a software passkey, as a phone or a laptop keeps one.
type authenticator struct {
	key    *ecdsa.PrivateKey
	id     []byte
	handle []byte
	rpID   string
	origin string
	count  uint32
}

var b64 = base64.RawURLEncoding

func newAuthenticator(t *testing.T, origin string) *authenticator {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	id := make([]byte, 16)
	rand.Read(id)
	u, _ := url.Parse(origin)
	return &authenticator{key: k, id: id, rpID: u.Hostname(), origin: origin}
}

func (a *authenticator) clientData(typ, challenge string) []byte {
	b, _ := json.Marshal(map[string]string{"type": typ, "challenge": challenge, "origin": a.origin})
	return b
}

func (a *authenticator) authData(flags byte, attested []byte) []byte {
	h := sha256.Sum256([]byte(a.rpID))
	a.count++
	out := append(h[:], flags)
	out = binary.BigEndian.AppendUint32(out, a.count)
	return append(out, attested...)
}

// create answers navigator.credentials.create.
func (a *authenticator) create(t *testing.T, options map[string]any) []byte {
	pk := options["publicKey"].(map[string]any)
	a.handle, _ = b64.DecodeString(pk["user"].(map[string]any)["id"].(string))
	x, y := a.key.PublicKey.X.FillBytes(make([]byte, 32)), a.key.PublicKey.Y.FillBytes(make([]byte, 32))
	cose, _ := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: x, -3: y})
	attested := append(make([]byte, 16), byte(len(a.id)>>8), byte(len(a.id)))
	attested = append(append(attested, a.id...), cose...)
	obj, _ := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": a.authData(0x45, attested)})
	b, _ := json.Marshal(map[string]any{"id": b64.EncodeToString(a.id), "rawId": b64.EncodeToString(a.id), "type": "public-key",
		"response": map[string]string{"clientDataJSON": b64.EncodeToString(a.clientData("webauthn.create", pk["challenge"].(string))), "attestationObject": b64.EncodeToString(obj)}})
	return b
}

// get answers navigator.credentials.get.
func (a *authenticator) get(t *testing.T, options map[string]any) []byte {
	pk := options["publicKey"].(map[string]any)
	cd := a.clientData("webauthn.get", pk["challenge"].(string))
	ad := a.authData(0x05, nil)
	h := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), h[:]...))
	sig, _ := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	b, _ := json.Marshal(map[string]any{"id": b64.EncodeToString(a.id), "rawId": b64.EncodeToString(a.id), "type": "public-key",
		"response": map[string]string{"clientDataJSON": b64.EncodeToString(cd), "authenticatorData": b64.EncodeToString(ad), "signature": b64.EncodeToString(sig), "userHandle": b64.EncodeToString(a.handle)}})
	return b
}

func (h *house) browser(t *testing.T, cookie, method, path, origin string, body []byte) (int, map[string]any, *http.Response) {
	t.Helper()
	req, _ := http.NewRequest(method, h.srv.URL+path, strings.NewReader(string(body)))
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Authorization", "Bearer "+cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	json.Unmarshal(raw, &out)
	return resp.StatusCode, out, resp
}

// Ana adds a passkey on her phone and later signs in with it, as herself;
// the session is hers alone and goes when she is removed.
func TestPasskeySignsInAsItsPerson(t *testing.T) {
	h := newHouse(t)
	u, _ := url.Parse(h.srv.URL)
	origin := "http://localhost:" + u.Port()
	auth := newAuthenticator(t, origin)

	if code, _, _ := h.browser(t, h.ana, "POST", "/api/passkeys/begin", "http://"+u.Host, []byte(`{}`)); code != 400 {
		t.Fatal("a passkey was offered for a bare IP address")
	}
	code, out, _ := h.browser(t, h.ana, "POST", "/api/passkeys/begin", origin, []byte(`{"name":"iPhone da Ana"}`))
	if code != 200 {
		t.Fatalf("begin %d %v", code, out)
	}
	code, out, _ = h.browser(t, h.ana, "POST", "/api/passkeys/finish?key="+out["key"].(string), origin, auth.create(t, out["options"].(map[string]any)))
	if code != 200 || out["name"] != "iPhone da Ana" {
		t.Fatalf("finish %d %v", code, out)
	}
	if _, body := h.raw(t, "tok", "GET", "/api/passkeys", nil); strings.Contains(body, "iPhone da Ana") {
		t.Fatal("the owner sees Ana's passkeys")
	}

	code, out, _ = h.browser(t, "", "POST", "/auth/passkey/begin", origin, nil)
	if code != 200 {
		t.Fatalf("login begin %d %v", code, out)
	}
	loginKey := out["key"].(string)
	code, out, resp := h.browser(t, "", "POST", "/auth/passkey/finish?key="+loginKey, origin, auth.get(t, out["options"].(map[string]any)))
	if code != 200 {
		t.Fatalf("login finish %d %v", code, out)
	}
	var session string
	for _, c := range resp.Cookies() {
		if c.Name == "pimpo_session" && c.HttpOnly {
			session = c.Value
		}
	}
	if session == "" {
		t.Fatal("no session cookie")
	}
	if code, body := h.raw(t, session, "GET", "/api/memory", nil); code != 200 || !strings.Contains(body, h.anaMark) || strings.Contains(body, h.ownerMark) {
		t.Fatalf("the passkey session is not Ana's: %d %.200s", code, body)
	}
	if code, _ := h.raw(t, session, "GET", "/api/settings", nil); code != 403 {
		t.Fatal("Ana's passkey session reached the owner's settings")
	}

	// A replayed answer does not sign in again.
	if code, _, _ := h.browser(t, "", "POST", "/auth/passkey/finish?key="+loginKey, origin, auth.get(t, map[string]any{"publicKey": map[string]any{"challenge": "x"}})); code == 200 {
		t.Fatal("a stale sign-in was accepted")
	}

	// Another key claiming Ana's passkey does not sign in.
	forged := newAuthenticator(t, origin)
	forged.id, forged.handle = auth.id, auth.handle
	code, out, _ = h.browser(t, "", "POST", "/auth/passkey/begin", origin, nil)
	if code, _, _ := h.browser(t, "", "POST", "/auth/passkey/finish?key="+out["key"].(string), origin, forged.get(t, out["options"].(map[string]any))); code == 200 {
		t.Fatal("a forged signature signed in")
	}

	h.do(t, "DELETE", "/api/people/"+h.anaID, nil)
	if code, _ := h.raw(t, session, "GET", "/api/memory", nil); code != 401 {
		t.Fatal("Ana's passkey session outlived her removal")
	}
	code, out, _ = h.browser(t, "", "POST", "/auth/passkey/begin", origin, nil)
	if code, _, _ := h.browser(t, "", "POST", "/auth/passkey/finish?key="+out["key"].(string), origin, auth.get(t, out["options"].(map[string]any))); code == 200 {
		t.Fatal("a removed person's passkey still signs in")
	}
}
