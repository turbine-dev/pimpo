package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/llm"
)

// open follows a sign-in link as a browser does, and returns the session
// it set, if any.
func (ta *testApp) open(t *testing.T, link string, cookies ...*http.Cookie) (int, string) {
	t.Helper()
	u, _ := url.Parse(link)
	req, _ := http.NewRequest("GET", ta.srv.URL+"/auth?"+u.RawQuery, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == "pimpo_session" {
			return resp.StatusCode, c.Value
		}
	}
	return resp.StatusCode, ""
}

func (ta *testApp) invite(t *testing.T, person, name string) (link string, out map[string]any) {
	t.Helper()
	_, out = ta.do(t, "POST", "/api/pairing", map[string]string{"base": "https://pimpo.example.com", "device": name, "person": person})
	link, _ = out["link"].(string)
	return link, out
}

// A link the owner makes for Ana is an invite: it opens nothing by itself,
// opens one session of hers the first time, and then nothing again. Ana
// sees the new device in her activity; the owner does not.
func TestAnInviteWorksOnceForItsPerson(t *testing.T) {
	h := newHouse(t)
	link, out := h.invite(t, h.anaID, "Tablet da Ana")
	if out["expires"] == nil || strings.Contains(link, "#home=") {
		t.Fatalf("invite answer: %v", out)
	}
	token, _ := url.Parse(link)
	if code, _ := h.raw(t, token.Query().Get("token"), "GET", "/api/memory", nil); code != 401 {
		t.Fatalf("an invite's token opened the API: %d", code)
	}
	code, session := h.open(t, link)
	if code != 303 || session == "" || session == token.Query().Get("token") {
		t.Fatalf("first use: %d %q", code, session)
	}
	if code, body := h.raw(t, session, "GET", "/api/memory", nil); code != 200 || !strings.Contains(body, h.anaMark) {
		t.Fatalf("the session is not Ana's: %d %.200s", code, body)
	}
	if code, again := h.open(t, link); code != 401 || again != "" {
		t.Fatalf("a spent invite signed in again: %d", code)
	}
	// The phone app opens its saved link again at every start; where its
	// session still works, that just goes on to the app.
	if code, _ := h.open(t, link, &http.Cookie{Name: "pimpo_session", Value: session}); code != 303 {
		t.Fatalf("reopening with the session: %d", code)
	}
	if _, body := h.raw(t, session, "GET", "/api/events?types=device.added", nil); !strings.Contains(body, "Tablet da Ana") {
		t.Fatalf("Ana does not see the device added to her account: %.300s", body)
	}
	if _, body := h.raw(t, "tok", "GET", "/api/events?types=device.added", nil); strings.Contains(body, "Tablet da Ana") {
		t.Fatal("the owner sees Ana's activity")
	}
}

// An invite nobody opens in time opens nothing.
func TestAnUnusedInviteExpires(t *testing.T) {
	h := newHouse(t)
	link, _ := h.invite(t, h.anaID, "Tablet")
	ctx := context.Background()
	list := h.devices(ctx)
	for i := range list {
		if list[i].Invite {
			list[i].Expires = time.Now().Add(-time.Second)
		}
	}
	h.saveDevices(ctx, list)
	if code, session := h.open(t, link); code != 401 || session != "" {
		t.Fatalf("an expired invite signed in: %d", code)
	}
	for _, d := range h.devices(ctx) {
		if d.Invite {
			t.Fatal("the expired invite was kept")
		}
	}
}

