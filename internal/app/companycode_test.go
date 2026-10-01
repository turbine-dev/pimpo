package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/connector/services"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/workspace"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return string(out)
}

// shopRepo stands in for a repository on GitHub.
func shopRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	seed := filepath.Join(dir, "seed")
	os.MkdirAll(seed, 0o700)
	gitIn(t, seed, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(seed, "cart.go"), []byte("package cart\n"), 0o600)
	gitIn(t, seed, "add", ".")
	gitIn(t, seed, "commit", "-q", "-m", "First")
	gitIn(t, dir, "clone", "-q", "--bare", seed, filepath.Join(dir, "shop.git"))
	return filepath.Join(dir, "shop.git")
}

func TestAMemberCodesOnItsBranchAndOpensAPullRequest(t *testing.T) {
	remote := shopRepo(t)
	var mu sync.Mutex
	var prs []string
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == "POST" && r.URL.Path == "/repos/ana/shop/pulls" && r.Header.Get("Authorization") == "Bearer bia-token" {
			prs = append(prs, r.URL.Path)
			w.Write([]byte(`{"number":3,"html_url":"https://github.com/ana/shop/pull/3"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer gh.Close()
	services.BaseURL["github"] = gh.URL
	defer delete(services.BaseURL, "github")

	s := &script{}
	var ta *testApp
	var co string
	s.rules = []scriptRule{{"Fix the cart", func(r llm.AgentRequest) string {
		if err := rpc(r.MCPURL, 1, "code_workspace", map[string]any{"repo": "ana/shop", "instructions": "Make the cart keep items", "message": "Keep items in the cart\n\nlonger text"}); err != nil {
			return err.Error()
		}
		tasks, _ := ta.Companies.Tasks(context.Background(), co)
		branch := workspace.Branch("bia", strings.ToLower(tasks[0].ID))
		if err := rpc(r.MCPURL, 2, "github_pr_create", map[string]any{"repo": "ana/shop", "head": branch, "base": "main", "title": "Keep items in the cart"}); err != nil {
			return err.Error()
		}
		return "Opened the pull request."
	}}}
	ta, co = team(t, s)
	ta.Workspaces = workspace.Spaces{Root: t.TempDir(), Remote: func(string) string { return remote }}
	var seen llm.CodeRequest
	ta.Coder = llm.FakeCoder(func(_ context.Context, r llm.CodeRequest) (llm.Response, error) {
		seen = r
		os.WriteFile(filepath.Join(r.Dir, "cart.go"), []byte("package cart\n\nvar Items []string\n"), 0o600)
		return llm.Response{Text: "The cart keeps its items.", CostUSD: 0.2}, nil
	})
	ctx := context.Background()
	base := "/api/companies/" + co
	ta.do(t, "PUT", base+"/roles/dev", map[string]any{"title": "Developer", "capabilities": []string{"code.workspace", "github.pr_create"}})
	ta.do(t, "PUT", base+"/members/bia", map[string]any{"name": "Bia", "role": "dev", "reports_to": "rui", "coder": "claude:opus", "code_sandbox": true})
	ta.do(t, "PUT", base+"/members/bia/accounts/github", map[string]string{"token": "bia-token", "account_kind": "machine"})
	o, _ := ta.Companies.Org(ctx, co)
	o.CodeEnv = map[string]string{"SHOP_API": "http://localhost:9000"}
	if o, err := ta.Companies.Update(ctx, o.Company); err != nil || o.CodeEnv["SHOP_API"] == "" {
		t.Fatalf("code env: %v", err)
	}
	o, _ = ta.Companies.Org(ctx, co)
	task, err := ta.assign(people.With(ctx, people.OwnerID), o, company.CEO, company.Work{}, company.Task{Assignee: "bia", Title: "Cart", Objective: "Fix the cart", Acceptance: "Items stay"})
	if err != nil {
		t.Fatal(err)
	}
	branch := workspace.Branch("bia", strings.ToLower(task.ID))
	var done company.Work
	ta.waitFor(t, "Bia's work", func() bool {
		works, _ := ta.Companies.Works(ctx, co, 10)
		i := slices.IndexFunc(works, func(w company.Work) bool { return w.Task == task.ID && w.State == company.WorkDone })
		if i >= 0 {
			done = works[i]
		}
		return i >= 0
	})
	if done.Summary != "Opened the pull request." || len(prs) != 1 {
		t.Fatalf("work = %q, pull requests = %v", done.Summary, prs)
	}
	if log := gitIn(t, remote, "log", "--format=%an %s", branch); !strings.HasPrefix(log, "Bia Keep items in the cart\n") {
		t.Fatalf("the branch on the remote = %q", log)
	}
	if seen.Coder != "claude" || seen.Model != "opus" || !seen.Sandbox || !strings.Contains(seen.System, branch) || !strings.Contains(seen.System, "do not commit, push") {
		t.Fatalf("the coding CLI got %+v", seen)
	}
	env := strings.Join(seen.Env, "\n")
	if !strings.Contains(env, "SHOP_API=http://localhost:9000") || strings.Contains(env, "bia-token") || !strings.Contains(env, "GIT_CONFIG_GLOBAL=/dev/null") {
		t.Fatalf("the CLI's environment = %s", env)
	}
	if spend := ta.spendOf(ctx, o); spend.Members["bia"].Day < 0.2 {
		t.Fatalf("the CLI's cost was not Bia's: %+v", spend)
	}

	tree := filepath.Join(ta.Workspaces.Root, co, "trees", "bia", strings.ToLower(task.ID))
	if _, err := os.Stat(tree); err != nil {
		t.Fatal("the task's worktree is gone while the task is open")
	}
	ta.Companies.UpdateTask(ctx, task.ID, func(x *company.Task) { x.State = company.TaskDone })
	ta.pruneSpaces(ctx, o)
	if _, err := os.Stat(tree); err == nil {
		t.Fatal("a done task kept its worktree")
	}
	if code, _ := ta.do(t, "DELETE", base, nil); code != 200 {
		t.Fatal("delete")
	}
	if _, err := os.Stat(filepath.Join(ta.Workspaces.Root, co)); err == nil {
		t.Fatal("a deleted company's clones stayed")
	}
}

func TestCodingNeedsAMemberAndAWayItCanRun(t *testing.T) {
	ta, co := team(t, &script{})
	ta.Workspaces = workspace.Spaces{Root: t.TempDir(), Remote: func(string) string { return shopRepo(t) }}
	ta.Coder = llm.FakeCoder(func(context.Context, llm.CodeRequest) (llm.Response, error) { return llm.Response{}, nil })
	base := "/api/companies/" + co
	args := map[string]any{"repo": "ana/shop", "instructions": "x"}
	if _, err := (codeWorkspace{ta.App}).Call(context.Background(), "code.workspace", "", args); err == nil || !strings.Contains(err.Error(), "company member") {
		t.Fatalf("the person coded as a member: %v", err)
	}
	if code, _ := ta.do(t, "PUT", base+"/members/bia", map[string]any{"name": "Bia", "role": "dev", "reports_to": "rui", "coder": "vim"}); code != 400 {
		t.Fatal("an unknown coding CLI was saved")
	}
	ta.do(t, "PUT", base+"/members/bia", map[string]any{"name": "Bia", "role": "dev", "reports_to": "rui", "coder": "opencode:zen/qwen", "code_sandbox": true})
	if _, err := (codeWorkspace{ta.App}).Call(asMember(co+"/bia", "code.workspace"), "code.workspace", "", args); err == nil || !strings.Contains(err.Error(), "no sandbox") {
		t.Fatalf("opencode ran in a sandbox it does not have: %v", err)
	}
	ta.do(t, "PUT", base+"/members/bia", map[string]any{"name": "Bia", "role": "dev", "reports_to": "rui", "coder": "codex"})
	if _, err := (codeWorkspace{ta.App}).Call(asMember(co+"/bia", "code.workspace"), "code.workspace", "", args); err == nil || !strings.Contains(err.Error(), "GitHub") {
		t.Fatalf("coded without a GitHub account: %v", err)
	}
	o, _ := ta.Companies.Org(context.Background(), co)
	for _, bad := range []map[string]string{{"PATH": "/tmp"}, {"GIT_DIR": "x"}, {"ANTHROPIC_API_KEY": "k"}, {"lower": "x"}} {
		o.CodeEnv = bad
		if _, err := ta.Companies.Update(context.Background(), o.Company); err == nil {
			t.Errorf("%v was taken as a variable for coding", bad)
		}
	}
}
