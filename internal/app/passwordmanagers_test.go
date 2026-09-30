package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPasswordManagersArePerPersonAndNeverShowSecrets(t *testing.T) {
	h := newHouse(t)
	ctx := context.Background()
	hv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Header.Get("X-Vault-Token") != "hvs.house-token":
			w.WriteHeader(403)
		case r.URL.Path == "/v1/auth/token/lookup-self":
			w.Write([]byte(`{"data":{}}`))
		case r.URL.Path == "/v1/secret/data/home":
			w.Write([]byte(`{"data":{"data":{"jev":"ts_REALVALUE","mail":"app pass word"}}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer hv.Close()

	code, body := h.raw(t, "tok", "PUT", "/api/password-managers/hashicorp", js(map[string]string{"addr": hv.URL, "auth": "token", "token": "hvs.house-token", "namespace": "family"}))
	if code != 200 || strings.Contains(body, "hvs.house-token") || !strings.Contains(body, `"auth":"token"`) || !strings.Contains(body, `"house":true`) {
		t.Fatalf("put %d %s", code, body)
	}
	if code, body := h.raw(t, "tok", "POST", "/api/password-managers/hashicorp/test", nil); code != 200 {
		t.Fatalf("test %d %s", code, body)
	}
	// A reference is checked once and answers only that it was found.
	code, body = h.raw(t, "tok", "POST", "/api/secrets/check", js(map[string]string{"reference": "vault://secret/data/home#jev"}))
	if code != 200 || strings.Contains(body, "REALVALUE") || !strings.Contains(body, `"found":true`) {
		t.Fatalf("check %d %s", code, body)
	}
	code, body = h.raw(t, "tok", "POST", "/api/secrets/check", js(map[string]string{"reference": "vault://secret/data/home#nope"}))
	if code != 400 || !strings.Contains(body, "vault://secret/data/home#nope") {
		t.Fatalf("missing %d %s", code, body)
	}

	// A connection keeps the reference, and its consumer gets the value.
	if code, body := h.raw(t, "tok", "PUT", "/api/connections/jev", js(map[string]string{"key": "vault://secret/data/home#jev"})); code != 200 && code != 204 {
		t.Fatalf("jev %d %s", code, body)
	}
	if raw, _ := h.Vault.Raw(ctx, "typesafe.key"); raw != "vault://secret/data/home#jev" {
		t.Fatalf("stored %q", raw)
	}
	if v, _ := h.Vault.Get(ctx, "typesafe.key"); v != "ts_REALVALUE" {
		t.Fatalf("resolved %q", v)
	}
	// Spaces in a reference stay; spaces in an app password go.
	if appPassword("op://Home/My Mail/app password") != "op://Home/My Mail/app password" || appPassword("abcd efgh") != "abcdefgh" {
		t.Fatal("app password")
	}

	// Ana sees only her own, empty, and cannot use the house's.
	code, body = h.raw(t, h.ana, "GET", "/api/password-managers", nil)
	if code != 200 || strings.Contains(body, hv.URL) || !strings.Contains(body, `"house":false`) {
		t.Fatalf("ana's view %d %s", code, body)
	}
	code, body = h.raw(t, h.ana, "POST", "/api/secrets/check", js(map[string]string{"reference": "vault://secret/data/home#jev"}))
	if code != 400 || !strings.Contains(body, "not set up for this person") {
		t.Fatalf("ana used the house's vault: %d %s", code, body)
	}
	// Her own must be reached over https.
	if code, body := h.raw(t, h.ana, "PUT", "/api/password-managers/hashicorp", js(map[string]string{"addr": hv.URL, "auth": "token", "token": "hvs.house-token"})); code != 400 || !strings.Contains(body, "https://") {
		t.Fatalf("ana over http %d %s", code, body)
	}
	if code, _ := h.raw(t, h.ana, "PUT", "/api/password-managers/onepassword", js(map[string]string{"mode": "desktop"})); code != 400 {
		t.Fatal("ana used the house's 1Password app")
	}
	code, body = h.raw(t, h.ana, "PUT", "/api/password-managers/onepassword", js(map[string]string{"mode": "service", "token": "ops_ana"}))
	if code != 200 || strings.Contains(body, "ops_ana") || !strings.Contains(body, `"mode":"service"`) {
		t.Fatalf("ana's own %d %s", code, body)
	}
	// The owner's view is still the house's alone.
	_, body = h.raw(t, "tok", "GET", "/api/password-managers", nil)
	if strings.Contains(body, `"mode":"service"`) {
		t.Fatalf("the owner saw Ana's: %s", body)
	}
	if code, body := h.raw(t, "tok", "DELETE", "/api/password-managers/hashicorp", nil); code != 200 || strings.Contains(body, hv.URL) {
		t.Fatalf("delete %d %s", code, body)
	}
	// Nothing in the event log holds a credential or a value.
	var n, all int
	h.Events.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE type = 'password_manager.changed'`).Scan(&all)
	h.Events.DB().QueryRow(`SELECT COUNT(*) FROM events WHERE data LIKE '%hvs.house%' OR data LIKE '%REALVALUE%' OR data LIKE '%ops_ana%'`).Scan(&n)
	if n != 0 || all != 3 {
		t.Fatal("a secret reached the event log")
	}
}