// Each person sees the devices and sessions that open their account, and
// only theirs, and can sign any of them out.
func TestPeopleSeeAndRevokeTheirOwnDevices(t *testing.T) {
	h := newHouse(t)
	ownerPhone := pairPhone(t, h.testApp)
	var ownerID string
	for _, d := range h.devices(context.Background()) {
		if d.Person == "" {
			ownerID = d.ID
		}
	}
	list := func(token string) []myDevice {
		_, body := h.raw(t, token, "GET", "/api/me/devices", nil)
		var out []myDevice
		json.Unmarshal([]byte(body), &out)
		return out
	}
	anas := list(h.ana)
	if len(anas) != 1 || anas[0].Name != "Celular da Ana" || !anas[0].Current {
		t.Fatalf("Ana's devices: %+v", anas)
	}
	if mine := list(ownerPhone); len(mine) != 1 || mine[0].ID != ownerID {
		t.Fatalf("the owner's devices: %+v", mine)
	}
	if code, _ := h.raw(t, h.ana, "DELETE", "/api/me/devices/"+ownerID, nil); code != 404 {
		t.Fatalf("Ana signed out the owner's phone: %d", code)
	}
	if code, _ := h.raw(t, "tok", "DELETE", "/api/me/devices/"+anas[0].ID, nil); code != 404 {
		t.Fatalf("the owner's own list reached Ana's device: %d", code)
	}
	if code, _ := h.raw(t, h.ana, "DELETE", "/api/me/devices/"+anas[0].ID, nil); code != 200 {
		t.Fatalf("Ana could not sign out her device: %d", code)
	}
	if code, _ := h.raw(t, h.ana, "GET", "/api/memory", nil); code != 401 {
		t.Fatal("a signed-out device still opens Pimpo")
	}
}

// A device's id, shown in lists and events, gives nothing of its token.
func TestDeviceIDsAreNotPartOfTheirTokens(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	token := pairPhone(t, ta)
	for _, d := range ta.devices(context.Background()) {
		if strings.Contains(token, d.ID) {
			t.Fatalf("id %s is part of the token", d.ID)
		}
	}
}

// A phone signed out for being unused cannot report with its key either,
// and what a member's phone changes is recorded as theirs.
func TestIdlePhonesCannotReport(t *testing.T) {
	h := newHouse(t)
	h.raw(t, h.ana, "POST", "/api/phone/shares", js(map[string]any{"shares": []string{"shortcuts"}}))
	_, body := h.raw(t, h.ana, "POST", "/api/phone/key", nil)
	key := field(body, "key")
	evs, _ := h.Events.List(context.Background(), event.Query{Types: []string{"phone.shares", "phone.key"}})
	if len(evs) != 2 {
		t.Fatalf("events %+v", evs)
	}
	for _, e := range evs {
		if e.Actor != "human:"+h.anaID {
			t.Errorf("%s recorded as %s, not Ana", e.Type, e.Actor)
		}
	}
	if code, _ := h.raw(t, key, "POST", "/api/phone/shortcut", js(map[string]string{"name": "x"})); code != 200 {
		t.Fatalf("the key does not report: %d", code)
	}
	ctx := context.Background()
	list := h.devices(ctx)
	for i := range list {
		list[i].LastSeen = time.Now().Add(-deviceIdle - time.Hour)
	}
	h.saveDevices(ctx, list)
	if code, _ := h.raw(t, key, "POST", "/api/phone/shortcut", js(map[string]string{"name": "x"})); code != 401 {
		t.Fatalf("an idle phone's key still reports: %d", code)
	}
}

// The app on the phone reports with its cookie; another page cannot make
// the browser report for it.
func TestPhoneReportsNeedPimposOwnPage(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	phone := pairPhone(t, ta)
	ta.as(t, phone, "POST", "/api/phone/shares", "application/json", js(map[string]any{"shares": []string{"shortcuts"}}))
	report := func(origin string) int {
		req, _ := http.NewRequest("POST", ta.srv.URL+"/api/phone/shortcut", strings.NewReader(`{"name":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		req.AddCookie(&http.Cookie{Name: "pimpo_session", Value: phone})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := report("http://127.0.0.1:1"); code != 403 {
		t.Fatalf("another page reported for the phone: %d", code)
	}
	if code := report(ta.srv.URL); code != 200 {
		t.Fatalf("the phone's own page could not report: %d", code)
	}
}
