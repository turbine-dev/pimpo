package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeAPI struct {
	mu    sync.Mutex
	calls []string
	sent  []map[string]any
	polls int
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	f.calls = append(f.calls, method)
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	switch method {
	case "sendMessage":
		f.sent = append(f.sent, body)
		w.Write([]byte(`{"ok":true,"result":{"message_id":7,"text":"x","chat":{"id":42}}}`))
	case "getUpdates":
		f.polls++
		if f.polls == 1 {
			w.Write([]byte(`{"ok":true,"result":[{"update_id":10,"message":{"message_id":1,"text":"/start 123456","chat":{"id":42},"from":{"id":42,"first_name":"Dener"}}},{"update_id":11,"callback_query":{"id":"cb1","data":"approve:9","message":{"message_id":7,"chat":{"id":42}},"from":{"id":42}}}]}`))
			return
		}
		w.Write([]byte(`{"ok":true,"result":[]}`))
	case "getMe":
		w.Write([]byte(`{"ok":false,"description":"Unauthorized"}`))
	default:
		w.Write([]byte(`{"ok":true,"result":true}`))
	}
}

func TestSendWithButtonsAndPoll(t *testing.T) {
	api := &fakeAPI{}
	srv := httptest.NewServer(api)
	defer srv.Close()
	bot := Bot{Token: "secret-token", BaseURL: srv.URL}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m, err := bot.Send(ctx, 42, "Apagar 212 e-mails?", []Button{{Text: "Permitir", Data: "approve:9"}, {Text: "Negar", Data: "deny:9"}})
	if err != nil || m.ID != 7 {
		t.Fatalf("send %+v %v", m, err)
	}
	kb := api.sent[0]["reply_markup"].(map[string]any)["inline_keyboard"].([]any)
	if len(kb[0].([]any)) != 2 {
		t.Fatalf("keyboard %+v", kb)
	}

	var got []Update
	done := make(chan struct{})
	go bot.Poll(ctx, 0, func(u Update) {
		got = append(got, u)
		if len(got) == 2 {
			close(done)
		}
	})
	<-done
	cancel()
	if got[0].Message.Text != "/start 123456" || got[1].Callback.Data != "approve:9" {
		t.Fatalf("updates %+v", got)
	}
}

func TestErrorsNeverContainTheToken(t *testing.T) {
	api := &fakeAPI{}
	srv := httptest.NewServer(api)
	defer srv.Close()
	bot := Bot{Token: "secret-token", BaseURL: srv.URL}
	_, err := bot.Me(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Unauthorized") {
		t.Fatalf("api error: %v", err)
	}
	srv.Close()
	_, err = bot.Send(context.Background(), 1, "x")
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("network error leaks token or is nil: %v", err)
	}
}

func TestPollReportsHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"ok":false,"description":"Unauthorized"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	var errs int
	b := Bot{Token: "x", BaseURL: srv.URL, Health: func(err error) {
		if err != nil {
			errs++
		}
	}}
	b.Poll(ctx, 0, func(Update) {})
	if errs < 1 {
		t.Fatal("failed polls were not reported")
	}
}

// A reply carries the message it replies to, with that message's buttons.
func TestReplyCarriesTheButtons(t *testing.T) {
	var u Update
	err := json.Unmarshal([]byte(`{"update_id":1,"message":{"message_id":9,"text":"natação","chat":{"id":42,"type":"private"},
	  "reply_to_message":{"message_id":7,"text":"❓ Que treino?","reply_markup":{"inline_keyboard":[[{"text":"Corrida","callback_data":"answer:q.0"},{"text":"Natação","callback_data":"answer:q.1"}]]}}}}`), &u)
	if err != nil {
		t.Fatal(err)
	}
	r := u.Message.ReplyTo
	if r == nil || r.ID != 7 || r.Markup == nil || len(r.Markup.Keyboard[0]) != 2 || r.Markup.Keyboard[0][1].Data != "answer:q.1" {
		t.Fatalf("%+v", r)
	}
}
