package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/turbine-dev/pimpo/internal/connector/services"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/workspace"
)

func TestWhatMembersPublishSaysItWasMadeWithAI(t *testing.T) {
	var mu sync.Mutex
	var posted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Commentary string }
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		mu.Lock()
		posted = append(posted, body.Commentary)
		mu.Unlock()
		w.Header().Set("x-restli-id", "urn:li:share:1")
	}))
	defer srv.Close()
	services.BaseURL["linkedin"] = srv.URL
	defer delete(services.BaseURL, "linkedin")

	ta, co := clerk(t)
	ta.Workspaces = workspace.Spaces{Root: t.TempDir()}
	ctx := context.Background()
	base := "/api/companies/" + co
	if code, out := ta.do(t, "PUT", base+"/members/clara/accounts/linkedin", map[string]string{"token": "li", "author": "urn:li:organization:42", "account_kind": "brand"}); code != 200 {
		t.Fatalf("Clara's LinkedIn: %d %v", code, out)
	}
	pc := publishCap{ta.App}
	clara := asMember(co+"/clara", "linkedin.post")
	if _, err := pc.Call(clara, "linkedin.post", "", map[string]any{"text": "We shipped dark mode."}); err != nil {
		t.Fatal(err)
	}
	o, _ := ta.Companies.Org(ctx, co)
	o.Disclosure = "Escrito com ajuda de IA."
	if _, err := ta.Companies.Update(ctx, o.Company); err != nil {
		t.Fatal(err)
	}
	pc.Call(clara, "linkedin.post", "", map[string]any{"text": "Second post"})
	if len(posted) != 2 || posted[0] != "We shipped dark mode.\n\nMade with the help of AI." || !strings.HasSuffix(posted[1], "Escrito com ajuda de IA.") {
		t.Fatalf("posted = %q", posted)
	}
	o.Disclosure = "AI"
	if _, err := ta.Companies.Update(ctx, o.Company); err == nil {
		t.Fatal("a disclosure too short to say anything")
	}

	if _, err := pc.Call(clara, "publish.assisted", "", map[string]any{"where": "Instagram", "text": "New reel"}); err != nil {
		t.Fatal(err)
	}
	evs, _ := ta.Events.List(ctx, event.Query{Types: []string{"company.publish.assisted"}})
	if len(evs) != 1 {
		t.Fatalf("events = %+v", evs)
	}
	if _, err := pc.Call(asMember(co+"/clara", "youtube.upload"), "youtube.upload", "", map[string]any{"media": "m_000000000000", "title": "x"}); err == nil || !strings.Contains(err.Error(), "no media") {
		t.Fatalf("uploaded media the company does not have: %v", err)
	}
	if _, err := pc.Call(ctx, "youtube.upload", "", map[string]any{"media": "m_000000000000", "title": "x"}); err == nil {
		t.Fatal("the person uploaded a company's video as a member")
	}
}
