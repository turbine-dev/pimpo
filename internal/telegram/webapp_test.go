package telegram

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

const botToken = "123456:ABC-test-token"

// signed builds initData by hand, the way Telegram's documentation spells
// it out, independent of SignInitData.
func signed(token string, authDate time.Time, user string) string {
	auth := strconv.FormatInt(authDate.Unix(), 10)
	check := "auth_date=" + auth + "\nquery_id=AAHdF6IQ\nuser=" + user
	k := hmac.New(sha256.New, []byte("WebAppData"))
	k.Write([]byte(token))
	m := hmac.New(sha256.New, k.Sum(nil))
	m.Write([]byte(check))
	return "query_id=AAHdF6IQ&user=" + url.QueryEscape(user) + "&auth_date=" + auth + "&hash=" + hex.EncodeToString(m.Sum(nil))
}

func TestVerifyInitData(t *testing.T) {
	now := time.Now()
	user := `{"id":4242,"first_name":"Ana","language_code":"pt-br"}`
	good := signed(botToken, now.Add(-time.Minute), user)
	u, err := VerifyInitData(good, botToken, time.Hour, now)
	if err != nil || u.ID != 4242 || u.FirstName != "Ana" || u.Language != "pt-br" {
		t.Fatalf("valid initData refused: %+v %v", u, err)
	}
	// The same shape through the helper tests use elsewhere.
	helper := SignInitData(botToken, url.Values{"user": {user}, "auth_date": {strconv.FormatInt(now.Unix(), 10)}})
	if _, err := VerifyInitData(helper, botToken, time.Hour, now); err != nil {
		t.Fatalf("SignInitData made data that does not verify: %v", err)
	}

	tampered := strings.Replace(good, "4242", "4243", 1)
	cases := map[string]struct {
		data, token string
		want        error
	}{
		"tampered user":     {tampered, botToken, ErrInitData},
		"another bot":       {good, "999:other", ErrInitData},
		"stale":             {signed(botToken, now.Add(-2*time.Hour), user), botToken, ErrStale},
		"from the future":   {signed(botToken, now.Add(time.Hour), user), botToken, ErrStale},
		"no hash":           {"user=" + url.QueryEscape(user) + "&auth_date=1", botToken, ErrInitData},
		"field given twice": {good + "&user=" + url.QueryEscape(`{"id":1}`), botToken, ErrInitData},
		"hash not hex":      {strings.Split(good, "&hash=")[0] + "&hash=zz", botToken, ErrInitData},
		"no bot token":      {good, "", ErrInitData},
		"empty":             {"", botToken, ErrInitData},
		"no user":           {SignInitData(botToken, url.Values{"auth_date": {strconv.FormatInt(now.Unix(), 10)}}), botToken, ErrInitData},
	}
	for name, c := range cases {
		if _, err := VerifyInitData(c.data, c.token, time.Hour, now); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", name, err, c.want)
		}
	}
}

func TestSetMenuButton(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/setChatMenuButton") {
			t.Errorf("unexpected call %s", r.URL.Path)
		}
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		bodies = append(bodies, b)
		w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()
	b := Bot{Token: "t", BaseURL: srv.URL}
	b.SetMenuButton(context.Background(), 7, "Dashboard", "https://pimpo.example.ts.net/tg/app")
	b.SetMenuButton(context.Background(), 7, "", "")
	if len(bodies) != 2 {
		t.Fatal(bodies)
	}
	first := bodies[0]["menu_button"].(map[string]any)
	if first["type"] != "web_app" || first["web_app"].(map[string]any)["url"] != "https://pimpo.example.ts.net/tg/app" || bodies[0]["chat_id"].(float64) != 7 {
		t.Fatalf("menu button: %v", bodies[0])
	}
	if bodies[1]["menu_button"].(map[string]any)["type"] != "default" {
		t.Fatalf("menu button not reset: %v", bodies[1])
	}
}
