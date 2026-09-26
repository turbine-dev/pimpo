package external

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/denerFernandes/pimpo/internal/capability"
)

// TestMain doubles as a fake connector when asked, so tests exercise a
// real child process speaking MCP over stdio.
func TestMain(m *testing.M) {
	if mode := os.Getenv("FAKE_CONNECTOR"); mode != "" {
		serve(mode)
		return
	}
	os.Exit(m.Run())
}

func serve(mode string) {
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		var req struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		json.Unmarshal(in.Bytes(), &req)
		if req.ID == nil {
			continue
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "serverInfo": map[string]string{"name": "fake"}}
		case "tools/list":
			tools := []map[string]string{{"name": "tides_today"}, {"name": "tides_alert"}}
			if mode == "extra" {
				tools = append(tools, map[string]string{"name": "tides_exfiltrate"})
			}
			result = map[string]any{"tools": tools}
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			json.Unmarshal(req.Params, &p)
			if mode == "hang" {
				time.Sleep(time.Hour)
			}
			body, _ := json.Marshal([]map[string]any{{"port": p.Arguments["port"], "high": "10:42", "secret_seen": os.Getenv("TIDES_KEY"), "home_seen": os.Getenv("PIMPO_LEAK")}})
			result = map[string]any{"content": []map[string]string{{"type": "text", "text": string(body)}}}
		}
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
		fmt.Println(string(b))
	}
}

func manifest(t *testing.T, mode string, edit func(m map[string]any)) string {
	dir := t.TempDir()
	m := map[string]any{
		"name": "tides", "description": "Tide tables", "command": os.Args[0], "args": []string{"-test.run=^$"},
		"env": []string{"TIDES_KEY", "FAKE_CONNECTOR"},
		"capabilities": []map[string]any{
			{"name": "tides.today", "risk": "read", "signature": "tides.today({port})", "returns": "[{port, high}]", "schema": map[string]any{"type": "object"}},
			{"name": "tides.alert", "risk": "notify", "signature": "tides.alert({text})", "returns": "{ok}"},
		},
		"contract": []map[string]any{{"capability": "tides.today", "args": map[string]any{"port": "Santos"}, "keys": []string{"port", "high"}}},
	}
	if edit != nil {
		edit(m)
	}
	b, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(dir, "connector.json"), b, 0o644)
	return dir
}

// as runs the fake connector in a mode; the connector only sees env vars it
// declares, so the mode travels as one of its secrets.
func as(mode string) Secrets {
	return func(_ context.Context, name string) (string, error) {
		if name == "FAKE_CONNECTOR" {
			return mode, nil
		}
		return "k-" + name, nil
	}
}

func TestConnectorRunsAndPassesItsContract(t *testing.T) {
	t.Setenv("PIMPO_LEAK", "should-not-pass")
	m, err := Load(manifest(t, "ok", nil))
	if err != nil {
		t.Fatal(err)
	}
	secrets := as("ok")
	if p := Check(context.Background(), m, secrets); len(p) != 0 {
		t.Fatalf("contract: %v", p)
	}
	m.Register()
	if capability.Catalog["tides.alert"].Risk != capability.Notify {
		t.Fatal("capability not registered with its risk")
	}
	c := &Connector{Manifest: m, Secrets: secrets}
	defer c.Close()
	out, err := c.Call(context.Background(), "tides.today", "", map[string]any{"port": "Santos"})
	row := out.([]any)[0].(map[string]any)
	if err != nil || row["port"] != "Santos" || row["secret_seen"] != "k-TIDES_KEY" || row["home_seen"] != "" {
		t.Fatalf("%v %v", out, err)
	}
}

func TestConnectorsAreHeldToTheirManifest(t *testing.T) {
	ctx := context.Background()
	m, _ := Load(manifest(t, "extra", nil))
	if p := Check(ctx, m, as("extra")); len(p) == 0 || !strings.Contains(p[0], "offers tools") {
		t.Fatalf("an undeclared tool went unnoticed: %v", p)
	}
	m, _ = Load(manifest(t, "hang", nil))
	c := &Connector{Manifest: m, Secrets: as("hang"), Timeout: 300_000_000}
	if _, err := c.Call(ctx, "tides.today", "", map[string]any{}); err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("hang: %v", err)
	}
	bad := map[string]func(map[string]any){
		"prefix":   func(m map[string]any) { m["capabilities"].([]map[string]any)[0]["name"] = "gmail.search" },
		"risk":     func(m map[string]any) { m["capabilities"].([]map[string]any)[0]["risk"] = "harmless" },
		"contract": func(m map[string]any) { m["contract"] = []any{} },
		"name":     func(m map[string]any) { m["name"] = "../x" },
		"env":      func(m map[string]any) { m["env"] = []string{"PATH=/evil"} },
	}
	for what, edit := range bad {
		if _, err := Load(manifest(t, "ok", edit)); err == nil {
			t.Errorf("accepted a manifest with a bad %s", what)
		}
	}
	m, _ = Load(manifest(t, "ok", func(m map[string]any) {
		m["contract"] = []map[string]any{{"capability": "tides.today", "args": map[string]any{}, "keys": []string{"low"}}}
	}))
	if p := Check(ctx, m, as("ok")); len(p) != 1 || !strings.Contains(p[0], `no "low"`) {
		t.Fatalf("missing key: %v", p)
	}
}
