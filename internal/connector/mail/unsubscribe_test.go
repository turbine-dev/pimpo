package mail

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListUnsubscribeHeaders(t *testing.T) {
	for raw, want := range map[string]string{
		"List-Unsubscribe: <mailto:out@news.com>, <https://news.com/u/1>\r\nList-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n\r\n": "https://news.com/u/1 true",
		"List-Unsubscribe: <https://news.com/u/1>, <mailto:out@news.com?subject=stop>\r\n\r\n":                                         "mailto:out@news.com?subject=stop false",
		"Subject: hi\r\n\r\n": " false",
	} {
		target, one := listUnsubscribe([]byte(raw))
		got := target + " " + map[bool]string{true: "true", false: "false"}[one]
		if got != want {
			t.Errorf("%q: got %q, want %q", raw, got, want)
		}
	}
}

func TestOneClickUnsubscribe(t *testing.T) {
	var body string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = r.Method + " " + string(b)
	}))
	defer srv.Close()
	old := unsubscribeClient
	unsubscribeClient = srv.Client()
	defer func() { unsubscribeClient = old }()

	addr := startServerWith(t, "From: News <news@techweekly.com>\r\nTo: eu@exemplo.com\r\nSubject: Weekly\r\nMessage-ID: <w1@x>\r\nList-Unsubscribe: <"+srv.URL+"/u/42>\r\nList-Unsubscribe-Post: List-Unsubscribe=One-Click\r\nDate: Mon, 01 Jan 2024 10:00:00 +0000\r\n\r\nNews!\r\n")
	m := newMail(addr)
	got := inbox(t, m, "")
	if len(got) != 1 || !got[0].CanUnsubscribe {
		t.Fatalf("search %+v", got)
	}
	res, err := m.Call(context.Background(), "gmail.unsubscribe", "", map[string]any{"id": got[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["method"] != "one-click" || body != "POST List-Unsubscribe=One-Click" {
		t.Fatalf("result %v body %q", res, body)
	}
}

// The link comes from a stranger's email: it must not reach this computer.
func TestOneClickUnsubscribeStaysOffPrivateAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	resp, err := unsubscribeClient.Post(srv.URL, "application/x-www-form-urlencoded", strings.NewReader("List-Unsubscribe=One-Click"))
	if err == nil {
		resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "private address") {
		t.Fatalf("posted to a loopback address: %v", err)
	}
}
