package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/turbine-dev/pimpo/internal/approval"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/policy"
	"github.com/turbine-dev/pimpo/internal/server"
)

var errApprovalGone = errors.New("this request is no longer waiting")

// runVersion is the code version a routine run started with, or 0 when
// the source is not a recorded run.
func (a *App) runVersion(ctx context.Context, source string) int {
	id := approval.RoutineOf(source)
	_, run, _ := strings.Cut(source, "#")
	n, err := strconv.ParseInt(run, 10, 64)
	if id == "" || err != nil {
		return 0
	}
	routine, version, err := a.Store.RunVersion(ctx, n)
	if err != nil || routine != id {
		return 0
	}
	return version
}

// granted lets through an action that would ask when the person approved
// this exact operation for this routine. Rules that block still block,
// and whatsapp.send_to and ha.critical always ask.
func (a *App) granted(ctx context.Context, act policy.Action, d policy.Decision) policy.Decision {
	if d.Verdict != policy.Ask || a.Grants == nil || approval.RoutineOf(act.Source) == "" {
		return d
	}
	version := a.runVersion(ctx, act.Source)
	if version == 0 {
		return d
	}
	if g, ok := a.Grants.Find(ctx, act, version); ok {
		return policy.Decision{Verdict: policy.Allow, Reason: i18n.T(ctx, "policy.granted"), Rule: "grant:" + g.ID}
	}
	return d
}

// resolveApproval answers a waiting request as the person in ctx. "For
// this routine" saves the grant first, so the run's next identical call
// already finds it.
func (a *App) resolveApproval(ctx context.Context, id string, ans approval.Answer, limit *float64) error {
	var grant approval.Grant
	if ans == approval.Routine {
		req, ok := a.Approvals.Get(id)
		if !ok {
			return errApprovalGone
		}
		g, err := approval.NewGrant(req, people.From(ctx), a.runVersion(ctx, req.Action.Source), limit)
		if err == nil && g.Version == 0 {
			err = errors.New("this action is not from a routine run")
		}
		if err != nil {
			return server.StatusError{Status: 403, Msg: err.Error()}
		}
		if err := a.Grants.Add(ctx, g, actor(ctx)); err != nil {
			return err
		}
		grant = g
	}
	if !a.Approvals.Resolve(ctx, id, ans, actor(ctx)) {
		if grant.ID != "" {
			a.Grants.Revoke(ctx, grant.ID, grant.Person, actor(ctx))
		}
		return errApprovalGone
	}
	return nil
}

// grantView is a grant with its routine's name, for the list.
type grantView struct {
	approval.Grant
	RoutineName string `json:"routine_name"`
}

// myGrants lists the person's grants still in force. Those of a routine
// that changed, went away or now works for someone else never apply
// again, so they are dropped.
func (a *App) myGrants(ctx context.Context) []grantView {
	person := people.From(ctx)
	names := map[string]string{}
	a.Grants.Prune(ctx, func(g approval.Grant) bool {
		rt, err := a.Store.Routine(ctx, g.Routine)
		if err != nil || rt.Version != g.Version || people.Norm(rt.Person) != g.Person {
			return false
		}
		names[g.Routine] = rt.Body.Name
		return true
	})
	out := []grantView{}
	for _, g := range a.Grants.Mine(ctx, person) {
		out = append(out, grantView{g, names[g.Routine]})
	}
	return out
}

func (a *App) grantRoutes() {
	a.Server.Handle("GET /api/grants", func(w http.ResponseWriter, r *http.Request) { server.WriteJSON(w, 200, a.myGrants(r.Context())) })
	a.Server.Handle("DELETE /api/grants/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if err := a.Grants.Revoke(ctx, r.PathValue("id"), people.From(ctx), actor(ctx)); err != nil {
			if errors.Is(err, approval.ErrNoGrant) {
				err = server.StatusError{Status: 404, Msg: err.Error()}
			}
			server.WriteError(w, err)
			return
		}
		server.WriteJSON(w, 200, map[string]string{"state": "revoked"})
	})
}
