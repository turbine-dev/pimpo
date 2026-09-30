package whatsapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSend(t *testing.T) {
	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/123/messages" || r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"message":"bad request"}}`))
			return
		}
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		got = append(got, m)
		w.Write([]byte(`{"messages":[{"id":"wamid.1"}]}`))
	}))
	defer srv.Close()
	c := Client{Token: "tok", PhoneID: "123", BaseURL: srv.URL}
	if err := c.Send(context.Background(), "+55 (11) 99999-0000", "oi"); err != nil {
		t.Fatal(err)
	}
	buttons := []Button{{"approve:1", "Permitir"}, {"always:1", "Sempre, sem perguntar de novo"}, {"deny:1", "Negar"}, {"batch:1", "Todos"}}
	if err := c.Send(context.Background(), "5511999990000", "Posso?", buttons...); err != nil {
		t.Fatal(err)
	}
	if got[0]["to"] != "5511999990000" || got[0]["type"] != "text" {
		t.Fatalf("text %v", got[0])
	}
	bs := got[1]["interactive"].(map[string]any)["action"].(map[string]any)["buttons"].([]any)
	title := bs[1].(map[string]any)["reply"].(map[string]any)["title"].(string)
	if len(bs) != 3 || len([]rune(title)) > 20 {
		t.Fatalf("buttons %v", bs)
	}
	if err := (Client{Token: "x", PhoneID: "123", BaseURL: srv.URL}).Send(context.Background(), "1", "oi"); err == nil || !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("error %v", err)
	}
}

func TestWebhook(t *testing.T) {
	body := []byte(`{"object":"whatsapp_business_account","entry":[{"changes":[{"value":{
	  "contacts":[{"wa_id":"5511999990000","profile":{"name":"Dener"}}],
	  "messages":[{"id":"wamid.A1","timestamp":"1790000000","from":"5511999990000","type":"text","text":{"body":"resuma meus e-mails"}},
	              {"from":"5511999990000","type":"interactive","interactive":{"type":"button_reply","button_reply":{"id":"approve:ab12","title":"Permitir"}}},
	              {"from":"5511999990000","type":"image","image":{"id":"x"}}]}}]}]}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if err := Verify(body, sig, "secret"); err != nil {
		t.Fatal(err)
	}
	if Verify(body, sig, "other") == nil || Verify(append(body, ' '), sig, "secret") == nil || Verify(body, "", "secret") == nil || Verify(body, sig, "") == nil {
		t.Fatal("accepted a bad signature")
	}
	in, err := Parse(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(in) != 2 || in[0].Text != "resuma meus e-mails" || in[0].Name != "Dener" || in[1].Button != "approve:ab12" || in[0].ID != "wamid.A1" || in[0].Time.Unix() != 1790000000 {
		t.Fatalf("%+v", in)
	}
}
