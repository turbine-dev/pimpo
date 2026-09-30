package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	starter "github.com/turbine-dev/pimpo/gallery"
	"github.com/turbine-dev/pimpo/internal/gallery"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/store"
	"github.com/turbine-dev/pimpo/internal/vault"
)

func (a *App) galleryRoutes() {
	a.Server.Handle("GET /api/gallery", a.listGallery)
	a.Server.Handle("POST /api/gallery/{id}/install", a.installFromGallery)
	a.Server.Handle("POST /api/routines/{id}/publish", a.publishRoutine)
	a.Server.Handle("POST /api/routines/{id}/update", a.updateFromGallery)
}

type galleryItem struct {
	gallery.Entry
	AuthorName string         `json:"author_name"`
	Report     gallery.Report `json:"report"`
	Installed  bool           `json:"installed"`
}

var galleryCache struct {
	sync.Mutex
	src   string
	at    time.Time
	index gallery.Index
}

// galleryIndex loads the index, keeping it for ten minutes; fresh forces a
// reload, which installing always does.
func (a *App) galleryIndex(ctx context.Context, fresh bool) (gallery.Index, error) {
	src := a.Settings(ctx).GalleryURL
	if src == "" {
		src = gallery.DefaultIndex
	}
	galleryCache.Lock()
	defer galleryCache.Unlock()
	if !fresh && galleryCache.src == src && time.Since(galleryCache.at) < 10*time.Minute {
		return galleryCache.index, nil
	}
	ix, err := gallery.Load(ctx, src)
	if err != nil && src == gallery.DefaultIndex {
		// Offline, or the community index moved: the starter set that came
		// with this version still verifies on its own.
		ix, err = gallery.Parse(starter.Index)
	}
	if err != nil {
		return gallery.Index{}, err
	}
	galleryCache.src, galleryCache.at, galleryCache.index = src, time.Now(), ix
	return ix, nil
}

func (a *App) listGallery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ix, err := a.galleryIndex(ctx, r.URL.Query().Get("fresh") != "")
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 502, Msg: "could not load the gallery: " + err.Error()})
		return
	}
	installed := map[string]bool{}
	if list, err := a.myRoutines(ctx); err == nil {
		for _, rt := range list {
			installed[gallery.Hash(rt.Body)] = true
		}
	}
	out := make([]galleryItem, 0, len(ix.Entries))
	for _, e := range ix.Entries {
		out = append(out, galleryItem{Entry: e, AuthorName: ix.Authors[e.Author].Name, Report: ix.Verify(ctx, e), Installed: installed[e.Hash]})
	}
	server.WriteJSON(w, 200, out)
}

