package app

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/company"
)

func (ta *testApp) public(t *testing.T, path string) (int, string) {
	t.Helper()
	resp, err := http.Get(ta.srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestTheShowcaseShowsOnlyWhatItsPersonPutThere(t *testing.T) {
	h := newHouse(t)
	ctx := context.Background()
	co := h.owner["company"]
	base := "/api/companies/" + co
	if code, _ := h.public(t, "/showcase/lume"); code != 404 {
		t.Fatalf("a showcase that is off: %d", code)
	}
	h.Companies.SaveBrief(ctx, company.Brief{ID: "b_ship", Company: co, Author: "rui", Title: "Dark <mode>", State: company.BriefShipped, Ref: "https://github.com/ana/shop/pull/9", Created: time.Now()})
	h.Companies.SaveBrief(ctx, company.Brief{ID: "b_open", Company: co, Author: "rui", Title: "Secret plan", State: company.BriefProposed, Created: time.Now()})

	if code, _ := h.raw(t, h.ana, "PUT", base+"/showcase", js(map[string]any{"on": true, "slug": "lume"})); code != 404 {
		t.Fatalf("Ana turned on someone else's showcase: %d", code)
	}
	if code, out := h.do(t, "PUT", base+"/showcase", map[string]any{"on": true, "slug": "Lume Moda!"}); code != 400 {
		t.Fatalf("an address with spaces: %d %v", code, out)
	}
	if code, out := h.do(t, "PUT", base+"/showcase", map[string]any{"on": true, "slug": "lume"}); code != 200 {
		t.Fatalf("on: %d %v", code, out)
	}
	for _, item := range []map[string]any{
		{"kind": "brief", "ref": "b_ship", "note": "It shipped in a week"},
		{"kind": "link", "title": "Our shop", "url": "https://lume.example"},
	} {
		if code, out := h.do(t, "POST", base+"/showcase/items", item); code != 200 {
			t.Fatalf("%v: %d %v", item, code, out)
		}
	}
	for _, bad := range []map[string]any{
		{"kind": "brief", "ref": "b_open"},
		{"kind": "video", "ref": "m_000000000000"},
		{"kind": "link", "title": "x", "url": "javascript:alert(1)"},
		{"kind": "secret", "title": "x"},
	} {
		if code, _ := h.do(t, "POST", base+"/showcase/items", bad); code < 400 {
			t.Errorf("%v went on the showcase", bad)
		}
	}
	// An ordinary change of the company keeps the showcase as it is.
	o, _ := h.Companies.Org(ctx, co)
	h.do(t, "PUT", base, map[string]any{"name": o.Name, "showcase": map[string]any{"on": true, "slug": "other", "items": []any{}}})

	code, page := h.public(t, "/showcase/lume")
	if code != 200 || !strings.Contains(page, "Dark &lt;mode&gt;") || !strings.Contains(page, "https://lume.example") || strings.Contains(page, "Secret plan") {
		t.Fatalf("page: %d %s", code, page)
	}
	// The company's name shows; who runs it and its agents do not.
	if strings.Contains(page, "rui") || strings.Contains(page, "Ana") || strings.Contains(page, "CEO") {
		t.Fatalf("the page names someone: %s", page)
	}
	h.do(t, "PUT", base+"/showcase", map[string]any{"on": false, "slug": "lume"})
	if code, _ := h.public(t, "/showcase/lume"); code != 404 {
		t.Fatalf("a showcase turned off: %d", code)
	}
}
