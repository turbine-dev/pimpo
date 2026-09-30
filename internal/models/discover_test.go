package models

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// The list is asked again with its ETag, a 304 reuses the last answer, and
// Discover keeps it a day unless asked for a fresh one.
func TestDiscoveryIsCachedAndConditional(t *testing.T) {
	var hits, notModified atomic.Int32
	anth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("If-None-Match") == `"v1"` {
			notModified.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if r.URL.Query().Get("after_id") == "claude-a" {
			w.Write([]byte(`{"data":[{"id":"claude-b"}],"has_more":false}`))
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Write([]byte(`{"data":[{"id":"claude-a","display_name":"Claude A"}],"has_more":true,"last_id":"claude-a"}`))
	}))
	defer anth.Close()
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"data":[]}`)) }))
	defer or.Close()
	c := &Client{OpenRouter: or.URL}
	e := Endpoint{Provider: "anthropic", Base: anth.URL, Key: "k"}
	ctx := context.Background()
	ms, err := c.Discover(ctx, e, false)
	if err != nil || len(ms) != 2 || ms[0].Name != "Claude A" || ms[1].ID != "claude-b" {
		t.Fatalf("pages not followed: %+v %v", ms, err)
	}
	if _, err := c.Discover(ctx, e, false); err != nil || hits.Load() != 2 {
		t.Fatalf("a day's cache asked again: %d hits", hits.Load())
	}
	ms, err = c.Discover(ctx, e, true)
	if err != nil || len(ms) != 2 || notModified.Load() != 1 {
		t.Fatalf("fresh look did not reuse the 304: %+v %v, %d", ms, err, notModified.Load())
	}
}

// OpenRouter's own list carries prices, so its new models are priced.
func TestOpenRouterNewModelsArePriced(t *testing.T) {
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"x/new-one","name":"New","pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`))
	}))
	defer or.Close()
	live, err := (&Client{OpenRouter: or.URL}).Discover(context.Background(), Endpoint{Provider: "openrouter"}, false)
	if err != nil {
		t.Fatal(err)
	}
	out, _, _ := Merge(live, nil, map[string]time.Time{"x/old": {}}, time.Now())
	if len(out) != 1 || !out[0].New || !out[0].Priced || out[0].PriceIn != 1 || out[0].PriceOut != 2 {
		t.Fatalf("%+v", out)
	}
}

func TestMergeMarksNewAndRetired(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	live := []Model{{ID: "keep", Name: "keep", PriceIn: 9, PriceOut: 9, Priced: true}, {ID: "fresh", Name: "fresh"}}
	mine := []Owned{{ID: "keep", PriceIn: 3, PriceOut: 15}, {ID: "gone", PriceIn: 1, PriceOut: 5}}

	// The first look sets the baseline: nothing is new yet.
	out, retired, seen := Merge(live, mine, nil, now)
	for _, m := range out {
		if m.New {
			t.Fatalf("new on the first look: %+v", m)
		}
	}
	if len(retired) != 1 || retired[0] != "gone" || !out[0].Retired || out[0].ID != "gone" {
		t.Fatalf("retired %v %+v", retired, out)
	}
	if m := out[2]; m.ID != "keep" || !m.Mine || m.PriceIn != 3 || m.PriceOut != 15 {
		t.Fatalf("the owner's price did not win: %+v", m)
	}

	// A model listed later is new, with no price, until two weeks pass.
	live = append(live, Model{ID: "brand"})
	out, _, seen = Merge(live, mine, seen, now)
	var brand Model
	for _, m := range out {
		if m.ID == "brand" {
			brand = m
		}
		if m.ID == "fresh" && m.New {
			t.Fatal("a model from the first look became new")
		}
	}
	if !brand.New || brand.Priced {
		t.Fatalf("brand %+v", brand)
	}
	out, _, _ = Merge(live, mine, seen, now.Add(NewFor+time.Hour))
	for _, m := range out {
		if m.New {
			t.Fatalf("still new after two weeks: %+v", m)
		}
	}
}
