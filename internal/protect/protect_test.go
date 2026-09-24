package protect

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func keys(t *testing.T) (pub, priv string) {
	p, k, _ := ed25519.GenerateKey(rand.Reader)
	return base64.StdEncoding.EncodeToString(p), base64.StdEncoding.EncodeToString(k)
}

func signed(t *testing.T, priv string, version int, entries ...Entry) []byte {
	l, err := Sign(List{Version: version, Updated: "2026-09-24", Entries: entries}, priv)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(l)
	return b
}

func TestGuardBlocksListedThings(t *testing.T) {
	pub, priv := keys(t)
	g := &Guard{Keys: []string{pub}}
	skill := Entry{ID: "s1", Kind: "skill", Value: strings.Repeat("ab", 32), Reason: "steals wallets", Reports: 12}
	raw := signed(t, priv, 3,
		Entry{ID: "d1", Kind: "domain", Value: "collect-data.example", Reason: "exfiltration", Reports: 12},
		Entry{ID: "p1", Kind: "pattern", Capability: "gmail.send", Value: `seed phrase|recovery words`, Reason: "asks for wallet seeds", Reports: 4},
		skill,
	)
	if err := g.Load(raw); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		cap, scope string
		args       any
		want       string
	}{
		{"http.getJSON", "api.collect-data.example", "https://api.collect-data.example/x", "d1"},
		{"gmail.send", "", map[string]any{"to": "boss@collect-data.example", "body": "hi"}, "d1"},
		{"telegram.send", "", map[string]any{"text": "veja https://collect-data.example/login"}, "d1"},
		{"gmail.send", "", map[string]any{"to": "x@ok.com", "body": "Send me your SEED PHRASE"}, "p1"},
		{"telegram.send", "", map[string]any{"text": "seed phrase"}, ""},
		{"http.getJSON", "api.open-meteo.com", "https://api.open-meteo.com/v1", ""},
		{"gmail.send", "", map[string]any{"to": "a@notcollect-data.example"}, ""},
	} {
		e, hit := g.Check(c.cap, c.scope, c.args)
		if (c.want == "") == hit || (hit && e.ID != c.want) {
			t.Errorf("%s %v: got %v %s, want %q", c.cap, c.args, hit, e.ID, c.want)
		}
	}
	if _, hit := g.Skill([]byte("benign")); hit {
		t.Fatal("flagged a benign skill")
	}

	tampered := strings.Replace(string(raw), "collect-data.example", "collect-data.exampl3", 1)
	if err := g.Load([]byte(tampered)); err == nil {
		t.Fatal("accepted a tampered list")
	}
	_, other := keys(t)
	if err := g.Load(signed(t, other, 9)); err == nil {
		t.Fatal("accepted a list signed by an unknown key")
	}
	if err := g.Load(signed(t, priv, 2)); err == nil {
		t.Fatal("accepted an older list")
	}
	if v, n, _, _ := g.Status(); v != 3 || n != 3 {
		t.Fatalf("status %d %d", v, n)
	}
}

func TestSuggestionsCarryNoContent(t *testing.T) {
	e, err := Suggestion("domain", "Evil.Example ", "phishing in a fake invoice")
	if err != nil || e.Value != "evil.example" || e.Reports != 1 {
		t.Fatalf("%+v %v", e, err)
	}
	for _, bad := range []string{"https://evil.example/path", "user@evil.example", "evil"} {
		if _, err := Suggestion("domain", bad, "x"); err == nil {
			t.Errorf("accepted %q as a domain", bad)
		}
	}
	if _, err := Sign(List{Entries: []Entry{{ID: "x", Kind: "pattern", Value: "(", Reason: "r"}}}, func() string { _, k := keys(t); return k }()); err == nil {
		t.Fatal("signed an invalid pattern")
	}
}
