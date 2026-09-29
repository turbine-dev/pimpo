package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/repo"
	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/runtime"
	"github.com/turbine-dev/pimpo/internal/server"
)

// Routines in a repository: Pimpo writes its routines to a folder (a git
// repository the owner controls) and reads changes back. What comes back
// is checked like a compiled routine and waits for the owner.

type repoChange struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	New      bool     `json:"new"`
	Added    []string `json:"added"`
	Removed  []string `json:"removed"`
	Tests    int      `json:"tests"`
	Problems []string `json:"problems"`
	Hash     string   `json:"hash"`
}

type repoView struct {
	Path    string            `json:"path"`
	Git     bool              `json:"git"`
	Remote  bool              `json:"remote"`
	Head    string            `json:"head,omitempty"`
	Changes []repoChange      `json:"changes"`
	Broken  map[string]string `json:"broken"`
	Error   string            `json:"error,omitempty"`
}

func (a *App) repoRoutes() {
	a.Server.Handle("GET /api/repo", func(w http.ResponseWriter, r *http.Request) { server.WriteJSON(w, 200, a.repoView(r.Context(), "")) })
	a.Server.Handle("PUT /api/repo", a.putRepo)
	a.Server.Handle("POST /api/repo/export", a.exportRepo)
	a.Server.Handle("POST /api/repo/pull", a.pullRepo)
	a.Server.Handle("POST /api/repo/push", a.pushRepo)
	a.Server.Handle("POST /api/repo/apply/{id}", a.applyRepo)
}

func (a *App) repoPath(ctx context.Context) string {
	p, _ := a.Events.Get(ctx, "repo.path")
	return p
}

func (a *App) putRepo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	path := strings.TrimSpace(req.Path)
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}
	if path != "" {
		if !filepath.IsAbs(path) {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "use the folder's full path"})
			return
		}
		if st, err := os.Stat(path); err != nil || !st.IsDir() {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "that folder does not exist"})
			return
		}
	}
	a.Events.Put(r.Context(), "repo.path", path)
	a.Events.Put(r.Context(), "repo.notified", "")
	a.Events.Append(r.Context(), "repo.set", actor(r.Context()), map[string]string{"path": path})
	server.WriteJSON(w, 200, a.repoView(r.Context(), ""))
}

// repoView reads the folder and checks every routine that differs from
// the installed one.
func (a *App) repoView(ctx context.Context, errText string) repoView {
	v := repoView{Path: a.repoPath(ctx), Changes: []repoChange{}, Broken: map[string]string{}, Error: errText}
	if v.Path == "" {
		return v
	}
	v.Git = repo.IsGit(ctx, v.Path)
	if v.Git {
		v.Remote, v.Head = repo.HasRemote(ctx, v.Path), repo.Head(ctx, v.Path)
	}
	found, broken := repo.Read(v.Path)
	v.Broken = broken
	for _, id := range repo.Sorted(found) {
		if c, changed := a.repoCheck(ctx, id, found[id]); changed {
			v.Changes = append(v.Changes, c)
		}
	}
	return v
}

// repoCheck compares a routine from the repository with the installed one
// and checks it: manifest, tests, and what it really touches.
func (a *App) repoCheck(ctx context.Context, id string, r routine.Routine) (repoChange, bool) {
	c := repoChange{ID: id, Name: r.Name, Tests: len(r.Tests), Hash: repo.Hash(r), Added: []string{}, Removed: []string{}, Problems: []string{}}
	var before []string
	if cur, err := a.Store.Routine(ctx, id); err == nil {
		if repo.Hash(cur.Body) == c.Hash {
			return c, false
		}
		before = cur.Body.Manifest.Capabilities
	} else {
		c.New = true
	}
	for _, cap := range r.Manifest.Capabilities {
		if !slices.Contains(before, cap) {
			c.Added = append(c.Added, cap)
		}
	}
	for _, cap := range before {
		if !slices.Contains(r.Manifest.Capabilities, cap) {
			c.Removed = append(c.Removed, cap)
		}
	}
	if err := r.Manifest.Validate(); err != nil {
		c.Problems = append(c.Problems, "manifest: "+err.Error())
		return c, true
	}
	if err := r.Manifest.Starts(); err != nil {
		c.Problems = append(c.Problems, "manifest: "+err.Error())
	}
	ctx = routine.WithLibrary(ctx, func(ctx context.Context, id string) (runtime.Helper, error) { return a.Scheduler.Library(ctx, id) })
	for _, t := range r.Tests {
		if out := routine.Check(ctx, r, t.Name, t.Scenario); !out.Passed {
			c.Problems = append(c.Problems, "test "+t.Name+": "+strings.Join(out.Problems, "; "))
		}
	}
	_, problems := routine.Audit(ctx, r)
	c.Problems = append(c.Problems, problems...)
	return c, true
}

