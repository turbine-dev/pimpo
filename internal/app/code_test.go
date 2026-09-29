package app

import (
	"context"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/sandbox"
)

func TestCodeRunsOnlyWhenTurnedOn(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	var got sandbox.Job
	old := runInSandbox
	runInSandbox = func(_ context.Context, j sandbox.Job) (sandbox.Result, error) {
		got = j
		return sandbox.Result{Stdout: "42\n"}, nil
	}
	defer func() { runInSandbox = old }()
	args := map[string]any{"language": "python", "code": "print(42)"}
	if _, err := ta.Router.Call(ctx, "code.run", "", args); err == nil || !strings.Contains(err.Error(), "Laboratório") {
		t.Fatalf("ran while off: %v", err)
	}
	s := ta.Settings(ctx)
	s.LabsOn = []string{"code_sandbox"}
	if err := ta.SaveSettings(ctx, s, "test"); err != nil {
		t.Fatal(err)
	}
	out, err := ta.Router.Call(ctx, "code.run", "", args)
	if err != nil || out.(sandbox.Result).Stdout != "42\n" || got.Code != "print(42)" {
		t.Fatalf("%v %v %+v", out, err, got)
	}
	s.LabsOn = []string{"rm -rf"}
	if err := ta.SaveSettings(ctx, s, "test"); err == nil {
		t.Fatal("an unknown opt-in feature was accepted")
	}
}
