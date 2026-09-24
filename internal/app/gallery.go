package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	starter "github.com/denerFernandes/vigia/gallery"
	"github.com/denerFernandes/vigia/internal/gallery"
	"github.com/denerFernandes/vigia/internal/server"
	"github.com/denerFernandes/vigia/internal/vault"
)

func (a *App) galleryRoutes() {
	a.Server.Handle("GET /api/gallery", a.listGallery)
	a.Server.Handle("POST /api/gallery/{id}/install", a.installFromGallery)
	a.Server.Handle("POST /api/routines/{id}/publish", a.publishRoutine)
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
		err = json.Unmarshal(starter.Index, &ix)
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
	if list, err := a.Store.Routines(ctx); err == nil {
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
	if rep := ix.Verify(ctx, e); !rep.Verified {
		server.WriteError(w, server.StatusError{Status: 422, Msg: "not installed: " + strings.Join(rep.Problems, "; ")})
		return
	}
	id := e.ID
	if existing, err := a.Store.Routine(ctx, id); err == nil && gallery.Hash(existing.Body) != e.Hash {
		id += "-" + e.Hash[:6]
	}
	rt, err := a.Store.SaveRoutine(ctx, id, e.Routine, "installed from the gallery: "+e.Author+" "+e.Hash[:12], "human:owner")
	if err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(ctx, "gallery.installed", "human:owner", map[string]string{"routine": rt.ID, "author": e.Author, "hash": e.Hash})
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
	rt, err := a.Store.Routine(ctx, r.PathValue("id"))
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
