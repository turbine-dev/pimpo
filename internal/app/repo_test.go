package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denerFernandes/pimpo/internal/repo"
	"github.com/denerFernandes/pimpo/internal/routine"
	"github.com/denerFernandes/pimpo/internal/runtime"
	"github.com/denerFernandes/pimpo/internal/trace"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=T", "-c", "user.email=t@t", "-c", "init.defaultBranch=main"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func hello(text string, caps ...string) routine.Routine {
	count := 1
	return routine.Routine{Name: "Bom dia", Description: "Manda bom dia.",
		Manifest: runtime.Manifest{Schedule: "0 7 * * *", Capabilities: append([]string{"telegram.send"}, caps...)},
		Code:     `async function run() { await telegram.send({text: "` + text + `"}); }`,
		Tests:    []routine.Test{{Name: "manda", Scenario: trace.Scenario{Now: "2026-09-25T10:00:00Z", Expect: []trace.Expect{{Capability: "telegram.send", Count: &count, Contains: []string{text}}}}}}}
}

// Routines go to a repository and come back only after their checks and
// the owner's click; a routine that grew what it touches says so.
func TestRoutinesThroughARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	ta := newApp(t, nil, nil)
	ctx := context.Background()
	ta.Store.SaveRoutine(ctx, "bom-dia", hello("Bom dia!"), "test", "human:owner")

	base := t.TempDir()
	remote, mine, other := filepath.Join(base, "remote.git"), filepath.Join(base, "mine"), filepath.Join(base, "other")
	git(t, base, "init", "--bare", remote)
	git(t, base, "clone", remote, mine)
	if code, _ := ta.do(t, "PUT", "/api/repo", map[string]string{"path": mine}); code != 200 {
		t.Fatal("set path", code)
	}
	code, out := ta.do(t, "POST", "/api/repo/export", nil)
	if code != 200 || out["written"] != float64(1) || out["commit"] == "" {
		t.Fatalf("export %d %v", code, out)
	}
	if _, err := os.Stat(filepath.Join(mine, "routines", "bom-dia", "routine.js")); err != nil {
		t.Fatal(err)
	}
	if code, _ := ta.do(t, "POST", "/api/repo/push", nil); code != 200 {
		t.Fatal("push", code)
	}

	// Someone edits the routine elsewhere: new text, and it now also reads email.
	git(t, base, "clone", remote, other)
	changed := hello("Bom dia, equipe!", "gmail.search")
	changed.Code = `async function run() { await gmail.search({query: "is:unread"}); await telegram.send({text: "Bom dia, equipe!"}); }`
	repo.Write(other, "bom-dia", changed)
	broken := hello("x")
	broken.Tests[0].Expect[0].Contains = []string{"nunca enviado"}
	repo.Write(other, "quebrada", broken)
	git(t, other, "add", ".")
	git(t, other, "commit", "-m", "edit")
	git(t, other, "push")

	_, view := ta.do(t, "POST", "/api/repo/pull", nil)
	b, _ := json.Marshal(view)
	var v repoView
	json.Unmarshal(b, &v)
	if len(v.Changes) != 2 {
		t.Fatalf("changes %s", b)
	}
	byID := map[string]repoChange{}
	for _, c := range v.Changes {
		byID[c.ID] = c
	}
	if c := byID["bom-dia"]; c.New || len(c.Added) != 1 || c.Added[0] != "gmail.search" || len(c.Problems) != 0 {
		t.Fatalf("bom-dia %+v", c)
	}
	if c := byID["quebrada"]; !c.New || len(c.Problems) == 0 {
		t.Fatalf("quebrada %+v", c)
	}
	if cur, _ := ta.Store.Routine(ctx, "bom-dia"); strings.Contains(cur.Body.Code, "equipe") {
		t.Fatal("applied without the owner")
	}
	if code, _ := ta.do(t, "POST", "/api/repo/apply/quebrada", nil); code != 409 {
		t.Fatalf("installed a routine that fails its tests: %d", code)
	}
	if code, _ := ta.do(t, "POST", "/api/repo/apply/bom-dia", nil); code != 200 {
		t.Fatal("apply", code)
	}
	cur, _ := ta.Store.Routine(ctx, "bom-dia")
	if !strings.Contains(cur.Body.Code, "equipe") || cur.Version != 2 {
		t.Fatalf("after apply: v%d %s", cur.Version, cur.Body.Code)
	}
	if code, _ := ta.do(t, "PUT", "/api/repo", map[string]string{"path": "relative/dir"}); code != 400 {
		t.Fatal("accepted a relative path")
	}
}
