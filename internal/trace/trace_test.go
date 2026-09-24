package trace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRequiresTheEssentials(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte(`{"id":"x","request":"do it"}`), 0o644)
	if _, err := Load(bad); err == nil {
		t.Fatal("accepted a trace without calls or expectations")
	}
	good := filepath.Join(dir, "good.json")
	os.WriteFile(good, []byte(`{"id":"x","request":"r","now":"2026-09-24T07:00:00Z",
	 "calls":[{"capability":"gmail.search","args":{},"result":[1]},{"capability":"telegram.send","args":{"text":"hi"}}],
	 "expect":[{"capability":"telegram.send"}]}`), 0o644)
	tr, err := Load(good)
	if err != nil {
		t.Fatal(err)
	}
	if s := tr.Replay(); len(s.Responses) != 1 || s.Responses[0].Capability != "gmail.search" {
		t.Fatalf("replay responses %+v", s.Responses)
	}
}

func TestProofFixturesLoad(t *testing.T) {
	paths, _ := filepath.Glob("../../testdata/proof/*.json")
	for _, p := range paths {
		tr, err := Load(p)
		if err != nil {
			t.Fatal(err)
		}
		if tr.Holdout == nil || len(tr.Holdout.Expect) == 0 {
			t.Errorf("%s has no holdout scenario", p)
		}
	}
}
