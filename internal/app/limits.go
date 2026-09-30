package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
)

// Models and spending per person: the owner chooses which of the house's
// models each person and each assistant may use, and a daily limit per
// person inside the house's. A call may use only what the house, the
// person it is for and the assistant answering all allow; the automatic
// choice stays inside that, an explicit choice outside it is refused, and
// every cost counts against the person it was for as well as the house.

func (a *App) limitRoutes() {
	a.Server.Handle("PUT /api/people/{id}/limits", a.putLimits)
	a.Server.Handle("GET /api/me/limits", a.myLimits)
}

// personLimit is a person's own daily limit: what the owner set, for a
// guest a small one by default, and for someone unknown the guest's.
func (a *App) personLimit(ctx context.Context, person string) float64 {
	if people.Norm(person) == people.OwnerID {
		return 0
	}
	p, err := a.People.Get(ctx, person)
	if err != nil {
		return people.GuestDailyUSD
	}
	return p.DailyLimit()
}

// personModels are the models a person may use, nil for all of the
// house's. The owner uses all of them.
func (a *App) personModels(ctx context.Context, person string) []string {
	if people.Norm(person) == people.OwnerID {
		return nil
	}
	p, err := a.People.Get(ctx, person)
	if err != nil {
		return []string{}
	}
	return p.Models
}

type assistantModelsKey struct{}

// withAssistant marks ctx as answered by an assistant limited to some
// models (nil for all of them), so every call made for it stays inside.
func withAssistant(ctx context.Context, models []string) context.Context {
	if len(models) == 0 {
		return ctx
	}
	return context.WithValue(ctx, assistantModelsKey{}, models)
}

func assistantModelsOf(ctx context.Context) []string {
	m, _ := ctx.Value(assistantModelsKey{}).([]string)
	return m
}

// houseModels are the models the house has: Claude Code's, Codex, and the
// owner's priced models.
func (a *App) houseModels(ctx context.Context) []string {
	var out []string
	if claudeInstalled() {
		out = append(out, "sonnet", "opus", "haiku")
	}
	if llm.CodexBinary() != "" {
		out = append(out, "codex")
	}
	for _, m := range a.Settings(ctx).Models {
		out = append(out, m.ID)
	}
	return out
}

// allowedModels are the models a call for ctx may use, when anything
// narrows the house's: the person's list and the assistant's, both kept.
// restricted is false when every house model may be used.
func (a *App) allowedModels(ctx context.Context) (list []string, restricted bool) {
	mine := a.personModels(ctx, people.From(ctx))
	role := assistantModelsOf(ctx)
	if mine == nil && role == nil {
		return nil, false
	}
	from := mine
	if from == nil {
		from = role
	}
	for _, m := range from {
		if a.usableModel(ctx, m) && m != "" && m != Auto && (mine == nil || slices.Contains(mine, m)) && (role == nil || slices.Contains(role, m)) {
			list = append(list, m)
		}
	}
	return list, true
}

// modelAllowed says whether a call for ctx may use a model; "" and auto
// always may, as the choice is then made inside what is allowed.
func (a *App) modelAllowed(ctx context.Context, model string) bool {
	if model == "" || model == Auto {
		return true
	}
	list, restricted := a.allowedModels(ctx)
	return !restricted || slices.Contains(list, model)
}

// modelRefusal explains why a model chosen by hand cannot be used, or is
// nil when it can.
func (a *App) modelRefusal(ctx context.Context, model string) error {
	if !a.usableModel(ctx, model) {
		return server.StatusError{Status: 400, Msg: model + " is not among your models"}
	}
	if a.modelAllowed(ctx, model) {
		return nil
	}
	if role := assistantModelsOf(ctx); role != nil && !slices.Contains(role, model) {
		return server.StatusError{Status: 400, Msg: i18n.T(ctx, "limits.assistant_model", "model", model)}
	}
	return server.StatusError{Status: 403, Msg: i18n.T(ctx, "limits.person_model", "model", model)}
}

// errNoModel is a call with nothing it may use.
func (a *App) errNoModel(ctx context.Context) error {
	return errors.New(i18n.T(ctx, "limits.no_model"))
}

