package app

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/models"
)

// The live model catalog: each provider's list comes from the provider
// itself, kept a day, and is joined with the owner's own models. Models the
// provider started offering are marked new; the owner's models it stopped
// offering are marked retired, and whoever uses one is told to switch. A
// model found without a price still needs one from the owner before it runs.

const (
	retiredKey     = "models.retired"
	catalogDoneKey = "models.checked"
	catalogEvery   = 24 * time.Hour
)

func seenKey(provider string) string { return "models.seen." + provider }

// catalog is a provider's merged list; fresh skips the day's cache.
func (a *App) catalog(ctx context.Context, provider string, fresh bool) ([]models.Model, error) {
	e, err := a.modelEndpoint(ctx, provider)
	if err != nil {
		return nil, err
	}
	live, err := a.modelClient().Discover(ctx, e, fresh)
	if err != nil {
		return nil, err
	}
	var mine []models.Owned
	for _, m := range a.Settings(ctx).Models {
		if p, name, _ := strings.Cut(m.ID, ":"); p == provider {
			mine = append(mine, models.Owned{ID: name, PriceIn: m.PriceIn, PriceOut: m.PriceOut})
		}
	}
	var seen map[string]time.Time
	if raw, _ := a.Events.Get(ctx, seenKey(provider)); raw != "" {
		json.Unmarshal([]byte(raw), &seen)
	}
	out, retired, seen := models.Merge(live, mine, seen, time.Now())
	if b, err := json.Marshal(seen); err == nil {
		a.Events.Put(ctx, seenKey(provider), string(b))
	}
	full := make([]string, len(retired))
	for i, id := range retired {
		full[i] = provider + ":" + id
	}
	a.noteRetired(ctx, provider, full)
	if out == nil {
		out = []models.Model{}
	}
	return out, nil
}

// retired is every model the owner added that its provider stopped
// offering, by provider.
func (a *App) retired(ctx context.Context) map[string][]string {
	all := map[string][]string{}
	if raw, _ := a.Events.Get(ctx, retiredKey); raw != "" {
		json.Unmarshal([]byte(raw), &all)
	}
	return all
}

// retiredModels lists them all, as provider:model ids still in the
// owner's list.
func (a *App) retiredModels(ctx context.Context) []string {
	mine := a.Settings(ctx).Models
	out := []string{}
	for _, ids := range a.retired(ctx) {
		for _, id := range ids {
			if slices.ContainsFunc(mine, func(m ModelOption) bool { return m.ID == id }) {
				out = append(out, id)
			}
		}
	}
	slices.Sort(out)
	return out
}

// noteRetired records a provider's retired models and tells the owner
// once about each one a job still uses.
func (a *App) noteRetired(ctx context.Context, provider string, ids []string) {
	all := a.retired(ctx)
	was := all[provider]
	if slices.Equal(was, ids) {
		return
	}
	if len(ids) == 0 {
		delete(all, provider)
	} else {
		all[provider] = ids
	}
	if b, err := json.Marshal(all); err == nil {
		a.Events.Put(ctx, retiredKey, string(b))
	}
	s := a.Settings(ctx)
	for _, id := range ids {
		if slices.Contains(was, id) {
			continue
		}
		a.Events.Append(ctx, "model.retired", "system", map[string]string{"model": id})
		if s.usesModel(id) {
			a.Channel.Notify(ctx, explore.Notice{Kind: "failure", Text: i18n.T(ctx, "msg.model.retired", "model", id)})
		}
	}
}

// usesModel says whether a job, fallback or automatic choice uses id.
func (s Settings) usesModel(id string) bool {
	if slices.Contains([]string{s.ExploreModel, s.CompileModel, s.JudgeModel, s.AutoLight, s.AutoStrong}, id) {
		return true
	}
	for _, f := range s.Fallbacks {
		if slices.Contains(f, id) {
			return true
		}
	}
	return false
}

// catalogLoop looks at the lists of the providers the owner uses once a
// day, so a retired model is noticed without opening Settings.
func (a *App) catalogLoop(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if a.dueForCatalog(ctx, time.Now()) {
				a.checkCatalogs(ctx)
			}
		}
	}
}

func (a *App) dueForCatalog(ctx context.Context, now time.Time) bool {
	last, _ := a.Events.Get(ctx, catalogDoneKey)
	at, err := time.Parse(time.RFC3339, last)
	return err != nil || now.Sub(at) >= catalogEvery
}

// checkCatalogs refreshes each provider the owner has models from. A
// provider that cannot be reached is skipped: only a list it gave can
// retire a model.
func (a *App) checkCatalogs(ctx context.Context) {
	a.Events.Put(ctx, catalogDoneKey, time.Now().UTC().Format(time.RFC3339))
	done := map[string]bool{}
	for _, m := range a.Settings(ctx).Models {
		p, _, _ := strings.Cut(m.ID, ":")
		if done[p] || !slices.Contains(llm.Providers, p) {
			continue
		}
		done[p] = true
		cctx, cancel := context.WithTimeout(ctx, time.Minute)
		a.catalog(cctx, p, true)
		cancel()
	}
}
