package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func call(t *testing.T, url, body string) map[string]any {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func TestToolsListAndCall(t *testing.T) {
	s := &Server{Name: "zodim", Tools: []Tool{
		{Name: "echo", Description: "echo", InputSchema: json.RawMessage(`{"type":"object"}`), Handle: func(_ context.Context, a json.RawMessage) (any, error) { return json.RawMessage(a), nil }},
		{Name: "fail", InputSchema: json.RawMessage(`{"type":"object"}`), Handle: func(context.Context, json.RawMessage) (any, error) { return nil, errors.New("nope") }},
	}}
	srv := httptest.NewServer(s)
	defer srv.Close()
	init := call(t, srv.URL, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if init["result"].(map[string]any)["serverInfo"].(map[string]any)["name"] != "zodim" {
		t.Fatalf("initialize %+v", init)
	}
	list := call(t, srv.URL, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if len(list["result"].(map[string]any)["tools"].([]any)) != 2 {
		t.Fatalf("list %+v", list)
	}
	out := call(t, srv.URL, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"x":1}}}`)
	text := out["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"]
	if text != `{"x":1}` {
		t.Fatalf("call %+v", out)
	}
	bad := call(t, srv.URL, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"fail","arguments":{}}}`)
	if bad["result"].(map[string]any)["isError"] != true {
		t.Fatalf("tool error %+v", bad)
	}
	unknown := call(t, srv.URL, `{"jsonrpc":"2.0","id":5,"method":"nope"}`)
	if !strings.Contains(unknown["error"].(map[string]any)["message"].(string), "not found") {
		t.Fatalf("unknown method %+v", unknown)
	}
	resp, _ := http.Post(srv.URL, "application/json", bytes.NewBufferString(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if resp.StatusCode != 202 {
		t.Fatalf("notification got %d", resp.StatusCode)
	}
}
