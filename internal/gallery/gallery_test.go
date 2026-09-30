package gallery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/runtime"
	"github.com/turbine-dev/pimpo/internal/trace"
)

var brief = routine.Routine{
	Name: "Resumo de não lidos", Description: "Conta os e-mails não lidos e me avisa.",
	Manifest: runtime.Manifest{Schedule: "0 7 * * *", Capabilities: []string{"gmail.search", "telegram.send"}},
	Code:     `async function run() { const m = await gmail.search({query: "is:unread"}); await telegram.send({text: m.length + " não lidos"}) }`,
	Tests: []routine.Test{{Name: "dois", Scenario: trace.Scenario{
		Responses: []trace.Response{{Capability: "gmail.search", Result: json.RawMessage(`[{"id":"1","unread":true},{"id":"2","unread":true}]`)}},
		Expect:    []trace.Expect{{Capability: "telegram.send", Contains: []string{"2 não lidos"}}}}}},
}

// testRoot stands in for the gallery root key in these tests.
var testRoot string

func init() {
	pub, priv, _ := Keygen()
	testRoot = priv
	RootKeys = append(RootKeys, pub)
}

func signedJSON(t *testing.T, ix Index) []byte {
	ix, err := SignAuthors(ix, testRoot)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(ix)
	return b
}

func index(t *testing.T) (Index, string) {
	pub, priv, err := Keygen()
	if err != nil {
		t.Fatal(err)
	}
	return Index{Authors: map[string]Author{"dener": {Name: "Dener", Key: pub}}}, priv
}

func TestSignedEntryVerifies(t *testing.T) {
	ix, priv := index(t)
	e, err := Sign("resumo-nao-lidos", "dener", brief, priv)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := Sign("resumo-nao-lidos", "dener", brief, priv); again.Hash != e.Hash {
		t.Fatal("the same routine gave two hashes")
	}
	ix.Entries = append(ix.Entries, e)
	r := ix.Verify(context.Background(), e)
	if !r.Verified || strings.Join(r.Uses, ",") != "gmail.search,telegram.send" || r.Sends || r.Risk != "notify" {
		t.Fatalf("%+v", r)
	}
	path := filepath.Join(t.TempDir(), "index.json")
	os.WriteFile(path, signedJSON(t, ix), 0o644)
	loaded, err := Load(context.Background(), path)
	if err != nil || !loaded.Verify(context.Background(), loaded.Entries[0]).Verified {
		t.Fatalf("round trip: %v", err)
	}
}

func TestTamperingIsCaught(t *testing.T) {
	ix, priv := index(t)
	e, _ := Sign("resumo", "dener", brief, priv)
	ctx := context.Background()
	problems := func(e Entry, ix Index) string { return strings.Join(ix.Verify(ctx, e).Problems, "\n") }

	changed := e
	changed.Routine.Code = strings.Replace(brief.Code, "telegram.send", "gmail.send", 1)
	if p := problems(changed, ix); !strings.Contains(p, "does not match its hash") {
		t.Fatalf("changed code: %s", p)
	}
	rehashed := changed
	rehashed.Hash = Hash(changed.Routine)
	if p := problems(rehashed, ix); !strings.Contains(p, "signature does not match") || !strings.Contains(p, "calls gmail.send without declaring it") {
		t.Fatalf("rehashed: %s", p)
	}
	_, other, _ := Keygen()
	forged, _ := Sign("resumo", "dener", brief, other)
	if p := problems(forged, ix); !strings.Contains(p, "signature does not match") {
		t.Fatalf("forged: %s", p)
	}
	stranger, _ := Sign("resumo", "mallory", brief, priv)
	if p := problems(stranger, ix); !strings.Contains(p, "unknown author") {
		t.Fatalf("stranger: %s", p)
	}
	renamed := e
	renamed.ID = "outro"
	if p := problems(renamed, ix); !strings.Contains(p, "signature does not match") {
		t.Fatalf("renamed: %s", p)
	}
	ix.Revoked = []string{e.Hash}
	if p := problems(e, ix); !strings.Contains(p, "removed from the gallery") {
		t.Fatalf("revoked: %s", p)
	}
	broken := brief
	broken.Code = `async function run() { await telegram.send({text: "oi"}) }`
	b, _ := Sign("quebrada", "dener", broken, priv)
	if p := problems(b, Index{Authors: ix.Authors}); !strings.Contains(p, `test "dois" fails`) {
		t.Fatalf("broken test: %s", p)
	}
}

func TestSendsOutside(t *testing.T) {
	r := brief
	if sends, _ := Sends(r); sends {
		t.Fatal("reading email and telling the owner is not sending out")
	}
	r.Manifest.Capabilities = []string{"gmail.search", "http.getJSON:attacker.example", "telegram.send"}
	if sends, outside := Sends(r); !sends || strings.Join(outside, ",") != "attacker.example" {
		t.Fatalf("inbox plus an outside host: %v %v", sends, outside)
	}
	r.Manifest.Capabilities = []string{"http.getJSON:api.open-meteo.com", "notify.send"}
	if sends, _ := Sends(r); sends {
		t.Fatal("a weather routine reads nothing of the owner's")
	}
}

func TestPlainHTTPIndexIsRefused(t *testing.T) {
	if _, err := Load(context.Background(), "http://gallery.example.com/index.json"); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("%v", err)
	}
	signed := signedJSON(t, Index{Authors: map[string]Author{}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(signed) }))
	defer srv.Close()
	if _, err := Load(context.Background(), srv.URL); err != nil {
		t.Fatalf("loopback: %v", err)
	}
}

// Whoever serves the index cannot add an author or change one's key: the
// authors list must be signed by a root key built into Pimpo.
func TestAuthorsMustBeSignedByARootKey(t *testing.T) {
	ix, _ := index(t)
	raw := signedJSON(t, ix)
	if _, err := Parse(raw); err != nil {
		t.Fatalf("a signed index was refused: %v", err)
	}
	var signed Index
	json.Unmarshal(raw, &signed)
	mallory, _, _ := Keygen()
	for name, change := range map[string]func(*Index){
		"unsigned":    func(ix *Index) { ix.AuthorsSignature = "" },
		"garbage":     func(ix *Index) { ix.AuthorsSignature = "bm90IGEgc2lnbmF0dXJl" },
		"swapped key": func(ix *Index) { ix.Authors["dener"] = Author{Name: "Dener", Key: mallory} },
		"added":       func(ix *Index) { ix.Authors["mallory"] = Author{Name: "Mallory", Key: mallory} },
		"renamed":     func(ix *Index) { ix.Authors["dener"] = Author{Name: "Dener (official)", Key: ix.Authors["dener"].Key} },
		"other key": func(ix *Index) {
			_, other, _ := Keygen()
			*ix, _ = SignAuthors(*ix, other)
		},
	} {
		var bad Index
		json.Unmarshal(raw, &bad)
		change(&bad)
		b, _ := json.Marshal(bad)
		if _, err := Parse(b); err == nil || !strings.Contains(err.Error(), "not signed") {
			t.Errorf("%s: accepted (%v)", name, err)
		}
	}
	// Any of the root keys will do, so a new one can ship before the old
	// one retires.
	if err := signed.CheckAuthors([]string{"not a key", RootKeys[len(RootKeys)-1]}); err != nil {
		t.Fatal(err)
	}
	if err := signed.CheckAuthors(RootKeys[:len(RootKeys)-1]); err == nil {
		t.Fatal("verified without the key that signed it")
	}
}
