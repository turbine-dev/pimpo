package routine

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/denerFernandes/pimpo/internal/runtime"
	"github.com/denerFernandes/pimpo/internal/trace"
)

// Every recorded search came from the real service with its recorded
// arguments, so filtering the recording by those arguments must keep all
// of it; a dropped item means the replay reads a query unlike the service.
func TestReplayKeepsRecordedResults(t *testing.T) {
	paths, _ := filepath.Glob("../../testdata/*/*.json")
	if len(paths) == 0 {
		t.Fatal("no traces found")
	}
	for _, p := range paths {
		tr, err := trace.Load(p)
		if err != nil || tr.ID == "" {
			continue
		}
		now, err := time.Parse(time.RFC3339, tr.Now)
		if err != nil {
			continue
		}
		for _, c := range tr.Calls {
			if !searchable[c.Capability] {
				continue
			}
			var args, result any
			json.Unmarshal(c.Args, &args)
			json.Unmarshal(c.Result, &result)
			list, _ := result.([]any)
			got, _ := filterResponse(c.Capability, args, result, now, true).([]any)
			if len(got) != len(list) {
				t.Errorf("%s: %s(%s) keeps %d of %d recorded items", tr.ID, c.Capability, c.Args, len(got), len(list))
			}
		}
	}
}

func TestMailQueries(t *testing.T) {
	now := time.Date(2026, 10, 8, 9, 0, 0, 0, time.FixedZone("BRT", -3*3600))
	m := func(from, date string, labels ...string) map[string]any {
		ls := []any{}
		for _, l := range labels {
			ls = append(ls, l)
		}
		return map[string]any{"from": from, "date": date, "labels": ls, "subject": "x"}
	}
	yesterday := m("a@x.com", "2026-10-07T10:00:00-03:00", "INBOX")
	before := m("b@quintoandar.com.br", "2026-10-05T10:00:00-03:00", "INBOX", "CATEGORY_PROMOTIONS")
	for q, want := range map[string][]bool{
		"after:2026/10/07 before:2026/10/08":          {true, false},
		"from:(quintoandar.com.br OR zapimoveis.com)": {false, true},
		"newer_than:2d":                 {true, false},
		"older_than:2d":                 {false, true},
		"category:promotions":           {false, true},
		"-category:promotions in:inbox": {true, false},
		"in:inbox":                      {true, true},
	} {
		match := mailMatcher(q, now, true)
		for i, it := range []map[string]any{yesterday, before} {
			if match(it) != want[i] {
				t.Errorf("%q on item %d: %v, want %v", q, i, match(it), want[i])
			}
		}
	}
}

// A watching routine in a world scenario is woken with what its watch
// finds there, filtered by the watch's own arguments.
func TestWatchInTheWorld(t *testing.T) {
	r := Routine{Manifest: runtime.Manifest{Capabilities: []string{"gmail.search", "telegram.send"},
		Watch: &runtime.Watch{Capability: "gmail.search", Args: map[string]any{"query": "from:ana@acme.com is:unread", "days": 1}, Key: "id"}},
		Code: `async function run() { for (const m of event.items) await telegram.send({text: m.subject}) }`}
	one := 1
	s := trace.Scenario{Now: "2026-10-07T16:00:00-03:00", World: true, Responses: []trace.Response{{Capability: "gmail.search", Result: json.RawMessage(`[
		{"id":"1","from":"ana@acme.com","subject":"Globex cancelou","date":"2026-10-07T15:00:00-03:00","labels":["INBOX","UNREAD"]},
		{"id":"2","from":"mariana@acme.com","subject":"Reembolso","date":"2026-10-07T14:00:00-03:00","labels":["INBOX","UNREAD"]}]`)}},
		Expect: []trace.Expect{{Capability: "telegram.send", Count: &one, Contains: []string{"Globex"}, NotContains: []string{"Reembolso"}}}}
	if o := Check(t.Context(), r, "holdout", s); !o.Passed {
		t.Fatal(o.Problems)
	}
}
