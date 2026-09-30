package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/telegram"
)

const testBot = "123456:mini-app-bot"

// initData is what Telegram hands the Mini App when the account id opens it.
func initData(token string, user int64, signed time.Time) string {
	return telegram.SignInitData(token, url.Values{
		"query_id":  {"AAHdF6IQ"},
		"user":      {`{"id":` + strconv.FormatInt(user, 10) + `,"first_name":"X"}`},
		"auth_date": {strconv.FormatInt(signed.Unix(), 10)},
	})
}

// exchange posts initData as the Mini App does, with no credential.
func (ta *testApp) exchange(t *testing.T, data string) (int, map[string]any, *http.Response) {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"init_data": data})
	resp, err := http.Post(ta.srv.URL+"/api/tg/session", "application/json", strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out, resp
}

// pairTelegram gives the owner Telegram account 1001 and Ana 4242, as
// sending /start with their codes would.
func (h *house) pairTelegram(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	h.Vault.Set(ctx, "telegram.token", testBot)
	h.TelegramAPI = "http://127.0.0.1:1" // nothing reaches Telegram
	h.Events.Put(ctx, "telegram.chat", "1001")
	ana, _ := h.People.Get(ctx, h.anaID)
	if _, err := h.People.Pair(ctx, ana.Invite, 4242); err != nil {
		t.Fatal(err)
	}
}

// Telegram's signed initData opens a short session of the person who
// paired that account, and nobody else's; anything unsigned, altered,
// old or from another bot opens nothing.
func TestMiniAppSignsInAsThePairedPerson(t *testing.T) {
	h := newHouse(t)
	h.pairTelegram(t)
	now := time.Now()

	status, out, resp := h.exchange(t, initData(testBot, 4242, now.Add(-time.Minute)))
	if status != 200 || out["person"] != h.anaID || out["role"] != "member" {
		t.Fatalf("Ana's Mini App: %d %v", status, out)
	}
	if len(resp.Cookies()) != 0 {
		t.Fatal("the Mini App session must not be a cookie")
	}
	ana := out["token"].(string)
	if exp, _ := time.Parse(time.RFC3339, out["expires"].(string)); exp.After(now.Add(miniAppLife + time.Minute)) {
		t.Fatalf("session lasts too long: %v", exp)
	}
	_, owner, _ := h.exchange(t, initData(testBot, 1001, now))
	if owner["person"] != "owner" || owner["token"] == "tok" || owner["token"] == "" {
		t.Fatalf("owner's Mini App: %v", owner)
	}

	for name, data := range map[string]string{
		"unpaired":    initData(testBot, 5555, now),
		"another bot": initData("999:other", 4242, now),
		"stale":       initData(testBot, 4242, now.Add(-2*time.Hour)),
		"tampered":    strings.Replace(initData(testBot, 1001, now), "1001", "4242", 1),
		"unsigned":    "user=" + url.QueryEscape(`{"id":4242}`) + "&auth_date=" + strconv.FormatInt(now.Unix(), 10),
	} {
		if status, out, _ := h.exchange(t, data); status == 200 || out["token"] != nil {
			t.Errorf("%s opened a session: %d %v", name, status, out)
		}
	}

	// Ana's Mini App sees her things and none of the owner's, and opens
	// only the Mini App's routes.
	for _, path := range []string{"/api/routines", "/api/widgets", "/api/dashboards", "/api/approvals", "/api/questions", "/api/cost", "/api/state"} {
		code, body := h.raw(t, ana, "GET", path, nil)
		if code != 200 || strings.Contains(body, h.ownerMark) {
			t.Errorf("%s: %d %s", path, code, body)
		}
	}
	if _, body := h.raw(t, ana, "GET", "/api/routines", nil); !strings.Contains(body, h.anaMark) {
		t.Errorf("Ana's routines are missing: %s", body)
	}
	for _, p := range h.Server.Patterns() {
		if strings.Contains(p, "/api/ws") {
			continue
		}
		for _, c := range []struct {
			token, other string
			ids          map[string]string
		}{{ana, h.ownerMark, h.owner}, {owner["token"].(string), h.anaMark, h.anas}} {
			method, path, ok := fill(p, c.ids)
			if !ok {
				continue
			}
			code, body := h.raw(t, c.token, method, path, js(map[string]any{}))
			if !miniAppRoutes[p] && code != http.StatusForbidden {
				t.Errorf("a Mini App session opened %s: %d", p, code)
			}
			if strings.Contains(body, c.other) {
				t.Errorf("%s leaked someone else's things: %s", p, body)
			}
		}
	}
	if code, _ := h.raw(t, owner["token"].(string), "GET", "/api/settings", nil); code != http.StatusForbidden {
		t.Errorf("the owner's Mini App opened settings: %d", code)
	}

	// Sessions run out, and a person keeps only a few.
	for range miniAppKeep + 2 {
		h.exchange(t, initData(testBot, 4242, now))
	}
	n := 0
	for _, d := range h.devices(context.Background()) {
		if d.MiniApp && d.Person == h.anaID {
			n++
		}
	}
	if n != miniAppKeep {
		t.Errorf("Ana has %d Mini App sessions", n)
	}
	list := h.devices(context.Background())
	for i := range list {
		list[i].Expires = now.Add(-time.Second)
	}
	h.saveDevices(context.Background(), list)
	if code, _ := h.raw(t, owner["token"].(string), "GET", "/api/state", nil); code != http.StatusUnauthorized {
		t.Errorf("an expired Mini App session still works: %d", code)
	}
	// Without Telegram set up there is nothing to sign in to.
	h.Vault.Delete(context.Background(), "telegram.token")
	if status, _, _ := h.exchange(t, initData(testBot, 4242, now)); status != 404 {
		t.Errorf("no bot: %d", status)
	}
}

