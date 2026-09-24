package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGetJSONWithinScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rate":
			w.Write([]byte(`{"rate":5.47}`))
		case "/away":
			http.Redirect(w, r, "http://example.org/x", http.StatusFound)
		default:
			w.Write([]byte(`<html>`))
		}
	}))
	defer srv.Close()
	host := strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")[0]
	w := &Web{AllowPrivate: true}
	ctx := context.Background()
	got, err := w.Call(ctx, "http.getJSON", host, srv.URL+"/rate")
	if err != nil || got.(map[string]any)["rate"] != 5.47 {
		t.Fatalf("%v %v", got, err)
	}
	for name, u := range map[string]string{"not json": srv.URL + "/page", "redirect out": srv.URL + "/away", "other host": "http://example.org/x", "not a url": "ftp://x"} {
		if _, err := w.Call(ctx, "http.getJSON", host, u); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestPrivateAddressesAreRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) }))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	_, err := (&Web{}).Call(context.Background(), "http.getJSON", u.Hostname(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "private address") {
		t.Fatalf("got %v", err)
	}
}
