package app

import (
	"context"
	"errors"

	"github.com/turbine-dev/pimpo/internal/connector"
	"github.com/turbine-dev/pimpo/internal/sandbox"
)

// codeCap runs programs in the sandbox (docs/rfcs/0001-code-sandbox.md),
// only after the owner turned it on in Laboratório.
type codeCap struct{ a *App }

func (codeCap) Capabilities() []string { return []string{"code.run"} }

// Sandbox runs the jobs; tests replace it.
var runInSandbox = func(ctx context.Context, j sandbox.Job) (sandbox.Result, error) { return sandbox.Docker{}.Run(ctx, j) }

func (c codeCap) Call(ctx context.Context, _, _ string, args any) (any, error) {
	if !c.a.chose(ctx, "code_sandbox") {
		return nil, errors.New("the code sandbox is off: the owner can turn it on in Ajustes › Laboratório (it needs Docker)")
	}
	var j sandbox.Job
	if err := connector.Args(args, &j); err != nil {
		return nil, err
	}
	return runInSandbox(ctx, j)
}