// fitModel keeps a model a call may use, and otherwise gives the cheapest
// one it may; ok is false when it may use none.
func (a *App) fitModel(ctx context.Context, model string) (string, bool) {
	list, restricted := a.allowedModels(ctx)
	if !restricted || (model != "" && slices.Contains(list, model)) {
		return model, true
	}
	m := a.cheapest(ctx, list)
	return m, m != ""
}

// cheapest is the model of a list with the lowest price: local models
// first, then the owner's priced models, then Claude Code's by size, and
// last those whose price Pimpo does not know.
func (a *App) cheapest(ctx context.Context, list []string) string {
	prices := map[string]float64{"haiku": 6, "sonnet": 18, "opus": 30}
	for _, m := range a.Settings(ctx).Models {
		if !llm.IsOpencode(m.ID) {
			prices[m.ID] = m.PriceIn + m.PriceOut
		}
	}
	best, bestPrice := "", 0.0
	for _, m := range list {
		p, known := prices[m]
		if !known {
			p = 1e9
		}
		if best == "" || p < bestPrice {
			best, bestPrice = m, p
		}
	}
	return best
}

// allowedChain keeps the models of a job's chain a call may use, or the
// cheapest one it may when none of them is.
func (a *App) allowedChain(ctx context.Context, chain []string) []string {
	if _, restricted := a.allowedModels(ctx); !restricted {
		return chain
	}
	var out []string
	for _, m := range chain {
		if m != "" && a.modelAllowed(ctx, m) {
			out = append(out, m)
		}
	}
	if len(out) == 0 && len(chain) > 0 {
		if m, ok := a.fitModel(ctx, chain[0]); ok {
			out = []string{m}
		}
	}
	return out
}

// putLimits is the owner setting a person's models and daily limit. It is
// the house's administration, not the person's things: the person sees
// their limits in their account and learns in their activity that they
// changed.
func (a *App) putLimits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ownerOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	if id == people.OwnerID {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "the administrator's limits are the house's, in Settings"})
		return
	}
	var req struct {
		Models   []string `json:"models"`
		DailyUSD float64  `json:"daily_usd"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	models, err := a.houseChoice(ctx, req.Models)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	if house := a.Budget.Limit(ctx); house > 0 && req.DailyUSD > house+1e-9 {
		server.WriteError(w, server.StatusError{Status: 400, Msg: i18n.T(ctx, "limits.over_house", "limit", fmt.Sprintf("$%.2f", house))})
		return
	}
	p, err := a.People.SetLimits(ctx, id, models, req.DailyUSD)
	if err != nil {
		server.WriteError(w, peopleError(err))
		return
	}
	a.Events.Append(ctx, "person.limits_changed", "system", map[string]any{"to": p.ID, "models": p.Models, "daily_usd": p.DailyUSD})
	server.WriteJSON(w, 200, a.viewPerson(ctx, p))
}

// houseChoice checks a list of models against the house's, without
// repeats; empty is all of them.
func (a *App) houseChoice(ctx context.Context, list []string) ([]string, error) {
	out := []string{}
	for _, m := range list {
		m = strings.TrimSpace(m)
		if m == "" || m == Auto || !a.usableModel(ctx, m) {
			return nil, server.StatusError{Status: 400, Msg: m + " is not among the house's models"}
		}
		if !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// myLimits is what the person asking may use and has spent today; their
// own, never anyone else's.
func (a *App) myLimits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := people.From(ctx)
	list, restricted := a.allowedModels(ctx)
	if !restricted {
		list = a.houseModels(ctx)
	}
	if list == nil {
		list = []string{}
	}
	limit := a.Budget.LimitFor(ctx, me)
	spent, _ := a.Budget.TodayFor(ctx, me)
	server.WriteJSON(w, 200, map[string]any{
		"models":      list,
		"all_models":  !restricted,
		"daily_usd":   limit,
		"house_usd":   a.Budget.Limit(ctx),
		"spent_today": spent,
		"reached":     limit > 0 && spent >= limit,
	})
}

// roomFor keeps the strong model for days the person asking has budget
// left of their own.
func (a *App) roomFor(ctx context.Context) bool {
	person := people.From(ctx)
	limit := a.Budget.LimitFor(ctx, person)
	if limit <= 0 {
		return true
	}
	spent, _ := a.Budget.TodayFor(ctx, person)
	return spent < limit*0.8
}
