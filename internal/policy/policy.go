// Package policy decides, before every capability call, whether it may
// happen: allowed, turned into a reversible version, held for the owner's
// approval, or blocked.
package policy

import (
	"context"

	"github.com/denerFernandes/pimpo/internal/capability"
)

type Verdict string

const (
	Allow      Verdict = "allow"
	Reversible Verdict = "reversible"
	Ask        Verdict = "ask"
	Block      Verdict = "block"
)

// Action is one capability call as the policy sees it.
type Action struct {
	Capability string          `json:"capability"`
	Scope      string          `json:"scope,omitempty"`
	Args       any             `json:"args"`
	Risk       capability.Risk `json:"risk"`
	// Source is "routine:<id>" or "exploration:<id>".
	Source string `json:"source"`
	// Person is who the run acts for, and Role their role in the house.
	Person string `json:"person,omitempty"`
	Role   string `json:"role,omitempty"`
}

type Decision struct {
	Verdict Verdict `json:"verdict"`
	Reason  string  `json:"reason,omitempty"`
	// Rule is the id of the user rule that decided, if any.
	Rule string `json:"rule,omitempty"`
}

type Policy interface {
	Decide(ctx context.Context, a Action) Decision
}

// Open allows everything the manifest allows. It is the F1 policy, before
// user rules exist; the runtime already confines calls to the manifest.
type Open struct{}

func (Open) Decide(context.Context, Action) Decision { return Decision{Verdict: Allow} }
