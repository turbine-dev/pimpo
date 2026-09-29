package explore

import (
	"testing"

	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/runtime"
	"github.com/turbine-dev/pimpo/internal/trace"
)

// A repair that drops a setting keeps the old tests that did not depend on
// it and drops the ones about it.
func TestCarryTests(t *testing.T) {
	old := routine.Routine{Manifest: runtime.Manifest{Params: []runtime.Param{{Name: "dias", Type: "number", Default: 7}, {Name: "destinos", Type: "destinations"}}},
		Tests: []routine.Test{
			{Name: "várias notícias", Scenario: trace.Scenario{Params: map[string]any{"dias": 7, "destinos": []any{"telegram"}}}},
			{Name: "janela reduzida", Scenario: trace.Scenario{Params: map[string]any{"dias": 3}}},
			{Name: "sem ajustes", Scenario: trace.Scenario{}},
		}}
	now := runtime.Manifest{Params: []runtime.Param{{Name: "destinos", Type: "destinations"}}}
	kept, dropped := carryTests(old, now)
	if len(kept) != 2 || len(dropped) != 1 || dropped[0] != "janela reduzida" {
		t.Fatalf("kept %v dropped %v", kept, dropped)
	}
	if _, ok := kept[0].Scenario.Params["dias"]; ok || kept[0].Scenario.Params["destinos"] == nil {
		t.Fatalf("params %v", kept[0].Scenario.Params)
	}
	if old.Tests[0].Scenario.Params["dias"] != 7 {
		t.Fatal("changed the old version's tests")
	}
}
