package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/skills"
)

// Skills in the SKILL.md format (OpenClaw, Hermes, agentskills.io) are
// installed from a zip or a GitHub link, reviewed first, and then work as
// assistants: the agent follows the skill's text as a third party's
// reference, with only the capabilities the owner granted. A skill the
// protection list knows as malicious is refused, a skill changed on disk
// stops working until it is installed again, and scripts are never run.

type installedSkill struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Source       string    `json:"source"`
	Hash         string    `json:"hash"`
	Capabilities []string  `json:"capabilities"`
	Scripts      []string  `json:"scripts"`
	Unsupported  []string  `json:"unsupported"`
	Installed    time.Time `json:"installed"`
}

const skillsKey = "skills"

func (a *App) skillsDir() string { return filepath.Join(a.Home, "skills") }

func (a *App) installedSkills(ctx context.Context) []installedSkill {
	list := []installedSkill{}
	if raw, _ := a.Events.Get(ctx, skillsKey); raw != "" {
		json.Unmarshal([]byte(raw), &list)
	}
	return list
}

func (a *App) saveSkills(ctx context.Context, list []installedSkill) error {
	b, _ := json.Marshal(list)
	return a.Events.Put(ctx, skillsKey, string(b))
}

// skillAssistant is an installed skill as a role for explorations. It is
// read from disk each time and must still be what was installed.
func (a *App) skillAssistant(ctx context.Context, id string) (Assistant, error) {
	for _, s := range a.installedSkills(ctx) {
		if s.ID != id {
			continue
		}
		sk, err := skills.Read(filepath.Join(a.skillsDir(), id))
		if err != nil {
			return Assistant{}, err
		}
		if sk.Hash != s.Hash {
			return Assistant{}, errors.New("the skill " + s.Name + " changed on disk since it was installed; install it again to review it")
		}
		return Assistant{ID: "skill:" + id, Name: s.Name, Emoji: "🧩", Instructions: sk.Instructions(), Capabilities: s.Capabilities, Skill: true}, nil
	}
	return Assistant{}, errors.New("that skill is not installed")
}

func (a *App) skillRoutes() {
	a.Server.Handle("GET /api/skills", func(w http.ResponseWriter, r *http.Request) {
		server.WriteJSON(w, 200, a.installedSkills(r.Context()))
	})
	a.Server.Handle("POST /api/skills/preview", a.previewSkill)
	a.Server.Handle("POST /api/skills/install", a.installSkill)
	a.Server.Handle("PUT /api/skills/{id}", a.putSkill)
	a.Server.Handle("DELETE /api/skills/{id}", a.deleteSkill)
}

var stagingID = regexp.MustCompile(`^[0-9a-f]{24}$`)

// previewSkill unpacks a skill (a zip sent as "file", or {"url": a GitHub
// link}) into a staging folder and says what it is, without installing.
func (a *App) previewSkill(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ownerOnly(w, r) {
		return
	}
	if a.Home == "" {
		server.WriteError(w, server.StatusError{Status: 503, Msg: "no skills folder"})
		return
	}
	b := make([]byte, 12)
	rand.Read(b)
	token := hex.EncodeToString(b)
	dir := filepath.Join(a.skillsDir(), ".staging", token)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		server.WriteError(w, err)
		return
	}
	fail := func(status int, err error) {
		os.RemoveAll(dir)
		server.WriteError(w, server.StatusError{Status: status, Msg: err.Error()})
	}
	source := "zip"
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
		f, _, err := r.FormFile("file")
		if err != nil {
			fail(400, errors.New("send the skill as a .zip"))
			return
		}
		defer f.Close()
		data, _ := io.ReadAll(f)
		if err := skills.Unzip(data, dir); err != nil {
			fail(422, err)
			return
		}
	} else {
		var req struct {
			URL string `json:"url"`
		}
		if err := server.Decode(r, &req); err != nil {
			fail(400, err)
			return
		}
		if err := skills.FetchGitHub(ctx, req.URL, dir); err != nil {
			fail(422, err)
			return
		}
		source = strings.TrimSpace(req.URL)
	}
	sk, err := skills.Read(dir)
	if err != nil {
		fail(422, err)
		return
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "SKILL.md")); err == nil && a.Protect != nil {
		if e, bad := a.Protect.Skill(raw); bad {
			fail(422, errors.New("the protection list reports this skill: "+e.Reason))
			return
		}
	}
	os.WriteFile(filepath.Join(dir, ".source"), []byte(source), 0o600)
	var known []string
	for _, c := range sk.Suggested {
		if _, ok := capability.Catalog[c]; ok {
			known = append(known, c)
		}
	}
	sk.Suggested = known
	server.WriteJSON(w, 200, map[string]any{"token": token, "skill": sk, "exists": a.hasSkill(ctx, sk.ID)})
}

