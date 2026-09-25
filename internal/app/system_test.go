package app

import (
	"testing"

	"github.com/denerFernandes/zodim/internal/llm"
	"github.com/denerFernandes/zodim/internal/routine"
	"github.com/denerFernandes/zodim/internal/store"
)

func TestSystemStatus(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := t.Context()
	ta.Home = t.TempDir()
	ta.Store.SaveRoutine(ctx, "brief", routine.Routine{Name: "Resumo", Code: "x"}, "", "owner")
	id, _ := ta.Store.StartRun(ctx, "brief", 1)
	ta.Store.FinishRun(ctx, id, store.RunFailed, "boom", 0, 0)
	ta.Vault.Set(ctx, "typesafe.key", "k")

	code, out := ta.do(t, "GET", "/api/system", nil)
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	host := out["host"].(map[string]any)
	if host["cpus"].(float64) < 1 || host["disk_size"].(float64) == 0 {
		t.Fatalf("host %v", host)
	}
	if out["activity"].(map[string]any)["runs_failed_today"] != float64(1) {
		t.Fatalf("activity %v", out["activity"])
	}
	states := map[string]string{}
	for _, c := range out["components"].([]any) {
		m := c.(map[string]any)
		states[m["id"].(string)] = m["state"].(string)
	}
	if states["jev"] != "ok" || states["telegram"] != "off" || states["mail"] != "off" {
		t.Fatalf("%v", states)
	}
}
