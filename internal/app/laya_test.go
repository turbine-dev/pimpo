package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/policy"
)

// layaServer stands in for laya-serve, answering every yes/no with p.
func layaServer(t *testing.T, p float64) (*httptest.Server, *int) {
	t.Helper()
	var mu sync.Mutex
	asked := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.Write([]byte(`{"status":"ok"}`))
		case "/v1/systemone":
			var body struct{ Model string }
			json.NewDecoder(r.Body).Decode(&body)
			if body.Model != "laya" {
				w.WriteHeader(400)
				return
			}
			mu.Lock()
			asked++
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]any{"answers": map[string]any{"q": map[string]any{"type": "noul", "noul": p}}})
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &asked
}

func (ta *testApp) useJudge(t *testing.T, backend, laya string) {
	t.Helper()
	_, s := ta.do(t, "GET", "/api/settings", nil)
	s["judge_backend"], s["laya_url"] = backend, laya
	if code, out := ta.do(t, "PUT", "/api/settings", s); code != 200 {
		t.Fatalf("settings: %d %v", code, out)
	}
}

func TestLayaJudgesOnThisComputer(t *testing.T) {
	srv, asked := layaServer(t, 0.91)
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	ta.useJudge(t, "laya", srv.URL)
	if a, err := ta.judge(ctx, "Is this urgent?", "the server is down"); err != nil || a.P != 0.91 || a.Backend != "laya" {
		t.Fatalf("laya: %+v %v", a, err)
	}
	// As the local judge, Laya answers first.
	ta.useJudge(t, "local", srv.URL)
	before := *asked
	if a, err := ta.judge(ctx, "Is this urgent?", "the server is down"); err != nil || a.Backend != "laya" || *asked != before+1 {
		t.Fatalf("local with Laya: %+v %v", a, err)
	}
	_, out := ta.do(t, "POST", "/api/judge/laya/test", nil)
	if out["ok"] != true || out["p"] != 0.91 {
		t.Fatalf("test: %v", out)
	}
	// The test reaches the saved address only, whatever the request says.
	ta.useJudge(t, "local", "http://127.0.0.1:1")
	if _, out := ta.do(t, "POST", "/api/judge/laya/test", map[string]string{"url": srv.URL}); out["ok"] != false || !strings.Contains(out["error"].(string), "unreachable") {
		t.Fatalf("a server that is not there: %v", out)
	}
	_, s := ta.do(t, "GET", "/api/settings", nil)
	s["laya_url"] = "ftp://laya"
	if code, _ := ta.do(t, "PUT", "/api/settings", s); code != 400 {
		t.Fatal("a Laya address that is not http")
	}
}

func TestLayaDecidesForACompanyMember(t *testing.T) {
	for _, c := range []struct {
		p    float64
		want policy.Verdict
	}{{0.95, policy.Allow}, {0.03, policy.Block}, {0.6, policy.Ask}} {
		srv, _ := layaServer(t, c.p)
		ta, co := clerk(t)
		ta.useJudge(t, "local", srv.URL)
		ta.autonomy(t, co, map[string]any{"kind": "laya", "threshold": 0.9})
		if d := ta.decide(context.Background(), sendAs(co)); d.Verdict != c.want {
			t.Errorf("p=%.2f: %+v", c.p, d)
		}
	}
	ta, co := clerk(t)
	if code, _ := ta.do(t, "PUT", "/api/companies/"+co+"/members/clara", map[string]any{"name": "Clara", "role": "clerk", "reports_to": "bia",
		"autonomy": []map[string]any{{"min_risk": "irreversible", "decider": map[string]any{"kind": "laya", "threshold": 0.3}}}}); code != 400 {
		t.Fatal("a Laya threshold under 0.5")
	}
}
