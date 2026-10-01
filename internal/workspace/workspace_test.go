package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return string(out)
}

// origin is a repository standing in for GitHub, with a main branch.
func origin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	seed := filepath.Join(dir, "seed")
	bare := filepath.Join(dir, "app.git")
	os.MkdirAll(seed, 0o700)
	run(t, seed, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(seed, "README.md"), []byte("# app\n"), 0o600)
	run(t, seed, "add", ".")
	run(t, seed, "commit", "-q", "-m", "First")
	run(t, dir, "clone", "-q", "--bare", seed, bare)
	return bare
}

func TestAMemberWorksOnItsOwnBranch(t *testing.T) {
	remote := origin(t)
	s := Spaces{Root: t.TempDir(), Remote: func(string) string { return remote }}
	ctx := context.Background()
	sp, err := s.Open(ctx, "co1", "bia", "t1", "ana/app", "", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if sp.Base != "main" || sp.Branch != "pimpo/bia/t1" || !strings.HasPrefix(sp.Dir, s.Root) {
		t.Fatalf("space = %+v", sp)
	}
	os.WriteFile(filepath.Join(sp.Dir, "cart.go"), []byte("package cart\n"), 0o600)
	res, err := s.Finish(ctx, sp, "Bia", "bia@co1.pimpo.invalid", "Add the cart", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Pushed || !slices.Equal(res.Commits, []string{"Add the cart"}) || !slices.Equal(res.Files, []string{"cart.go"}) {
		t.Fatalf("result = %+v", res)
	}
	if log := run(t, remote, "log", "--format=%an <%ae> %s", "pimpo/bia/t1"); !strings.HasPrefix(log, "Bia <bia@co1.pimpo.invalid> Add the cart") {
		t.Fatalf("remote log = %q", log)
	}
	if cfg, _ := os.ReadFile(filepath.Join(sp.bare, "config")); strings.Contains(string(cfg), "tok") || strings.Contains(string(cfg), "Authorization") {
		t.Fatal("the token was written to the clone")
	}

	again, err := s.Open(ctx, "co1", "bia", "t1", "ana/app", "", "tok")
	if err != nil || again.Dir != sp.Dir {
		t.Fatalf("the same task opened another worktree: %+v %v", again, err)
	}
	if res, _ := s.Finish(ctx, again, "Bia", "bia@co1.pimpo.invalid", "", "tok"); len(res.Commits) != 1 {
		t.Fatalf("nothing new still lists the branch's work: %+v", res)
	}
	other, err := s.Open(ctx, "co1", "rui", "t2", "ana/app", "main", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(other.Dir, "cart.go")); err == nil {
		t.Fatal("another task saw Bia's work")
	}

	s.Prune(ctx, "co1", func(member, key string) bool { return key == "t2" })
	if _, err := os.Stat(sp.Dir); err == nil {
		t.Fatal("a finished task's worktree stayed")
	}
	if _, err := os.Stat(other.Dir); err != nil {
		t.Fatal("an open task's worktree was removed")
	}
	if b := run(t, sp.bare, "branch", "--list", "pimpo/bia/t1"); b != "" {
		t.Fatalf("its local branch stayed: %q", b)
	}

	for _, bad := range [][]string{{"co1", "bia", "t1", "../x/y"}, {"co1", "../bia", "t1", "ana/app"}, {"co1", "bia", "T 1", "ana/app"}} {
		if _, err := s.Open(ctx, bad[0], bad[1], bad[2], bad[3], "", ""); err == nil {
			t.Errorf("%v opened", bad)
		}
	}
	if _, err := s.Open(ctx, "co1", "bia", "t3", "ana/app", "../../etc", ""); err == nil {
		t.Error("a base outside branches")
	}
	if _, err := s.Open(ctx, "co1", "bia", "t4", "ana/app", "nope", ""); err == nil || !strings.Contains(err.Error(), "no branch") {
		t.Errorf("a base the repository does not have: %v", err)
	}
}

func TestTheTokenTravelsAsAHeaderOnly(t *testing.T) {
	sp := Space{remote: "https://github.com/ana/app.git"}
	env := strings.Join(sp.origin("secret"), " ")
	if !strings.Contains(env, "GIT_CONFIG_VALUE_1=Authorization: Basic ") || !strings.Contains(env, "GIT_CONFIG_COUNT=2") || strings.Contains(env, "secret") {
		t.Fatalf("env = %v", env)
	}
	if env := strings.Join(sp.origin(""), " "); strings.Contains(env, "Authorization") || !strings.Contains(env, "GIT_CONFIG_COUNT=1") {
		t.Fatalf("no token, no header: %v", env)
	}
}
