package chatlink

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestWhatsAppPersonalBridge(t *testing.T) {
	var sent map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "k" && r.URL.Query().Get("x-api-key") != "k" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/api/sessions/default":
			json.NewEncoder(w).Encode(map[string]string{"status": "WORKING"})
		case "/api/sendText":
			json.NewDecoder(r.Body).Decode(&sent)
			w.WriteHeader(201)
		case "/ws":
			c, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer c.CloseNow()
			for _, ev := range []string{
				`{"event":"message","payload":{"from":"120363@g.us","body":"num grupo"}}`,
				`{"event":"message","payload":{"from":"5511999990000@c.us","fromMe":true,"body":"eu mesmo"}}`,
				`{"event":"session.status","payload":{}}`,
				`{"event":"message","payload":{"from":"5511999990000@c.us","body":"oi Pimpo"}}`,
			} {
				c.Write(r.Context(), websocket.MessageText, []byte(ev))
			}
			<-r.Context().Done()
		}
	}))
	defer srv.Close()
	w := &WhatsAppPersonal{URL: srv.URL, APIKey: "k"}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := w.Check(ctx); err != nil {
		t.Fatal(err)
	}
	got := make(chan Inbound, 4)
	go w.Run(ctx, func(in Inbound) { got <- in })
	select {
	case in := <-got:
		if in.From != "5511999990000@c.us" || in.Text != "oi Pimpo" {
			t.Fatalf("%+v", in)
		}
	case <-ctx.Done():
		t.Fatal("no message")
	}
	if err := w.Send(ctx, "5511999990000@c.us", "olá"); err != nil || sent["chatId"] != "5511999990000@c.us" || sent["session"] != "default" {
		t.Fatalf("%v %v", err, sent)
	}
	if !w.NoApprovals() {
		t.Fatal("the unofficial WhatsApp may approve")
	}
}
