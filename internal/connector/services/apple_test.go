//go:build darwin

package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// Calls reach the script as one JSON argument, with the owner's defaults
// filled in, and changes say how to undo themselves.
func TestApple(t *testing.T) {
	defer func(old func(context.Context, string) ([]byte, error)) { osascript = old }(osascript)
	var got []map[string]any
	osascript = func(_ context.Context, arg string) ([]byte, error) {
		var in map[string]any
		json.Unmarshal([]byte(arg), &in)
		got = append(got, in)
		switch in["op"] {
		case "reminders.add":
			return []byte(`{"id":"x-apple-reminder://1"}`), nil
		case "notes.append":
			return []byte(`{"id":"note-1","before":"<div>old</div>"}`), nil
		}
		return []byte(`[]`), nil
	}
	ctx := context.Background()
	cfg := cfgMap(map[string]string{"notes_folder": "Casa", "reminders_list": "Pessoal"})
	out, err := callApple(ctx, cfg, "apple.reminders.add", "", map[string]any{"title": "Ligar pra Ana", "due": "2026-09-29T09:00:00-03:00"})
	if err != nil {
		t.Fatal(err)
	}
	if got[0]["op"] != "reminders.add" || got[0]["list"] != "Pessoal" || got[0]["title"] != "Ligar pra Ana" {
		t.Fatalf("sent %v", got[0])
	}
	undo := out.(map[string]any)["undo"].(map[string]any)
	if undo["capability"] != "apple.reminders.delete" || undo["args"].(map[string]any)["id"] != "x-apple-reminder://1" {
		t.Fatalf("undo %v", undo)
	}
	out, _ = callApple(ctx, cfg, "apple.notes.append", "", map[string]any{"note": "Diário", "text": "Hoje <b>foi</b> bom"})
	if got[1]["folder"] != "Casa" {
		t.Fatalf("folder %v", got[1])
	}
	r := out.(map[string]any)
	if _, leaked := r["before"]; leaked || r["undo"].(map[string]any)["args"].(map[string]any)["body"] != "<div>old</div>" {
		t.Fatalf("append result %v", r)
	}
	for name, args := range map[string]map[string]any{
		"apple.reminders.add":    {"title": "x", "due": "amanhã"},
		"apple.notes.append":     {"note": "x"},
		"apple.calendar.events":  {"from": "2026-09-29", "to": "2026-09-30T00:00:00Z"},
		"apple.reminders.delete": {},
	} {
		if _, err := callApple(ctx, cfg, name, "", args); err == nil {
			t.Errorf("%s accepted %v", name, args)
		}
	}
	if !strings.Contains(appleScript, "JSON.parse(argv[0])") {
		t.Fatal("arguments must arrive as data")
	}
}
