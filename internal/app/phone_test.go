package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/runtime"
)

func (ta *testApp) as(t *testing.T, token, method, path, ctype string, body []byte) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, ta.srv.URL+path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func pairPhone(t *testing.T, ta *testApp) string {
	t.Helper()
	_, out := ta.do(t, "POST", "/api/pairing", map[string]string{"base": "https://pimpo.example.com", "device": "iPhone"})
	u, err := url.Parse(out["link"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("token")
}

func js(v any) jsonText { b, _ := json.Marshal(v); return b }

func waitRuns(t *testing.T, ta *testApp, id string, n int) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if runs, _ := ta.Store.Runs(context.Background(), id, 10); len(runs) >= n && runs[0].Outcome != "" {
			if runs[0].Outcome != "ok" {
				t.Fatalf("%s: %s %s", id, runs[0].Outcome, runs[0].Error)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	runs, _ := ta.Store.Runs(context.Background(), id, 10)
	t.Fatalf("%s ran %d times, want %d", id, len(runs), n)
}

func watching(t *testing.T, ta *testApp, id, capName string, args map[string]any) {
	t.Helper()
	ctx := context.Background()
	code := `async function run() { for (const x of event.items) await notify.send({text: JSON.stringify(x)}); }`
	if _, err := ta.Store.SaveRoutine(ctx, id, routine.Routine{Name: id, Code: code, Manifest: runtime.Manifest{
		Capabilities: []string{capName, "notify.send"},
		Watch:        &runtime.Watch{Capability: capName, Args: args, Key: "id"},
	}}, "test", "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := ta.Scheduler.Poll(ctx, id); err != nil {
		t.Fatal(err)
	}
}

// The roadmap's measure for 5.2: a routine runs on arriving home, another
// on a photo of a bill, and neither works unless the phone shares it.
func TestPhoneArrivingHomeAndAPhotoOfABill(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.Home = t.TempDir()
	ta.Channel.ReadPhoto = func(context.Context, []byte) (string, error) {
		return "ENEL Conta de luz Total R$ 132,40 Vencimento 10/10/2026", nil
	}
	phone := pairPhone(t, ta)
	ta.do(t, "POST", "/api/phone/places", Place{Name: "Casa", Lat: -23.5617, Lon: -46.6560})
	watching(t, ta, "chegou-em-casa", "phone.arrivals", map[string]any{"place": "Casa"})
	watching(t, ta, "conta-na-foto", "phone.photos", nil)

	far := js(map[string]float64{"lat": -23.5890, "lon": -46.6340, "accuracy": 20})
	home := js(map[string]float64{"lat": -23.5620, "lon": -46.6562, "accuracy": 20})
	if code, _ := ta.as(t, phone, "POST", "/api/phone/location", "application/json", home); code != 403 {
		t.Fatalf("a phone that does not share its location reported one: %d", code)
	}
	if code, _ := ta.as(t, phone, "POST", "/api/phone/shares", "application/json", js(map[string]any{"shares": []string{"location", "camera", "nonsense"}})); code != 200 {
		t.Fatal(code)
	}
	if code, _ := ta.do(t, "POST", "/api/phone/shares", map[string]any{"shares": []string{"shortcuts"}}); code != 403 {
		t.Fatal("only the phone chooses what it shares")
	}
	ta.as(t, phone, "POST", "/api/phone/location", "application/json", far)
	_, out := ta.as(t, phone, "POST", "/api/phone/location", "application/json", home)
	if !strings.Contains(js(out["changed"]).String(), "arrived Casa") {
		t.Fatalf("no arrival: %v", out)
	}
	waitRuns(t, ta, "chegou-em-casa", 1)
	// Still home: no second arrival.
	ta.as(t, phone, "POST", "/api/phone/location", "application/json", home)
	time.Sleep(200 * time.Millisecond)
	if runs, _ := ta.Store.Runs(context.Background(), "chegou-em-casa", 10); len(runs) != 1 {
		t.Fatalf("ran %d times while staying home", len(runs))
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("photo", "conta.jpg")
	fw.Write(append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F', 0}, make([]byte, 64)...))
	mw.WriteField("caption", "conta de luz")
	mw.Close()
	code, out := ta.as(t, phone, "POST", "/api/phone/photo", mw.FormDataContentType(), body.Bytes())
	if code != 200 || !strings.Contains(out["text"].(string), "132,40") {
		t.Fatalf("%d %v", code, out)
	}
	waitRuns(t, ta, "conta-na-foto", 1)
	items, _ := (phoneCap{ta.App}).Call(context.Background(), "phone.photos", "", map[string]any{})
	if got := js(items).String(); !strings.Contains(got, "Vencimento") || !strings.Contains(got, "conta de luz") {
		t.Fatalf("photos: %s", got)
	}

	// The automation key reports events but cannot open the app.
	_, out = ta.as(t, phone, "POST", "/api/phone/key", "", nil)
	key := out["key"].(string)
	if code, _ := ta.as(t, key, "GET", "/api/routines", "", nil); code != 401 {
		t.Fatalf("the automation key opened the app: %d", code)
	}
	if code, _ := ta.as(t, key, "POST", "/api/phone/shortcut", "application/json", js(map[string]string{"name": "Saí do trabalho"})); code != 403 {
		t.Fatal("a shortcut went through without the phone sharing shortcuts")
	}
	if code, _ := ta.as(t, key, "POST", "/api/phone/left", "application/json", js(map[string]string{"place": "Casa"})); code != 200 {
		t.Fatal(code)
	}
	if code, _ := ta.as(t, "wrong", "POST", "/api/phone/arrived", "application/json", js(map[string]string{"place": "Casa"})); code != 401 {
		t.Fatal("a stranger reported an arrival")
	}
	arr, _ := (phoneCap{ta.App}).Call(context.Background(), "phone.arrivals", "", map[string]any{"place": "casa"})
	if got := js(arr).String(); !strings.Contains(got, `"kind":"left"`) || !strings.Contains(got, `"kind":"arrived"`) {
		t.Fatalf("arrivals: %s", got)
	}
}

type jsonText []byte

func (j jsonText) String() string { return string(j) }
