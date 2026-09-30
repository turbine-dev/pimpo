package vault

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
)

// fakeOP puts testdata/fakeop/op first on PATH with a fresh HOME holding its
// answers, and gives a vault whose clock the test moves.
func fakeOP(t *testing.T, store string) (*Vault, string, *time.Time) {
	t.Helper()
	bin, _ := filepath.Abs("testdata/fakeop")
	home := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", home)
	os.WriteFile(filepath.Join(home, "op-store"), []byte(store), 0o600)
	dir := t.TempDir()
	st, err := event.Open(filepath.Join(dir, "pimpo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	v, err := Open(st.DB(), FileKey(filepath.Join(dir, "vault.key")))
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	v.out.now = func() time.Time { return clock }
	return v, home, &clock
}

func opCalls(home string) int {
	b, _ := os.ReadFile(filepath.Join(home, "op-calls"))
	return strings.Count(string(b), "\n")
}

func TestReferencesAreRecognized(t *testing.T) {
	for s, want := range map[string]bool{
		"op://Home/Mail/password":           true,
		"op://Home/My Mail/login/password":  true,
		"vault://secret/data/mail#password": true,
		"op://Home/Mail":                    false,
		"op://Home//password":               false,
		"op://Home/../password":             false,
		"vault://secret/data/mail":          false,
		"vault:///etc#x":                    false,
		"hunter2":                           false,
		"https://example.com/feed.ics":      false,
	} {
		if IsReference(s) != want {
			t.Errorf("IsReference(%q) = %v", s, !want)
		}
	}
	if ScopeOf("person.ana.mail.password") != "ana" || ScopeOf("mail.password") != "" || ScopeOf("person.x") != "" {
		t.Fatal("wrong scope")
	}
}

func TestOnePasswordReferencesResolveWithACleanEnvironment(t *testing.T) {
	v, home, clock := fakeOP(t, "tok-house op://Home/Mail/password s3cret-house\n")
	t.Setenv("PIMPO_SHOULD_NOT_LEAK", "1")
	ctx := context.Background()
	if err := v.SetManagers(ctx, "", Managers{OnePassword: &OnePassword{Token: "tok-house"}}); err != nil {
		t.Fatal(err)
	}
	v.Set(ctx, "mail.password", "op://Home/Mail/password")
	got, err := v.Get(ctx, "mail.password")
	if err != nil || got != "s3cret-house" {
		t.Fatalf("got %q %v", got, err)
	}
	env, _ := os.ReadFile(filepath.Join(home, "op-env"))
	if strings.Contains(string(env), "PIMPO_SHOULD_NOT_LEAK") || !strings.Contains(string(env), "OP_SERVICE_ACCOUNT_TOKEN=tok-house") {
		t.Fatalf("op did not get a clean environment:\n%s", env)
	}
	for _, l := range strings.Split(strings.TrimSpace(string(env)), "\n") {
		k, _, _ := strings.Cut(l, "=")
		switch k {
		case "PATH", "HOME", "OP_SERVICE_ACCOUNT_TOKEN", "PWD", "SHLVL", "_", "OLDPWD":
		default:
			t.Errorf("op saw %s", k)
		}
	}
	// What is stored and exported is the reference.
	if raw, _ := v.Raw(ctx, "mail.password"); raw != "op://Home/Mail/password" {
		t.Fatalf("raw %q", raw)
	}

	// A second read within five minutes does not ask 1Password again.
	v.Get(ctx, "mail.password")
	if n := opCalls(home); n != 1 {
		t.Fatalf("op ran %d times", n)
	}
	*clock = clock.Add(6 * time.Minute)
	v.Get(ctx, "mail.password")
	if n := opCalls(home); n != 2 {
		t.Fatalf("after five minutes op ran %d times", n)
	}
	// A new set of credentials forgets what the old ones read.
	v.SetManagers(ctx, "", Managers{OnePassword: &OnePassword{Token: "tok-house"}})
	v.Get(ctx, "mail.password")
	if n := opCalls(home); n != 3 {
		t.Fatalf("after new credentials op ran %d times", n)
	}
	if !v.OPInstalled() {
		t.Fatal("op not found")
	}
	if err := v.TestManager(ctx, "", "onepassword"); err != nil {
		t.Fatal(err)
	}
}

func TestAReferenceThatFailsNamesTheReference(t *testing.T) {
	v, home, clock := fakeOP(t, "tok op://Home/Mail/password s3cret\n")
	ctx := context.Background()
	v.Set(ctx, "model.openai.key", "op://Home/Gone/password")
	_, err := v.Get(ctx, "model.openai.key")
	var re *RefError
	if !errors.As(err, &re) || !strings.Contains(err.Error(), "op://Home/Gone/password") || !strings.Contains(err.Error(), "not set up for the house") {
		t.Fatalf("unset: %v", err)
	}
	v.SetManagers(ctx, "", Managers{OnePassword: &OnePassword{Token: "tok"}})
	_, err = v.Get(ctx, "model.openai.key")
	if err == nil || !strings.Contains(err.Error(), "op://Home/Gone/password") || !strings.Contains(err.Error(), "item not found") || strings.Contains(err.Error(), "2026/09/30") {
		t.Fatalf("missing: %v", err)
	}
	// A failure is kept for a short while, then tried again.
	n := opCalls(home)
	v.Get(ctx, "model.openai.key")
	if opCalls(home) != n {
		t.Fatal("a failure was not kept")
	}
	*clock = clock.Add(time.Minute)
	v.Get(ctx, "model.openai.key")
	if opCalls(home) != n+1 {
		t.Fatal("a failure was kept too long")
	}
	// Check reads afresh and says only whether it was found.
	if err := v.Check(ctx, "", "op://Home/Mail/password"); err != nil {
		t.Fatal(err)
	}
	if err := v.Check(ctx, "", "op://Home/Mail"); !errors.Is(err, ErrBadReference) {
		t.Fatalf("bad reference: %v", err)
	}

	opTimeout = 200 * time.Millisecond
	t.Cleanup(func() { opTimeout = 15 * time.Second })
	start := time.Now()
	err = v.Check(ctx, "", "op://Slow/Mail/password")
	if err == nil || !strings.Contains(err.Error(), "did not answer in time") || time.Since(start) > 3*time.Second {
		t.Fatalf("slow: %v after %s", err, time.Since(start))
	}
}

func TestAPersonsReferencesUseOnlyTheirOwnCredentials(t *testing.T) {
	v, _, _ := fakeOP(t, "tok-house op://Home/Mail/password house-value\ntok-ana op://Home/Mail/password ana-value\n- op://Home/Mail/password desktop-value\n")
	ctx := context.Background()
	v.SetManagers(ctx, "", Managers{OnePassword: &OnePassword{Token: "tok-house"}})
	v.Set(ctx, "person.ana.mail.password", "op://Home/Mail/password")
	if _, err := v.Get(ctx, "person.ana.mail.password"); err == nil || !strings.Contains(err.Error(), "not set up for this person") {
		t.Fatalf("a person's reference used the house's credentials: %v", err)
	}
	if err := v.SetManagers(ctx, "ana", Managers{OnePassword: &OnePassword{Desktop: true}}); err == nil {
		t.Fatal("a person may not use the house's 1Password app")
	}
	v.SetManagers(ctx, "ana", Managers{OnePassword: &OnePassword{Token: "tok-ana"}})
	if got, err := v.Get(ctx, "person.ana.mail.password"); err != nil || got != "ana-value" {
		t.Fatalf("ana: %q %v", got, err)
	}
	v.Set(ctx, "mail.password", "op://Home/Mail/password")
	if got, _ := v.Get(ctx, "mail.password"); got != "house-value" {
		t.Fatalf("house: %q", got)
	}
	// The house may use the op signed in through the desktop app.
	v.SetManagers(ctx, "", Managers{OnePassword: &OnePassword{Desktop: true}})
	if got, _ := v.Get(ctx, "mail.password"); got != "desktop-value" {
		t.Fatalf("desktop: %q", got)
	}
	m, _ := v.Managers(ctx, "ana")
	if m.OnePassword.Mode() != "service" || m.HashiCorp != nil {
		t.Fatalf("ana's managers %+v", m)
	}
}

// fakeHV is a HashiCorp Vault with one KV v2 secret, a token, and an
// AppRole that signs in to it.
func fakeHV(t *testing.T) (*httptest.Server, *[]string) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path+" ns="+r.Header.Get("X-Vault-Namespace"))
		switch {
		case r.URL.Path == "/v1/auth/approle/login":
			var in map[string]string
			json.NewDecoder(r.Body).Decode(&in)
			if in["role_id"] != "role" || in["secret_id"] != "sid" {
				w.WriteHeader(400)
				return
			}
			w.Write([]byte(`{"auth":{"client_token":"hvs.approle","lease_duration":3600}}`))
		case r.Header.Get("X-Vault-Token") != "hvs.root" && r.Header.Get("X-Vault-Token") != "hvs.approle":
			w.WriteHeader(403)
		case r.URL.Path == "/v1/auth/token/lookup-self":
			w.Write([]byte(`{"data":{}}`))
		case r.URL.Path == "/v1/secret/data/home/mail":
			w.Write([]byte(`{"data":{"data":{"password":"hv-value","port":993},"metadata":{}}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestHashiCorpVaultReferences(t *testing.T) {
	v, _, _ := fakeOP(t, "")
	srv, seen := fakeHV(t)
	ctx := context.Background()
	v.SetManagers(ctx, "", Managers{HashiCorp: &HashiCorp{Addr: srv.URL, Token: "hvs.root", Namespace: "family"}})
	v.Set(ctx, "typesafe.key", "vault://secret/data/home/mail#password")
	if got, err := v.Get(ctx, "typesafe.key"); err != nil || got != "hv-value" {
		t.Fatalf("got %q %v", got, err)
	}
	if (*seen)[0] != "GET /v1/secret/data/home/mail ns=family" {
		t.Fatalf("seen %v", *seen)
	}
	for ref, want := range map[string]string{
		"vault://secret/data/home/mail#user": "has no field user",
		"vault://secret/data/home/mail#port": "is not text",
		"vault://secret/data/other#password": "not found in HashiCorp Vault",
	} {
		if err := v.Check(ctx, "", ref); err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), ref) {
			t.Errorf("%s: %v", ref, err)
		}
	}
	if err := v.TestManager(ctx, "", "hashicorp"); err != nil {
		t.Fatal(err)
	}
	v.SetManagers(ctx, "", Managers{HashiCorp: &HashiCorp{Addr: srv.URL, Token: "wrong"}})
	if err := v.Check(ctx, "", "vault://secret/data/home/mail#password"); err == nil || !strings.Contains(err.Error(), "refused the credentials (403)") {
		t.Fatalf("wrong token: %v", err)
	}
	// AppRole signs in once and keeps the token while it lasts.
	v.SetManagers(ctx, "", Managers{HashiCorp: &HashiCorp{Addr: srv.URL, RoleID: "role", SecretID: "sid"}})
	*seen = nil
	v.Check(ctx, "", "vault://secret/data/home/mail#password")
	if err := v.Check(ctx, "", "vault://secret/data/home/mail#password"); err != nil {
		t.Fatal(err)
	}
	if len(*seen) != 3 || (*seen)[0] != "POST /v1/auth/approle/login ns=" {
		t.Fatalf("approle: %v", *seen)
	}
}

func TestPasswordManagersMustBeReachedOverHTTPS(t *testing.T) {
	v, _, _ := fakeOP(t, "")
	srv, _ := fakeHV(t)
	ctx := context.Background()
	hv := func(addr string) Managers { return Managers{HashiCorp: &HashiCorp{Addr: addr, Token: "hvs.root"}} }
	for _, addr := range []string{"http://vault.example.com:8200", "ftp://vault", "https://user:pw@vault.example.com", "vault.example.com"} {
		if err := v.SetManagers(ctx, "", hv(addr)); err == nil {
			t.Errorf("%s was allowed", addr)
		}
	}
	if err := v.SetManagers(ctx, "", hv("https://vault.example.com:8200")); err != nil {
		t.Fatal(err)
	}
	// Plain http only to this computer, and only for the house.
	if err := v.SetManagers(ctx, "", hv(srv.URL)); err != nil {
		t.Fatal(err)
	}
	if err := v.SetManagers(ctx, "ana", hv(srv.URL)); err == nil {
		t.Fatal("a person reached plain http")
	}
	if err := v.SetManagers(ctx, "", Managers{OnePassword: &OnePassword{ConnectURL: "http://connect.example.com", ConnectToken: "t"}}); err == nil {
		t.Fatal("Connect over plain http was allowed")
	}
	// Removing every manager removes the stored credentials.
	v.SetManagers(ctx, "", Managers{})
	if _, err := v.Raw(ctx, "pm"); !errors.Is(err, ErrNotFound) {
		t.Fatal("credentials were kept")
	}
}

func TestOnePasswordConnectReferences(t *testing.T) {
	v, _, _ := fakeOP(t, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer connect-token" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/v1/vaults":
			if f := r.URL.Query().Get("filter"); f != "" && f != `name eq "Home"` {
				w.Write([]byte(`[]`))
				return
			}
			w.Write([]byte(`[{"id":"v1","name":"Home"}]`))
		case "/v1/vaults/v1/items":
			if r.URL.Query().Get("filter") != `title eq "Mail"` {
				w.Write([]byte(`[]`))
				return
			}
			w.Write([]byte(`[{"id":"i1"}]`))
		case "/v1/vaults/v1/items/i1":
			w.Write([]byte(`{"fields":[{"id":"password","label":"password","value":"connect-value"},{"id":"x","label":"pin","value":"1234","section":{"id":"s1","label":"extra"}}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	if err := v.SetManagers(ctx, "", Managers{OnePassword: &OnePassword{ConnectURL: srv.URL}}); err == nil {
		t.Fatal("Connect without a token was allowed")
	}
	v.SetManagers(ctx, "", Managers{OnePassword: &OnePassword{ConnectURL: srv.URL, ConnectToken: "connect-token"}})
	for ref, want := range map[string]string{"op://Home/Mail/password": "connect-value", "op://Home/Mail/extra/pin": "1234"} {
		if got, err := v.Resolve(ctx, "", ref); err != nil || got != want {
			t.Errorf("%s: %q %v", ref, got, err)
		}
	}
	if err := v.Check(ctx, "", "op://Work/Mail/password"); err == nil || !strings.Contains(err.Error(), "no vault Work") {
		t.Fatalf("other vault: %v", err)
	}
	if err := v.Check(ctx, "", "op://Home/Mail/other/pin"); err == nil || !strings.Contains(err.Error(), "has no field pin") {
		t.Fatalf("other section: %v", err)
	}
	if err := v.TestManager(ctx, "", "onepassword"); err != nil {
		t.Fatal(err)
	}
}
