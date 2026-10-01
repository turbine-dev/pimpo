// Package workspace keeps the git worktrees company members code in: one
// clone per repository and company, one worktree and branch per member and
// task, under the company's folder.
package workspace

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	repoName = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	part     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
	ref      = regexp.MustCompile(`^[A-Za-z0-9_./-]{1,200}$`)
)

const gitTimeout = 5 * time.Minute

// Spaces is where the companies' clones and worktrees live.
type Spaces struct {
	Root string
	// Remote is where a repository is fetched from; GitHub unless a test
	// says otherwise.
	Remote func(repo string) string
}

// A Space is one member's worktree for one task.
type Space struct {
	Dir, Branch, Repo, Base string
	bare                    string
}

func (s Spaces) remote(repo string) string {
	if s.Remote != nil {
		return s.Remote(repo)
	}
	return "https://github.com/" + repo + ".git"
}

func (s Spaces) bare(company, repo string) string {
	return filepath.Join(s.Root, company, "repos", filepath.FromSlash(repo)+".git")
}

func (s Spaces) tree(company, member, key string) string {
	return filepath.Join(s.Root, company, "trees", member, key)
}

// Branch is the branch a member's work on a task goes to.
func Branch(member, key string) string { return "pimpo/" + member + "/" + key }

// Open fetches the repository and gives the member's worktree for key,
// made from base (the repository's default branch when empty) the first
// time and kept as it was after that.
func (s Spaces) Open(ctx context.Context, company, member, key, repo, base, token string) (Space, error) {
	if !repoName.MatchString(repo) || strings.Contains(repo, "..") {
		return Space{}, errors.New("repo must look like owner/name")
	}
	for _, p := range []string{company, member, key} {
		if !part.MatchString(p) {
			return Space{}, fmt.Errorf("%q cannot name a folder", p)
		}
	}
	if base != "" && (!ref.MatchString(base) || strings.Contains(base, "..")) {
		return Space{}, errors.New("base is a branch name")
	}
	sp := Space{Dir: s.tree(company, member, key), Branch: Branch(member, key), Repo: repo, bare: s.bare(company, repo)}
	if _, err := os.Stat(sp.bare); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(sp.bare), 0o700); err != nil {
			return sp, err
		}
		if _, err := git(ctx, nil, "", "init", "--bare", "-q", sp.bare); err != nil {
			return sp, err
		}
		if _, err := git(ctx, nil, sp.bare, "remote", "add", "origin", s.remote(repo)); err != nil {
			return sp, err
		}
	}
	if _, err := git(ctx, auth(token), sp.bare, "fetch", "-q", "--prune", "origin", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return sp, fmt.Errorf("fetching %s: %w", repo, err)
	}
	if base == "" {
		head, err := git(ctx, auth(token), sp.bare, "ls-remote", "--symref", "origin", "HEAD")
		if err != nil {
			return sp, err
		}
		base = defaultBranch(head)
		if base == "" {
			return sp, errors.New("the repository has no default branch yet")
		}
	}
	sp.Base = base
	if _, err := os.Stat(sp.Dir); err == nil {
		return sp, nil
	}
	if err := os.MkdirAll(filepath.Dir(sp.Dir), 0o700); err != nil {
		return sp, err
	}
	if _, err := git(ctx, nil, sp.bare, "rev-parse", "--verify", "-q", "refs/heads/"+sp.Branch); err == nil {
		_, err = git(ctx, nil, sp.bare, "worktree", "add", "-q", sp.Dir, sp.Branch)
		return sp, err
	}
	_, err := git(ctx, nil, sp.bare, "worktree", "add", "-q", "--no-track", "-b", sp.Branch, sp.Dir, "origin/"+base)
	return sp, err
}

func defaultBranch(lsRemote string) string {
	for _, line := range strings.Split(lsRemote, "\n") {
		if rest, ok := strings.CutPrefix(line, "ref: refs/heads/"); ok {
			b, _, _ := strings.Cut(rest, "\t")
			return b
		}
	}
	return ""
}

// A Result is what a stretch of work left on its branch.
type Result struct {
	Commits []string `json:"commits"`
	Files   []string `json:"files"`
	Pushed  bool     `json:"pushed"`
}

// Finish commits what is left in the worktree as the member, and pushes
// the branch when it has commits the base does not.
func (s Spaces) Finish(ctx context.Context, sp Space, name, email, message, token string) (Result, error) {
	var res Result
	if _, err := git(ctx, nil, sp.Dir, "add", "-A"); err != nil {
		return res, err
	}
	if _, err := git(ctx, nil, sp.Dir, "diff", "--cached", "--quiet"); err != nil {
		if strings.TrimSpace(message) == "" {
			message = "Work on " + sp.Branch
		}
		// Hooks and signing are the person's; the member's commits use neither.
		if _, err := git(ctx, nil, sp.Dir, "-c", "user.name="+name, "-c", "user.email="+email, "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false",
			"commit", "-q", "-m", message); err != nil {
			return res, err
		}
	}
	log, err := git(ctx, nil, sp.Dir, "log", "--format=%s", "origin/"+sp.Base+"..HEAD")
	if err != nil {
		return res, err
	}
	res.Commits = lines(log)
	files, err := git(ctx, nil, sp.Dir, "diff", "--name-only", "origin/"+sp.Base+"...HEAD")
	if err != nil {
		return res, err
	}
	res.Files = lines(files)
	if len(res.Commits) == 0 {
		return res, nil
	}
	if _, err := git(ctx, auth(token), sp.Dir, "-c", "core.hooksPath=/dev/null", "push", "-q", "origin", "HEAD:refs/heads/"+sp.Branch); err != nil {
		return res, fmt.Errorf("pushing %s: %w", sp.Branch, err)
	}
	res.Pushed = true
	return res, nil
}

// Prune removes a company's worktrees whose key keep does not want, with
// their local branches; what was pushed stays on the remote.
func (s Spaces) Prune(ctx context.Context, company string, keep func(member, key string) bool) error {
	members, err := os.ReadDir(filepath.Join(s.Root, company, "trees"))
	if err != nil {
		return nil
	}
	for _, m := range members {
		keys, _ := os.ReadDir(filepath.Join(s.Root, company, "trees", m.Name()))
		for _, k := range keys {
			if keep(m.Name(), k.Name()) {
				continue
			}
			dir := s.tree(company, m.Name(), k.Name())
			common, err := git(ctx, nil, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
			os.RemoveAll(dir)
			if err == nil {
				bare := strings.TrimSpace(common)
				git(ctx, nil, bare, "worktree", "prune")
				git(ctx, nil, bare, "branch", "-q", "-D", Branch(m.Name(), k.Name()))
			}
		}
	}
	return nil
}

// Drop removes everything a company has on disk.
func (s Spaces) Drop(company string) error {
	if !part.MatchString(company) {
		return nil
	}
	return os.RemoveAll(filepath.Join(s.Root, company))
}

// auth hands git the token as a header for this command only: it is never
// written to the clone's config nor seen on a command line.
func auth(token string) []string {
	if token == "" {
		return nil
	}
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	return []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.https://github.com/.extraheader", "GIT_CONFIG_VALUE_0=Authorization: Basic " + basic}
}

// git runs without the person's own git config, so neither their
// credential helpers nor their identity reach a member's work.
func git(ctx context.Context, env []string, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "LANG=C"}, env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return stdout.String(), err
		}
		return stdout.String(), errors.New(msg)
	}
	return stdout.String(), nil
}

func lines(s string) []string {
	out := []string{}
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
