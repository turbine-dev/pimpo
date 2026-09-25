package chatlink

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type recorder struct {
	mu   sync.Mutex
	hits []string
}

func (r *recorder) add(s string) { r.mu.Lock(); r.hits = append(r.hits, s); r.mu.Unlock() }
func (r *recorder) all() string  { r.mu.Lock(); defer r.mu.Unlock(); return strings.Join(r.hits, "|") }

func TestDiscordDirectMessages(t *testing.T) {
	rec := &recorder{}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws := strings.HasPrefix(r.URL.Path, "/ws")
		if r.Header.Get("Authorization") != "Bot tk" && !ws {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.URL.Path == "/gateway/bot":
			fmt.Fprintf(w, `{"url":"ws%s/ws"}`, strings.TrimPrefix(srv.URL, "http"))
		case r.URL.Path == "/users/@me":
			io.WriteString(w, `{"id":"bot"}`)
		case r.URL.Path == "/users/@me/channels":
			io.WriteString(w, `{"id":"dm-2"}`)
		case strings.HasPrefix(r.URL.Path, "/channels/"):
			var b map[string]string
			json.NewDecoder(r.Body).Decode(&b)
			rec.add(r.URL.Path + "=" + b["content"])
			io.WriteString(w, `{}`)
		case ws:
			c, _ := websocket.Accept(w, r, nil)
			defer c.CloseNow()
			ctx := r.Context()
			wsjson.Write(ctx, c, map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": 1000}})
			var id struct {
				Op int `json:"op"`
				D  struct {
					Token   string `json:"token"`
					Intents int    `json:"intents"`
				} `json:"d"`
			}
			wsjson.Read(ctx, c, &id)
			rec.add(fmt.Sprintf("identify:%d:%s:%d", id.Op, id.D.Token, id.D.Intents))
			s := 1
			for _, d := range []string{
				`{"channel_id":"g1","guild_id":"x","content":"in a server","author":{"id":"u1"}}`,
				`{"channel_id":"dm-1","content":"sou um bot","author":{"id":"b","bot":true}}`,
				`{"channel_id":"dm-1","content":"oi zodim","author":{"id":"u1"}}`,
			} {
				wsjson.Write(ctx, c, map[string]any{"op": 0, "t": "MESSAGE_CREATE", "s": s, "d": json.RawMessage(d)})
				s++
			}
			wsjson.Write(ctx, c, map[string]any{"op": 7})
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer srv.Close()
	d := &Discord{Token: "tk", API: srv.URL}
	if err := d.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	var got []Inbound
	err := d.Run(context.Background(), func(in Inbound) { got = append(got, in) })
	if err == nil || len(got) != 1 || got[0] != (Inbound{From: "u1", Chat: "dm-1", Text: "oi zodim"}) {
		t.Fatalf("%v %+v", err, got)
	}
	d.Send(context.Background(), "u1", "olá")
	d.Send(context.Background(), "u2", strings.Repeat("a", 2500))
	want := fmt.Sprintf("identify:2:tk:%d|/channels/dm-1/messages=olá|/channels/dm-2/messages=%s|/channels/dm-2/messages=%s", directMessages, strings.Repeat("a", 1900), strings.Repeat("a", 600))
	if rec.all() != want {
		t.Fatalf("%s", rec.all())
	}
	if err := (&Discord{Token: "bad", API: srv.URL}).Check(context.Background()); err == nil {
		t.Fatal("accepted a bad token")
	}
}

func TestSlackSocketMode(t *testing.T) {
	rec := &recorder{}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/apps.connections.open":
			if auth != "Bearer xapp" {
				io.WriteString(w, `{"ok":false,"error":"invalid_auth"}`)
				return
			}
			fmt.Fprintf(w, `{"ok":true,"url":"ws%s/ws"}`, strings.TrimPrefix(srv.URL, "http"))
		case "/auth.test":
			fmt.Fprintf(w, `{"ok":%v}`, auth == "Bearer xoxb")
		case "/conversations.open":
			io.WriteString(w, `{"ok":true,"channel":{"id":"D2"}}`)
		case "/chat.postMessage":
			var b map[string]string
			json.NewDecoder(r.Body).Decode(&b)
			rec.add(b["channel"] + "=" + b["text"])
			io.WriteString(w, `{"ok":true}`)
		case "/ws":
			c, _ := websocket.Accept(w, r, nil)
			defer c.CloseNow()
			ctx := r.Context()
			wsjson.Write(ctx, c, map[string]any{"type": "hello"})
			for i, e := range []string{
				`{"type":"message","channel_type":"channel","channel":"C1","user":"U1","text":"no canal"}`,
				`{"type":"message","channel_type":"im","channel":"D1","user":"U1","text":"editada","subtype":"message_changed"}`,
				`{"type":"message","channel_type":"im","channel":"D1","user":"U1","text":"oi zodim"}`,
			} {
				wsjson.Write(ctx, c, map[string]any{"type": "events_api", "envelope_id": fmt.Sprint("e", i), "payload": map[string]any{"event": json.RawMessage(e)}})
				var ack map[string]string
				wsjson.Read(ctx, c, &ack)
				rec.add("ack:" + ack["envelope_id"])
			}
			wsjson.Write(ctx, c, map[string]any{"type": "disconnect"})
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer srv.Close()
	s := &Slack{BotToken: "xoxb", AppToken: "xapp", API: srv.URL}
	if err := s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	var got []Inbound
	s.Run(context.Background(), func(in Inbound) { got = append(got, in) })
	if len(got) != 1 || got[0].Text != "oi zodim" || got[0].From != "U1" {
		t.Fatalf("%+v", got)
	}
	s.Send(context.Background(), "U1", "olá")
	s.Send(context.Background(), "U2", "oi")
	if rec.all() != "ack:e0|ack:e1|ack:e2|D1=olá|D2=oi" {
		t.Fatal(rec.all())
	}
	if err := (&Slack{BotToken: "xoxb", AppToken: "bad", API: srv.URL}).Check(context.Background()); err == nil || !strings.Contains(err.Error(), "app token") {
		t.Fatalf("%v", err)
	}
}

func TestSignalThroughSignalCLI(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/events":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event:receive\ndata:{\"jsonrpc\":\"2.0\",\"method\":\"receive\",\"params\":{\"envelope\":{\"sourceNumber\":\"+5511999\",\"typingMessage\":{}}}}\n\n")
			fmt.Fprint(w, "event:receive\ndata:{\"jsonrpc\":\"2.0\",\"method\":\"receive\",\"params\":{\"envelope\":{\"sourceNumber\":\"+5511999\",\"dataMessage\":{\"message\":\"oi zodim\"}}}}\n\n")
		case "/api/v1/rpc":
			var req struct {
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			b, _ := json.Marshal(req.Params)
			rec.add(req.Method + string(b))
			io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
		}
	}))
	defer srv.Close()
	s := &Signal{URL: srv.URL, Account: "+5511000"}
	var got []Inbound
	s.Run(context.Background(), func(in Inbound) { got = append(got, in) })
	if len(got) != 1 || got[0] != (Inbound{From: "+5511999", Text: "oi zodim"}) {
		t.Fatalf("%+v", got)
	}
	if err := s.Send(context.Background(), "+5511999", "olá"); err != nil {
		t.Fatal(err)
	}
	if rec.all() != `send{"account":"+5511000","message":"olá","recipient":["+5511999"]}` {
		t.Fatal(rec.all())
	}
	if err := (&Signal{URL: "http://127.0.0.1:1"}).Check(context.Background()); err == nil || !strings.Contains(err.Error(), "daemon --http") {
		t.Fatalf("%v", err)
	}
}