// The bot offers the Mini App in each paired chat only while Pimpo has a
// public https address, and takes it back when the address goes.
func TestMiniAppMenuButtonNeedsHTTPS(t *testing.T) {
	h := newHouse(t)
	var mu sync.Mutex
	var calls []map[string]any
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/setChatMenuButton") {
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			mu.Lock()
			calls = append(calls, b)
			mu.Unlock()
		}
		w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer tg.Close()
	h.pairTelegram(t)
	h.TelegramAPI = tg.URL
	ctx := context.Background()
	buttons := func() map[float64]string {
		mu.Lock()
		defer mu.Unlock()
		out := map[float64]string{}
		for _, c := range calls {
			mb := c["menu_button"].(map[string]any)
			out[c["chat_id"].(float64)] = mb["type"].(string)
			if wa, ok := mb["web_app"].(map[string]any); ok {
				out[c["chat_id"].(float64)] = wa["url"].(string)
			}
		}
		calls = nil
		return out
	}

	for _, base := range []string{"", "http://192.168.1.20:7788"} {
		h.Events.Put(ctx, "public_url", base)
		h.syncMiniApp(ctx)
		if got := buttons(); len(got) != 0 || h.miniAppURL(ctx) != "" {
			t.Fatalf("offered a Mini App at %q: %v", base, got)
		}
	}
	h.Events.Put(ctx, "public_url", "https://pimpo.example.ts.net")
	h.syncMiniApp(ctx)
	want := "https://pimpo.example.ts.net/tg/app"
	if got := buttons(); got[1001] != want || got[4242] != want || len(got) != 2 {
		t.Fatalf("menu buttons: %v", got)
	}
	h.syncMiniApp(ctx)
	if got := buttons(); len(got) != 0 {
		t.Fatalf("asked Telegram again for nothing: %v", got)
	}
	h.Events.Put(ctx, "public_url", "")
	h.syncMiniApp(ctx)
	if got := buttons(); got[1001] != "default" || got[4242] != "default" {
		t.Fatalf("menu buttons not taken back: %v", got)
	}
}
