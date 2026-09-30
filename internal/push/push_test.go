package push

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/push/pushtest"
)

func claims(aud, email string, exp time.Time) map[string]any {
	return map[string]any{"iss": "https://accounts.google.com", "aud": aud, "email": email, "email_verified": true, "exp": exp.Unix(), "iat": time.Now().Unix()}
}

func TestPubSubTokenVerification(t *testing.T) {
	g := pushtest.NewFakeGoogle(t)
	v := &Verifier{CertsURL: g.Certs.URL}
	ctx := context.Background()
	aud, sa := "https://pimpo.example.ts.net/push/gmail", "push@proj.iam.gserviceaccount.com"
	soon := time.Now().Add(time.Hour)
	if err := v.Verify(ctx, g.Sign(claims(aud, sa, soon)), aud, sa); err != nil {
		t.Fatalf("a valid token was refused: %v", err)
	}
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	forged := &pushtest.FakeGoogle{Key: other, Kid: g.Kid}
	unverified := claims(aud, sa, soon)
	unverified["email_verified"] = false
	stranger := claims(aud, sa, soon)
	stranger["iss"] = "https://evil.example"
	bad := map[string]string{
		"forged signature": forged.Sign(claims(aud, sa, soon)),
		"other audience":   g.Sign(claims("https://evil.example/push", sa, soon)),
		"other account":    g.Sign(claims(aud, "someone@evil.example", soon)),
		"expired":          g.Sign(claims(aud, sa, time.Now().Add(-time.Minute))),
		"unverified email": g.Sign(unverified),
		"other issuer":     g.Sign(stranger),
		"not a JWT":        "abc",
		"alg none":         base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","kid":"k1"}`)) + "." + strings.Split(g.Sign(claims(aud, sa, soon)), ".")[1] + ".",
	}
	for name, tok := range bad {
		if v.Verify(ctx, tok, aud, sa) == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if v.Verify(ctx, g.Sign(claims(aud, sa, soon)), "", "") == nil {
		t.Error("a token was accepted with push not set up")
	}
}

func TestParseNotification(t *testing.T) {
	data := base64.StdEncoding.EncodeToString([]byte(`{"emailAddress":"Ana@Example.com","historyId":9876}`))
	n, err := ParseNotification([]byte(`{"message":{"data":"` + data + `","messageId":"m1"},"subscription":"projects/p/subscriptions/s"}`))
	if err != nil || n.Email != "ana@example.com" || n.HistoryID != 9876 || n.MessageID != "m1" {
		t.Fatalf("%+v %v", n, err)
	}
	if _, err := ParseNotification([]byte(`{"message":{"data":"bm9wZQ=="}}`)); err == nil {
		t.Fatal("garbage was read as a notification")
	}
}

func TestGmailWatchAndHistory(t *testing.T) {
	var got []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/users/me/watch":
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			if b["topicName"] != "projects/p/topics/t" {
				t.Errorf("topic %v", b)
			}
			w.Write([]byte(`{"historyId":"100","expiration":"1900000000000"}`))
		case "/users/me/history":
			if r.URL.Query().Get("startHistoryId") == "1" {
				http.Error(w, `{"error":{"message":"not found"}}`, 404)
				return
			}
			if r.URL.Query().Get("pageToken") == "" {
				w.Write([]byte(`{"history":[{"messagesAdded":[{"message":{"id":"a"}}]}],"historyId":"105","nextPageToken":"p2"}`))
				return
			}
			w.Write([]byte(`{"history":[{"messagesAdded":[{"message":{"id":"b"}}]}],"historyId":"106"}`))
		}
	}))
	defer api.Close()
	g := Gmail{API: api.URL, Token: func(context.Context) (string, error) { return "tok", nil }}
	ctx := context.Background()
	w, err := g.Watch(ctx, "projects/p/topics/t")
	if err != nil || w.HistoryID != 100 || w.Expires.UnixMilli() != 1900000000000 {
		t.Fatalf("%+v %v", w, err)
	}
	ids, latest, err := g.Added(ctx, 100)
	if err != nil || strings.Join(ids, ",") != "a,b" || latest != 106 {
		t.Fatalf("%v %d %v", ids, latest, err)
	}
	if _, _, err := g.Added(ctx, 1); err != ErrHistoryGone {
		t.Fatalf("old history: %v", err)
	}
	if !strings.HasSuffix(got[0], "Bearer tok") {
		t.Fatalf("no token: %v", got)
	}
}

func TestGitHubSignature(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write(body)
	good := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !SignatureOK("s3cret", body, good) {
		t.Fatal("a good signature was refused")
	}
	for _, h := range []string{"", "sha1=" + good[7:], "sha256=zz", "sha256=" + strings.Repeat("0", 64)} {
		if SignatureOK("s3cret", body, h) {
			t.Errorf("%q was accepted", h)
		}
	}
	if SignatureOK("other", body, good) || SignatureOK("s3cret", []byte(`{"action":"closed"}`), good) || SignatureOK("", body, good) {
		t.Fatal("a signature passed for another secret or body")
	}
}

func TestTrimKeepsWhatARoutineUses(t *testing.T) {
	in := map[string]any{"action": "opened", "url": "x", "comments_url": "x", "html_url": "https://github.com/o/r/pull/1",
		"body": strings.Repeat("a", 3000), "labels": make([]any, 50)}
	out := Trim(in).(map[string]any)
	if _, ok := out["comments_url"]; ok || out["html_url"] == nil || out["action"] != "opened" {
		t.Fatalf("%v", out)
	}
	if len([]rune(out["body"].(string))) != 2001 || len(out["labels"].([]any)) != 30 {
		t.Fatal("not clipped")
	}
}
