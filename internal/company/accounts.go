package company

import (
	"fmt"
	"slices"
)

// Grants on a company's shared account: reading only, or acting too.
const (
	GrantRead = "read"
	GrantAct  = "act"
)

// A SharedAccount is one of the company's own accounts (the official
// channel, the support inbox) and which members may use it, and how.
type SharedAccount struct {
	Kind   string            `json:"kind" yaml:"kind"`
	Label  string            `json:"label,omitempty" yaml:"label,omitempty"`
	Grants map[string]string `json:"grants,omitempty" yaml:"grants,omitempty"`
}

// AccountGrant is what a member may do with the company's shared account
// of a kind, or "".
func (o Org) AccountGrant(kind, member string) string {
	for _, a := range o.Accounts {
		if a.Kind == kind {
			return a.Grants[member]
		}
	}
	return ""
}

func (o Org) checkAccounts() error {
	seen := map[string]bool{}
	for _, a := range o.Accounts {
		if a.Kind == "" || seen[a.Kind] {
			return fmt.Errorf("one shared account per kind")
		}
		seen[a.Kind] = true
		for m, g := range a.Grants {
			if err := o.CheckWork(m); err != nil {
				return fmt.Errorf("the %s account is shared with %q, who is not an agent of the company", a.Kind, m)
			}
			if !slices.Contains([]string{GrantRead, GrantAct}, g) {
				return fmt.Errorf("a shared account is granted to read or to act")
			}
		}
	}
	return nil
}
