package sheets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSheets(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tk" {
			w.WriteHeader(401)
			return
		}
		got = append(got, r.Method+" "+r.URL.EscapedPath()+"?"+r.URL.RawQuery)
		switch {
		case strings.HasSuffix(r.URL.Path, ":append"):
			var body struct{ Values [][]any }
			json.NewDecoder(r.Body).Decode(&body)
			if len(body.Values) != 1 || body.Values[0][1] != 42.5 {
				w.WriteHeader(400)
				return
			}
			w.Write([]byte(`{"updates":{"updatedRange":"Gastos!A7:C7","updatedRows":1}}`))
		case strings.HasSuffix(r.URL.Path, ":clear"):
			w.Write([]byte(`{}`))
		default:
			w.Write([]byte(`{"range":"Gastos!A1:C2","values":[["Data","Valor","O quê"],["28/09","12,00","Café"]]}`))
		}
	}))
	defer srv.Close()
	s := &Sheets{Token: func(context.Context) (string, error) { return "tk", nil }, Granted: func(context.Context) bool { return true }, API: srv.URL}
	ctx := context.Background()
	url := "https://docs.google.com/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz0123456789/edit#gid=0"
	out, err := s.Call(ctx, "sheets.read", "", map[string]any{"sheet": url, "range": "Gastos!A:C"})
	if err != nil || len(out.(map[string]any)["values"].([][]any)) != 2 {
		t.Fatalf("%v %v", out, err)
	}
	if !strings.Contains(got[0], "/spreadsheets/1AbCdEfGhIjKlMnOpQrStUvWxYz0123456789/values/Gastos%21A:C") {
		t.Fatalf("read %s", got[0])
	}
	out, err = s.Call(ctx, "sheets.append", "", map[string]any{"sheet": url, "range": "Gastos!A:C", "values": []any{[]any{"29/09", 42.5, "Mercado"}}})
	if err != nil {
		t.Fatal(err)
	}
	undo := out.(map[string]any)["undo"].(map[string]any)
	if undo["capability"] != "sheets.clear" || undo["args"].(map[string]any)["range"] != "Gastos!A7:C7" || !strings.Contains(got[1], "valueInputOption=USER_ENTERED") {
		t.Fatalf("%v %s", undo, got[1])
	}
	if _, err := s.Call(ctx, "sheets.clear", "", undo["args"]); err != nil {
		t.Fatal(err)
	}
	s.Granted = func(context.Context) bool { return false }
	if _, err := s.Call(ctx, "sheets.read", "", map[string]any{"sheet": url}); err == nil || !strings.Contains(err.Error(), "reconnect Google") {
		t.Fatalf("not granted: %v", err)
	}
	s.Granted = nil
	if _, err := s.Call(ctx, "sheets.read", "", map[string]any{"sheet": "planilha"}); err == nil {
		t.Fatal("accepted a name for a sheet")
	}
}