func (a *App) installFromGallery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ix, err := a.galleryIndex(ctx, true)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 502, Msg: "could not load the gallery: " + err.Error()})
		return
	}
	e, ok := ix.Find(r.PathValue("id"))
	if !ok {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such routine in the gallery"})
		return
	}
	rep := ix.Verify(ctx, e)
	if !rep.Verified {
		server.WriteError(w, server.StatusError{Status: 422, Msg: "not installed: " + strings.Join(rep.Problems, "; ")})
		return
	}
	if err := a.trustAuthor(ctx, ix, e); err != nil {
		server.WriteError(w, server.StatusError{Status: 422, Msg: "not installed: " + err.Error()})
		return
	}
	var req struct {
		Confirm bool `json:"confirm"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if rep.Sends && !req.Confirm {
		server.WriteError(w, server.StatusError{Status: 409, Msg: "this routine can send your data out of Pimpo; confirm to install it"})
		return
	}
	// Someone else's copy is theirs: this person gets their own.
	id := e.ID
	if existing, err := a.Store.Routine(ctx, id); err == nil && !mine(ctx, existing.Person) {
		id += "-" + people.From(ctx)
	}
	if existing, err := a.Store.Routine(ctx, id); err == nil && (!mine(ctx, existing.Person) || gallery.Hash(existing.Body) != e.Hash) {
		id += "-" + e.Hash[:6]
	}
	rt, err := a.Store.SaveRoutine(ctx, id, e.Routine, "installed from the gallery: "+e.Author+" "+e.Hash[:12], actor(ctx))
	if err != nil {
		server.WriteError(w, err)
		return
	}
	if p := people.From(ctx); p != people.OwnerID {
		a.Store.SetRoutinePerson(ctx, id, p)
		rt.Person = p
	}
	a.Events.Put(ctx, "gallery.origin."+rt.ID, e.ID)
	a.rememberGalleryVersion(ctx, ix, rt.ID, e)
	a.Events.Append(ctx, "gallery.installed", actor(ctx), map[string]any{"routine": rt.ID, "author": e.Author, "hash": e.Hash, "sends": rep.Sends})
	a.Scheduler.Changed(ctx, rt.ID)
	server.WriteJSON(w, 200, a.summary(ctx, rt))
}

// publishRoutine signs one of the owner's routines with their author key
// (made on first use and kept in the vault) so it can be proposed to the
// gallery by pull request.
func (a *App) publishRoutine(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Author string `json:"author"`
	}
	server.Decode(r, &req)
	author := strings.ToLower(strings.TrimSpace(req.Author))
	if author == "" {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "choose an author name"})
		return
	}
	rt, err := a.myRoutine(ctx, r.PathValue("id"))
	if err != nil {
		server.WriteError(w, notFound(err))
		return
	}
	if len(rt.Body.Tests) == 0 {
		server.WriteError(w, server.StatusError{Status: 422, Msg: "a routine needs tests before it can be shared"})
		return
	}
	pub, priv, err := a.authorKey(ctx)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	e, err := gallery.Sign(rt.ID, author, rt.Body, priv)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(ctx, "gallery.signed", "human:owner", map[string]string{"routine": rt.ID, "hash": e.Hash})
	server.WriteJSON(w, 200, map[string]any{"entry": e, "author": gallery.Author{Name: req.Author, Key: pub}})
}

func (a *App) authorKey(ctx context.Context) (string, string, error) {
	priv, err := a.Vault.Get(ctx, "gallery.key")
	pub, _ := a.Events.Get(ctx, "gallery.public_key")
	if err == nil && pub != "" {
		return pub, priv, nil
	}
	if err != nil && !errors.Is(err, vault.ErrNotFound) {
		return "", "", err
	}
	pub, priv, err = gallery.Keygen()
	if err != nil {
		return "", "", err
	}
	if err := a.Vault.Set(ctx, "gallery.key", priv); err != nil {
		return "", "", err
	}
	return pub, priv, a.Events.Put(ctx, "gallery.public_key", pub)
}

// galleryOrigin is the gallery entry a routine was installed from, if any.
// Installs from before this was recorded are recognized by their version
// note and matching id.
func (a *App) galleryOrigin(ctx context.Context, rt store.Routine) string {
	if id, _ := a.Events.Get(ctx, "gallery.origin."+rt.ID); id != "" {
		return id
	}
	versions, _ := a.Store.Versions(ctx, rt.ID)
	for _, v := range versions {
		if strings.HasPrefix(v.Reason, "installed from the gallery") || strings.HasPrefix(v.Reason, "updated from the gallery") {
			return rt.ID
		}
	}
	return ""
}

type galleryUpdate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Settings are the labels of parameters the new version adds.
	Settings []string `json:"settings"`
}

// pendingUpdate says whether the gallery has a different version of the
// routine it came from. It uses the cached index; applying the update
// verifies again.
func (a *App) pendingUpdate(ctx context.Context, rt store.Routine) *galleryUpdate {
	origin := a.galleryOrigin(ctx, rt)
	if origin == "" {
		return nil
	}
	ix, err := a.galleryIndex(ctx, false)
	if err != nil {
		return nil
	}
	e, ok := ix.Find(origin)
	if !ok || e.Hash == gallery.Hash(rt.Body) {
		return nil
	}
	had := map[string]bool{}
	for _, p := range rt.Body.Manifest.Params {
		had[p.Name] = true
	}
	u := &galleryUpdate{Name: e.Routine.Name, Description: e.Routine.Description, Settings: []string{}}
	for _, p := range e.Routine.Manifest.Params {
		if !had[p.Name] {
			u.Settings = append(u.Settings, p.Label)
		}
	}
	return u
}

// updateFromGallery saves the gallery's current version as a new version
// of the same routine, keeping the owner's schedule and the settings the
// new version still has.
func (a *App) updateFromGallery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rt, err := a.myRoutine(ctx, r.PathValue("id"))
	if err != nil {
		server.WriteError(w, notFound(err))
		return
	}
	origin := a.galleryOrigin(ctx, rt)
	if origin == "" {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "this routine did not come from the gallery"})
		return
	}
	ix, err := a.galleryIndex(ctx, true)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 502, Msg: "could not load the gallery: " + err.Error()})
		return
	}
	e, ok := ix.Find(origin)
	if !ok {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "the routine is no longer in the gallery"})
		return
	}
	rep := ix.Verify(ctx, e)
	if !rep.Verified {
		server.WriteError(w, server.StatusError{Status: 422, Msg: "not updated: " + strings.Join(rep.Problems, "; ")})
		return
	}
	if err := a.galleryUpdateAllowed(ctx, ix, rt, e); err != nil {
		server.WriteError(w, server.StatusError{Status: 422, Msg: "not updated: " + err.Error()})
		return
	}
	if sends, _ := gallery.Sends(rt.Body); rep.Sends && !sends {
		server.WriteError(w, server.StatusError{Status: 422, Msg: "not updated: the new version can send your data out of Pimpo; install it again from the gallery to review it"})
		return
	}
	if _, err := a.Store.SaveRoutine(ctx, rt.ID, e.Routine, "updated from the gallery: "+e.Author+" "+e.Hash[:12], "human:owner"); err != nil {
		server.WriteError(w, err)
		return
	}
	kept := map[string]any{}
	for _, p := range e.Routine.Manifest.Params {
		if v, ok := rt.Settings.Params[p.Name]; ok {
			kept[p.Name] = v
		}
	}
	if _, err := e.Routine.Manifest.ResolveParams(kept); err != nil {
		kept = map[string]any{}
	}
	a.Store.SetRoutineSettings(ctx, rt.ID, store.Settings{Schedule: rt.Settings.Schedule, Params: kept})
	a.Events.Put(ctx, "gallery.origin."+rt.ID, e.ID)
	a.rememberGalleryVersion(ctx, ix, rt.ID, e)
	a.Events.Append(ctx, "gallery.updated", "human:owner", map[string]string{"routine": rt.ID, "hash": e.Hash})
	a.Scheduler.Changed(ctx, rt.ID)
	rt, _ = a.Store.Routine(ctx, rt.ID)
	server.WriteJSON(w, 200, a.summary(ctx, rt))
}

// Trust on first use: the first key seen for an author is the one their
// later entries must be signed with, and a routine only updates to a
// newer entry by the author it was installed from. The index brings both
// the entries and the keys, so without this whoever controls the index
// could swap in their own key.
const galleryAuthorsKey = "gallery.authors"

func (a *App) galleryAuthors(ctx context.Context) map[string]string {
	raw, _ := a.Events.Get(ctx, galleryAuthorsKey)
	m := map[string]string{}
	json.Unmarshal([]byte(raw), &m)
	return m
}

// trustAuthor refuses an entry signed with a key other than the one first
// seen for its author.
func (a *App) trustAuthor(ctx context.Context, ix gallery.Index, e gallery.Entry) error {
	key := ix.Authors[e.Author].Key
	if known, ok := a.galleryAuthors(ctx)[e.Author]; ok && known != key {
		return errors.New("the author " + e.Author + " now signs with a different key than when you first installed from them; if they really changed it, remove their routines and install again")
	}
	return nil
}

type galleryVersion struct {
	Author    string    `json:"author"`
	Key       string    `json:"key"`
	Published time.Time `json:"published"`
	// Hashes are every version installed, so an older one is never taken
	// back.
	Hashes []string `json:"hashes"`
}

func (a *App) galleryVersionOf(ctx context.Context, id string) (galleryVersion, bool) {
	raw, _ := a.Events.Get(ctx, "gallery.version."+id)
	var v galleryVersion
	return v, raw != "" && json.Unmarshal([]byte(raw), &v) == nil
}

// rememberGalleryVersion records who signed the version now installed.
func (a *App) rememberGalleryVersion(ctx context.Context, ix gallery.Index, id string, e gallery.Entry) {
	key := ix.Authors[e.Author].Key
	authors := a.galleryAuthors(ctx)
	if _, ok := authors[e.Author]; !ok && key != "" {
		authors[e.Author] = key
		b, _ := json.Marshal(authors)
		a.Events.Put(ctx, galleryAuthorsKey, string(b))
	}
	v, _ := a.galleryVersionOf(ctx, id)
	v.Author, v.Key, v.Published = e.Author, key, e.Published
	if !slices.Contains(v.Hashes, e.Hash) {
		v.Hashes = append(v.Hashes, e.Hash)
	}
	b, _ := json.Marshal(v)
	a.Events.Put(ctx, "gallery.version."+id, string(b))
}

// galleryUpdateAllowed checks an update is by the same author, with the
// same key, and not an older version than the one installed.
func (a *App) galleryUpdateAllowed(ctx context.Context, ix gallery.Index, rt store.Routine, e gallery.Entry) error {
	if err := a.trustAuthor(ctx, ix, e); err != nil {
		return err
	}
	v, ok := a.galleryVersionOf(ctx, rt.ID)
	if !ok {
		// Installed before versions were recorded: this update is the
		// first use.
		return nil
	}
	switch {
	case e.Author != v.Author:
		return errors.New("the gallery now lists this routine under another author (" + e.Author + ", not " + v.Author + ")")
	case ix.Authors[e.Author].Key != v.Key:
		return errors.New("the author signs with a different key than the version you installed")
	case slices.Contains(v.Hashes, e.Hash) || e.Published.Before(v.Published):
		return errors.New("the gallery offers an older version than the one you have")
	}
	return nil
}