func (a *App) exportRepo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	path := a.repoPath(ctx)
	if path == "" {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "choose the repository folder first"})
		return
	}
	all, err := a.Store.Routines(ctx)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	for _, rt := range all {
		if err := repo.Write(path, rt.ID, rt.Body); err != nil {
			server.WriteError(w, err)
			return
		}
	}
	commit := ""
	if repo.IsGit(ctx, path) {
		if commit, err = repo.Commit(ctx, path, fmt.Sprintf("Pimpo: %d routines", len(all))); err != nil {
			server.WriteJSON(w, 200, map[string]any{"written": len(all), "view": a.repoView(ctx, err.Error())})
			return
		}
	}
	a.Events.Append(ctx, "repo.exported", actor(ctx), map[string]any{"routines": len(all), "commit": commit})
	server.WriteJSON(w, 200, map[string]any{"written": len(all), "commit": commit, "view": a.repoView(ctx, "")})
}

func (a *App) pullRepo(w http.ResponseWriter, r *http.Request) {
	server.WriteJSON(w, 200, a.pullAndCheck(r.Context()))
}

// pullAndCheck brings the repository up to date (fast-forward only, so a
// local edit is never overwritten) and lists what changed.
func (a *App) pullAndCheck(ctx context.Context) repoView {
	path := a.repoPath(ctx)
	errText := ""
	if path != "" && repo.IsGit(ctx, path) && repo.HasRemote(ctx, path) {
		if _, err := repo.Git(ctx, path, "pull", "--ff-only"); err != nil {
			errText = err.Error()
		}
	}
	return a.repoView(ctx, errText)
}

func (a *App) pushRepo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	path := a.repoPath(ctx)
	if path == "" || !repo.IsGit(ctx, path) || !repo.HasRemote(ctx, path) {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "this folder is not a git repository with a remote"})
		return
	}
	out, err := repo.Git(ctx, path, "push")
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 502, Msg: err.Error()})
		return
	}
	a.Events.Append(ctx, "repo.pushed", actor(ctx), map[string]string{"output": out})
	server.WriteJSON(w, 200, map[string]any{"pushed": true, "view": a.repoView(ctx, "")})
}

// applyRepo installs one routine from the repository after checking it
// again, so what is saved is exactly what passed.
func (a *App) applyRepo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	path := a.repoPath(ctx)
	found, broken := repo.Read(path)
	body, ok := found[id]
	if !ok {
		msg := "not in the repository"
		if b, isBroken := broken[id]; isBroken {
			msg = b
		}
		server.WriteError(w, server.StatusError{Status: 404, Msg: msg})
		return
	}
	c, changed := a.repoCheck(ctx, id, body)
	if !changed {
		server.WriteJSON(w, 200, map[string]any{"applied": false, "view": a.repoView(ctx, "")})
		return
	}
	if len(c.Problems) > 0 {
		server.WriteError(w, server.StatusError{Status: 409, Msg: strings.Join(c.Problems, "; ")})
		return
	}
	reason := "from the repository"
	if head := repo.Head(ctx, path); head != "" {
		reason += " at " + head
	}
	if _, err := a.Store.SaveRoutine(ctx, id, body, reason, actor(ctx)); err != nil {
		server.WriteError(w, err)
		return
	}
	a.Scheduler.Changed(ctx, id)
	a.Events.Append(ctx, "repo.applied", actor(ctx), map[string]any{"routine": id, "hash": c.Hash, "added": c.Added})
	server.WriteJSON(w, 200, map[string]any{"applied": true, "view": a.repoView(ctx, "")})
}

// repoLoop checks the repository now and then and tells the owner, once,
// when routines there changed.
func (a *App) repoLoop(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
		if a.repoPath(ctx) == "" {
			continue
		}
		v := a.pullAndCheck(ctx)
		var sig []string
		for _, c := range v.Changes {
			sig = append(sig, c.ID+"@"+c.Hash)
		}
		key := strings.Join(sig, ",")
		if last, _ := a.Events.Get(ctx, "repo.notified"); key == last {
			continue
		}
		a.Events.Put(ctx, "repo.notified", key)
		if len(v.Changes) > 0 {
			a.Channel.Notify(ctx, explore.Notice{Kind: "task", Text: i18n.N(ctx, "msg.repo.changes", len(v.Changes))})
		}
	}
}
