package app

import (
	"context"
	"strings"
	"testing"

	"github.com/denerFernandes/zodim/internal/llm"
)

func TestDemoRunsTheWholeStoryOffline(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	ta.EnableDemo(ctx, 0)
	for _, req := range []string{"Todo dia às 7h me manda a agenda e os e-mails importantes", "Arquiva as newsletters às 18h", "Me avisa das contas que vencem"} {
		_, out := ta.do(t, "POST", "/api/explorations", map[string]string{"request": req})
		id := out["id"].(string)
		ta.Explore.Wait()
		code, r := ta.do(t, "POST", "/api/explorations/"+id+"/compile", nil)
		if code != 200 {
			t.Fatalf("%s: compile %v", req, r)
		}
		_, run := ta.do(t, "POST", "/api/routines/"+r["id"].(string)+"/run", nil)
		if run["error"] != "" {
			t.Fatalf("%s: run %v", req, run["error"])
		}
	}
	_, out := ta.do(t, "GET", "/api/routines", nil)
	if n := len(out["list"].([]any)); n != 3 {
		t.Fatalf("routines %d", n)
	}
	code, rule := ta.do(t, "POST", "/api/rules/compile", map[string]string{"text": "Nunca apague e-mail sem me perguntar"})
	if code != 200 || !strings.Contains(strings.Join([]string{rule["rule"].(map[string]any)["then"].(string)}, ""), "ask") {
		t.Fatalf("demo rule %v", rule)
	}
}
