package gallery

import (
	"context"
	"encoding/json"
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
	b, _ := json.Marshal(ix)
	os.WriteFile(path, b, 0o644)
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
