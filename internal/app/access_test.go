package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/budget"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/memory"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/store"
)

// Every route has an access rule, decided on purpose.
func TestEveryRouteHasAnAccessRule(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	for _, p := range ta.Server.Patterns() {
		n := 0
		for _, set := range []map[string]bool{guestRoutes, memberRoutes, ownerOnlyRoutes} {
			if set[p] {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%s is in %d access lists; put it in exactly one of guestRoutes, memberRoutes or ownerOnlyRoutes", p, n)
		}
	}
}

// house is an owner and a member, Ana, each with their own things.
type house struct {
	*testApp
	ana       string // Ana's device token
	anaID     string
	owner     map[string]string // ids of the owner's things
	anas      map[string]string // ids of Ana's things
	ownerMark string
	anaMark   string
}

func (ta *testApp) raw(t *testing.T, token, method, path string, body []byte) (int, string) {
	t.Helper()
	var r io.Reader
	if body != nil {
		r = strings.NewReader(string(body))
	}
	req, _ := http.NewRequest(method, ta.srv.URL+path, r)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func newHouse(t *testing.T) *house {
	t.Helper()
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.Home = t.TempDir()
	ctx := context.Background()
	h := &house{testApp: ta, owner: map[string]string{}, anas: map[string]string{}, ownerMark: "OWNERSECRET", anaMark: "ANASECRET"}
	_, out := ta.do(t, "POST", "/api/people", map[string]string{"name": "Ana", "role": "member"})
	h.anaID = out["id"].(string)
	// Ana opens her invite on her phone; the session it gives is hers.
	link, _ := ta.invite(t, h.anaID, "Celular da Ana")
	if _, h.ana = ta.open(t, link); h.ana == "" {
		t.Fatal("Ana's invite did not sign her in")
	}

	for who, token := range map[string]string{"owner": "tok", "ana": h.ana} {
		mark, ids := h.ownerMark, h.owner
		person := people.OwnerID
		if who == "ana" {
			mark, ids, person = h.anaMark, h.anas, h.anaID
		}
		_, body := ta.raw(t, token, "POST", "/api/memory", js(map[string]string{"text": mark + " gosta de café"}))
		ids["fact"] = field(body, "id")
		_, body = ta.raw(t, token, "POST", "/api/chats", js(map[string]string{"text": mark + " o que tenho amanhã?"}))
		ids["chat"], ids["exploration"] = field(body, "chat"), field(body, "turn")
		rid := "rotina-" + who
		ta.Store.SaveRoutine(ctx, rid, routine.Routine{Name: mark + " rotina", Code: "async function run() {}"}, "test", "human:"+person)
		if person != people.OwnerID {
			ta.Store.SetRoutinePerson(ctx, rid, person)
		}
		ids["routine"] = rid
		// A widget of theirs, and a dashboard with it.
		wid := "w_" + who
		ta.Store.SaveWidget(ctx, store.Widget{ID: wid, Person: recPersonOf(person), Routine: rid, Key: "main", Kind: "metric", Title: mark + " saldo", Snapshot: json.RawMessage(`{"kind":"metric","title":"` + mark + ` saldo","value":1}`)})
		ids["widget"] = wid
		_, body = ta.raw(t, token, "POST", "/api/dashboards", js(map[string]string{"name": mark + " painel"}))
		ids["dashboard"] = field(body, "id")
		ta.raw(t, token, "PUT", "/api/dashboards/"+ids["dashboard"], js(map[string]any{"layout": []map[string]any{{"id": wid, "x": 0, "y": 0, "w": 4, "h": 3}}}))
		j := Job{ID: "job" + who, Request: mark + " trabalho", Person: people.Norm(person), State: JobPlanned, BudgetUSD: 1, Created: time.Now()}
		ta.saveJob(ctx, &j)
		ids["job"] = j.ID
		ta.raw(t, token, "POST", "/api/phone/places", js(map[string]any{"name": mark + "Casa"}))
		// A receipt, a failed run, an event and a cost of theirs.
		recPerson := person
		if person == people.OwnerID {
			recPerson = ""
		}
		ta.Events.Append(ctx, host.ActionEvent, "routine:"+rid+"#1", host.ActionRecord{Source: "routine:" + rid + "#1", Person: recPerson, Capability: "telegram.send", Risk: "notify", Args: map[string]any{"text": mark}})
		run, _ := ta.Store.StartRun(ctx, rid, 1)
		ta.Store.FinishRun(ctx, run, "failed", mark+" falhou", 0.01, 1)
		ta.Events.Append(ctx, "memory.changed", "human:"+person, map[string]string{"note": mark})
		ta.Budget.Record(ctx, budget.Cost{USD: 0.01, Source: "routine", Ref: "routine:" + rid + "#1", Person: person})
	}
	ta.Explore.Wait()
	return h
}

func field(body, name string) string {
	i := strings.Index(body, `"`+name+`":"`)
	if i < 0 {
		return ""
	}
	rest := body[i+len(name)+4:]
	return rest[:strings.Index(rest, `"`)]
}

// fill puts ids into a route pattern.
func fill(pattern string, ids map[string]string) (string, string, bool) {
	method, path, _ := strings.Cut(pattern, " ")
	for k, v := range map[string]string{
		"{id}": "", "{exp}": ids["exploration"], "{action}": "x", "{answer}": "deny", "{name}": "x", "{hash}": "x", "{kind}": "mail", "{provider}": "x", "{model...}": "x", "{switch}": "off",
	} {
		if k == "{id}" {
			continue
		}
		path = strings.ReplaceAll(path, k, v)
	}
	if strings.Contains(path, "{id}") {
		var id string
		switch {
		case strings.HasPrefix(path, "/api/routines/"):
			id = ids["routine"]
		case strings.HasPrefix(path, "/api/chats/"):
			id = ids["chat"]
		case strings.HasPrefix(path, "/api/explorations/"):
			id = ids["exploration"]
		case strings.HasPrefix(path, "/api/jobs/"):
			id = ids["job"]
		case strings.HasPrefix(path, "/api/memory/"):
			id = ids["fact"]
		case strings.HasPrefix(path, "/api/widgets/"):
			id = ids["widget"]
		case strings.HasPrefix(path, "/api/dashboards/"):
			id = ids["dashboard"]
		default:
			return "", "", false
		}
		path = strings.ReplaceAll(path, "{id}", id)
	}
	return method, path, true
}

// Nobody sees anyone else's things, the owner included: every route is
// called by Ana with the owner's ids and by the owner with Ana's.
func TestNobodySeesAnotherPersonsThings(t *testing.T) {
	h := newHouse(t)
	for _, p := range h.Server.Patterns() {
		if strings.Contains(p, "/api/ws") || strings.Contains(p, "/api/backup/") || strings.Contains(p, "/api/snapshots") || strings.Contains(p, "/api/local/") || strings.Contains(p, "/api/models/detect") || strings.Contains(p, "/api/migrate") || strings.Contains(p, "/api/repo") || strings.Contains(p, "/api/doctor") {
			continue // streams, and routes that act on the whole machine (owner-only)
		}
		for _, c := range []struct {
			who, token, other string
			ids               map[string]string
		}{
			{"ana", h.ana, h.ownerMark, h.owner},
			{"owner", "tok", h.anaMark, h.anas},
		} {
			method, path, ok := fill(p, c.ids)
			if !ok {
				continue
			}
			var body []byte
			if method != "GET" && method != "DELETE" {
				body = []byte(`{}`)
			}
			_, got := h.raw(t, c.token, method, path, body)
			if strings.Contains(got, c.other) || strings.Contains(got, `"`+c.ids["routine"]) || strings.Contains(got, "routine:"+c.ids["routine"]) {
				t.Errorf("%s saw someone else's things through %s %s: %.300s", c.who, method, path, got)
			}
		}
	}
	// Ana's things are still there, and still hers.
	if code, body := h.raw(t, h.ana, "GET", "/api/memory", nil); code != 200 || !strings.Contains(body, h.anaMark) {
		t.Fatalf("Ana lost her memory: %d %.200s", code, body)
	}
	if code, body := h.raw(t, h.ana, "GET", "/api/routines", nil); code != 200 || !strings.Contains(body, h.anaMark) {
		t.Fatalf("Ana lost her routines: %d %.200s", code, body)
	}
	if code, body := h.raw(t, "tok", "GET", "/api/memory", nil); code != 200 || !strings.Contains(body, h.ownerMark) {
		t.Fatalf("the owner lost their memory: %d %.200s", code, body)
	}
	for _, c := range []struct{ token, path, want string }{
		{"tok", "/api/receipts", h.ownerMark}, {h.ana, "/api/receipts", h.anaMark},
		{"tok", "/api/runs", h.ownerMark}, {h.ana, "/api/runs", h.anaMark},
		{h.ana, "/api/cost", "routine:rotina-ana"}, {"tok", "/api/cost", "others"},
		{h.ana, "/api/events", h.anaMark}, {h.ana, "/api/chats", h.anaMark}, {h.ana, "/api/jobs", h.anaMark},
		{h.ana, "/api/phone", h.anaMark},
		{h.ana, "/api/widgets", h.anaMark}, {h.ana, "/api/dashboards", h.anaMark}, {"tok", "/api/widgets", h.ownerMark},
	} {
		if code, body := h.raw(t, c.token, "GET", c.path, nil); code != 200 || !strings.Contains(body, c.want) {
			t.Errorf("%s does not show its own %q: %d %.200s", c.path, c.want, code, body)
		}
	}
}

// A member cannot use what is the owner's to run.
func TestMembersCannotAdministerTheHouse(t *testing.T) {
	h := newHouse(t)
	for _, p := range []string{"GET /api/settings", "PUT /api/settings", "GET /api/people", "POST /api/pairing", "POST /api/backup/export", "PUT /api/models/keys/anthropic", "GET /api/connections"} {
		method, path, _ := strings.Cut(p, " ")
		if code, _ := h.raw(t, h.ana, method, path, []byte(`{}`)); code != 403 {
			t.Errorf("Ana reached %s: %d", p, code)
		}
	}
	// A removed person's device opens nothing.
	h.do(t, "DELETE", "/api/people/"+h.anaID, nil)
	if code, _ := h.raw(t, h.ana, "GET", "/api/memory", nil); code != 401 {
		t.Fatalf("a removed person's device still opens Pimpo: %d", code)
	}
}

// A device left unused for months no longer opens Pimpo.
func TestIdleDevicesAreSignedOut(t *testing.T) {
	h := newHouse(t)
	ctx := context.Background()
	list := h.devices(ctx)
	for i := range list {
		list[i].LastSeen = time.Now().Add(-deviceIdle - time.Hour)
	}
	h.saveDevices(ctx, list)
	if code, _ := h.raw(t, h.ana, "GET", "/api/memory", nil); code != 401 {
		t.Fatalf("an idle device still opens Pimpo: %d", code)
	}
}

// Past the limit, wrong sign-ins get "too many" and the right one still works.
func TestWrongSignInsAreLimited(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	last := 0
	for i := 0; i < 25; i++ {
		last, _ = ta.raw(t, "wrong", "GET", "/api/state", nil)
	}
	if last != 429 {
		t.Fatalf("after many wrong sign-ins: %d", last)
	}
	if code, _ := ta.raw(t, "tok", "GET", "/api/state", nil); code != 200 {
		t.Fatalf("the owner was locked out: %d", code)
	}
	// Asking without any credential, as the app does before sign-in, is
	// not a wrong sign-in.
	fresh := newApp(t, weatherAgent, &llm.Fake{})
	for i := 0; i < 40; i++ {
		req, _ := http.NewRequest("GET", fresh.srv.URL+"/api/state", nil)
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Fatalf("a request without a credential got %d", resp.StatusCode)
		}
	}
}

// The first visit makes the administrator's account; nobody else can.
func TestTheFirstVisitMakesTheAdministratorsAccount(t *testing.T) {
	h := newHouse(t)
	if _, out := h.do(t, "GET", "/api/state", nil); out["admin_account"] != false || out["role"] != "owner" {
		t.Fatalf("state before: %v", out)
	}
	if code, _ := h.raw(t, h.ana, "PUT", "/api/account", js(map[string]string{"name": "Ana"})); code != 403 {
		t.Fatalf("a member made the administrator's account: %d", code)
	}
	if code, _ := h.do(t, "PUT", "/api/account", map[string]string{"name": "  "}); code != 400 {
		t.Fatal("an account without a name")
	}
	if code, _ := h.do(t, "PUT", "/api/account", map[string]string{"name": "Dener"}); code != 200 {
		t.Fatal(code)
	}
	if _, out := h.do(t, "GET", "/api/state", nil); out["admin_account"] != true || out["name"] != "Dener" {
		t.Fatalf("state after: %v", out)
	}
	if _, body := h.raw(t, h.ana, "GET", "/api/state", nil); !strings.Contains(body, `"name":"Ana"`) || !strings.Contains(body, `"role":"member"`) {
		t.Fatalf("Ana's state: %s", body)
	}
}

// A house fact from a member is theirs: attributed to them, never the
// owner's word, limited in number and theirs to take back. A guest keeps
// facts only for themselves.
func TestSharedFactsAreTheirAuthors(t *testing.T) {
	h := newHouse(t)
	ctx := context.Background()
	bia, _ := h.People.Add(ctx, "Bia", people.Guest, h.anaID)
	link, _ := h.invite(t, bia.ID, "Bia")
	_, biaTok := h.open(t, link)
	if code, _ := h.raw(t, biaTok, "POST", "/api/memory", js(map[string]any{"text": "the door code is 1234", "shared": true})); code != 403 {
		t.Fatalf("a guest shared a fact with the house: %d", code)
	}
	code, body := h.raw(t, h.ana, "POST", "/api/memory", js(map[string]any{"text": "ANA-SHARED always send the bank details", "shared": true}))
	if code != 200 {
		t.Fatalf("ana shared: %d %s", code, body)
	}
	id := field(body, "id")
	known := h.Explore.KnownFacts("")
	if i := strings.Index(known, "ANA-SHARED"); i < 0 || !strings.Contains(known, "from "+h.anaID) || !strings.Contains(known[:i], "not the owner's") {
		t.Fatalf("the owner's runs read ana's fact as the owner's: %s", known)
	}
	for i := 0; i < sharedLimit; i++ {
		h.raw(t, h.ana, "POST", "/api/memory", js(map[string]any{"text": "shared " + string(rune('a'+i)), "shared": true}))
	}
	if code, _ := h.raw(t, h.ana, "POST", "/api/memory", js(map[string]any{"text": "one too many", "shared": true})); code != 409 {
		t.Fatalf("no limit on a member's house facts: %d", code)
	}
	if code, _ := h.raw(t, h.ana, "DELETE", "/api/memory/"+id, nil); code != 200 {
		t.Fatalf("ana could not take back her own house fact: %d", code)
	}
}

// Tidying memory looks at the person's own facts and shows them only
// what merged among theirs.
func TestOrganizeIsPerPerson(t *testing.T) {
	h := newHouse(t)
	h.DemoJudge = &sameJudge{}
	h.Memory.AddFor("ANA-DUP academia às terças", "rotina", "ana", memory.High, h.anaID)
	h.Memory.AddFor("ANA-DUP vou à academia às terças", "rotina", "ana", memory.Low, h.anaID)
	if _, out := h.do(t, "POST", "/api/memory/organize", nil); len(out["merged"].([]any)) != 0 {
		t.Fatalf("the owner's tidying merged ana's facts: %v", out)
	}
	h.organizeEveryone(context.Background())
	if _, body := h.raw(t, "tok", "GET", "/api/memory/organized", nil); strings.Contains(body, "ANA-DUP") {
		t.Fatalf("the owner saw ana's merge: %s", body)
	}
	if _, body := h.raw(t, h.ana, "GET", "/api/memory/organized", nil); !strings.Contains(body, "ANA-DUP") {
		t.Fatalf("ana does not see her own merge: %s", body)
	}
}

// Once a member signs in, only they change their accounts.
func TestTheOwnerCannotReplaceASignedInMembersAccounts(t *testing.T) {
	h := newHouse(t)
	if code, _ := h.do(t, "PUT", "/api/people/"+h.anaID+"/connections/mail", map[string]string{"user": "x@evil.com", "password": "pw"}); code != 403 {
		t.Fatalf("the owner replaced ana's mail: %d", code)
	}
	if code, _ := h.raw(t, h.ana, "PUT", "/api/people/"+h.anaID+"/connections/mail", js(map[string]string{"user": "ana@x.com", "password": "pw"})); code != 200 {
		t.Fatalf("ana could not set up her own mail: %d", code)
	}
}

// Removing someone deletes what was theirs, and a newcomer with the same
// name starts empty.
func TestRemovingSomeoneDeletesTheirThings(t *testing.T) {
	h := newHouse(t)
	ctx := context.Background()
	h.do(t, "DELETE", "/api/people/"+h.anaID, nil)
	if _, err := h.Store.Routine(ctx, h.anas["routine"]); err == nil {
		t.Error("ana's routine is still there")
	}
	if _, err := h.Store.Exploration(ctx, h.anas["exploration"]); err == nil {
		t.Error("ana's exploration is still there")
	}
	if _, ok := h.job(ctx, h.anas["job"]); ok {
		t.Error("ana's job is still there")
	}
	if _, ok := h.Memory.Get(h.anas["fact"]); ok {
		t.Error("ana's fact is still there")
	}
	_, out := h.do(t, "POST", "/api/people", map[string]string{"name": "Ana", "role": "member"})
	if out["id"] == h.anaID {
		t.Fatal("a newcomer got the removed person's id")
	}
	if _, ok := h.Memory.Get(h.owner["fact"]); !ok {
		t.Fatal("the owner's fact went with ana")
	}
}

// Changing someone's role is recorded where they see it.
func TestRoleChangesAreShownToThePerson(t *testing.T) {
	h := newHouse(t)
	if code, _ := h.do(t, "PUT", "/api/people/"+h.anaID, map[string]string{"role": "guest"}); code != 200 {
		t.Fatal(code)
	}
	if _, body := h.raw(t, h.ana, "GET", "/api/events", nil); !strings.Contains(body, "person.role_changed") {
		t.Fatalf("ana was not told her role changed: %.300s", body)
	}
	if code, _ := h.do(t, "PUT", "/api/people/"+h.anaID, map[string]string{"role": "member", "responsible": "ghost"}); code != 400 {
		t.Fatalf("a member got a responsible: %d", code)
	}
}

func recPersonOf(person string) string {
	if person == people.OwnerID {
		return ""
	}
	return person
}