func (a *App) hasSkill(ctx context.Context, id string) bool {
	for _, s := range a.installedSkills(ctx) {
		if s.ID == id {
			return true
		}
	}
	return false
}

func validCaps(list []string) ([]string, error) {
	out := []string{}
	for _, c := range list {
		if _, ok := capability.Catalog[c]; !ok {
			return nil, errors.New("unknown capability " + c)
		}
		out = append(out, c)
	}
	return out, nil
}

// installSkill moves a previewed skill into place with the capabilities
// the owner chose. Installing one with the same id replaces it.
func (a *App) installSkill(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ownerOnly(w, r) {
		return
	}
	var req struct {
		Token        string   `json:"token"`
		Capabilities []string `json:"capabilities"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	if !stagingID.MatchString(req.Token) {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "preview the skill first"})
		return
	}
	caps, err := validCaps(req.Capabilities)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	staged := filepath.Join(a.skillsDir(), ".staging", req.Token)
	sk, err := skills.Read(staged)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "that preview is gone; preview the skill again"})
		return
	}
	src, _ := os.ReadFile(filepath.Join(staged, ".source"))
	os.Remove(filepath.Join(staged, ".source"))
	sk, _ = skills.Read(staged) // without .source, as it will be read later
	dest := filepath.Join(a.skillsDir(), sk.ID)
	os.RemoveAll(dest)
	if err := os.Rename(staged, dest); err != nil {
		server.WriteError(w, err)
		return
	}
	list := a.installedSkills(ctx)
	kept := list[:0]
	for _, s := range list {
		if s.ID != sk.ID {
			kept = append(kept, s)
		}
	}
	item := installedSkill{ID: sk.ID, Name: sk.Name, Description: sk.Description, Source: string(src), Hash: sk.Hash, Capabilities: caps,
		Scripts: sk.Scripts, Unsupported: sk.Unsupported, Installed: time.Now()}
	if err := a.saveSkills(ctx, append(kept, item)); err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(ctx, "skill.installed", actor(ctx), map[string]any{"id": sk.ID, "source": string(src), "hash": sk.Hash, "capabilities": caps})
	server.WriteJSON(w, 200, item)
}

func (a *App) putSkill(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ownerOnly(w, r) {
		return
	}
	var req struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	caps, err := validCaps(req.Capabilities)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	list := a.installedSkills(ctx)
	for i := range list {
		if list[i].ID == r.PathValue("id") {
			list[i].Capabilities = caps
			a.saveSkills(ctx, list)
			a.Events.Append(ctx, "skill.changed", actor(ctx), map[string]any{"id": list[i].ID, "capabilities": caps})
			server.WriteJSON(w, 200, list[i])
			return
		}
	}
	server.WriteError(w, server.StatusError{Status: 404, Msg: "no such skill"})
}

func (a *App) deleteSkill(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ownerOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	list := a.installedSkills(ctx)
	kept := list[:0]
	found := false
	for _, s := range list {
		if s.ID == id {
			found = true
			continue
		}
		kept = append(kept, s)
	}
	if !found || strings.ContainsAny(id, `/\.`) {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such skill"})
		return
	}
	a.saveSkills(ctx, kept)
	os.RemoveAll(filepath.Join(a.skillsDir(), id))
	a.Events.Append(ctx, "skill.removed", actor(ctx), map[string]string{"id": id})
	server.WriteJSON(w, 200, map[string]string{"state": "removed"})
}
