package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/turbine-dev/pimpo/internal/chatlink"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/people"
)

func TestMatchOption(t *testing.T) {
	opts := []string{"Sim", "Não", "Talvez mais tarde"}
	for text, want := range map[string]int{"sim": 0, "SIM!": 0, "nao": 1, " Não. ": 1, "2": 1, "t": 2, "talvez  MAIS": 2, "3": 2} {
		if got, ok := matchOption(text, opts); !ok || got != want {
			t.Errorf("%q: %d %v, want %d", text, got, ok, want)
		}
	}
	for _, text := range []string{"", "4", "0", "talvez sim", "yes", "?"} {
		if got, ok := matchOption(text, opts); ok {
			t.Errorf("%q matched %d", text, got)
		}
	}
	if _, ok := matchOption("ho", []string{"Hoje", "Hora"}); ok {
		t.Error("an ambiguous start matched")
	}
	if got, ok := matchOption("hoj", []string{"Hoje", "Hora"}); !ok || got != 0 {
		t.Errorf("hoj: %d %v", got, ok)
	}
	// An option that reads as a number is taken as that option.
	if got, ok := matchOption("20", []string{"10", "20"}); !ok || got != 1 {
		t.Errorf("20: %d %v", got, ok)
	}
	if got, ok := matchOption("1", []string{"10", "20"}); !ok || got != 0 {
		t.Errorf("1: %d %v", got, ok)
	}
}

// ask puts a question to the owner as a routine would, and returns its id.
func ask(t *testing.T, ta *testApp, question string, options ...string) string {
	t.Helper()
	ctx := people.With(context.Background(), people.OwnerID)
	out, err := (askCap{ta.App}).Call(ctx, "ask.owner", "", map[string]any{"question": question, "options": options})
	if err != nil {
		t.Fatal(err)
	}
	return out.(map[string]any)["asked"].(string)
}

func pending(ta *testApp, id string) bool {
	for _, q := range ta.questions(context.Background()) {
		if q.ID == id {
			return true
		}
	}
	return false
}

// Typed in the app, an answer that is none of the options gets them again
// and answers nothing; one that matches answers. Only the person asked
// may answer.
func TestTypedAnswerInTheApp(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	id := ask(t, ta, "Treinou hoje?", "Sim", "Não")
	code, out := ta.do(t, "POST", "/api/questions/"+id+"/answer", map[string]any{"text": "talvez"})
	if code != 422 || !strings.Contains(fmt.Sprint(out["error"]), "1 = Sim · 2 = Não") || !pending(ta, id) {
		t.Fatalf("%d %v", code, out)
	}
	ana := people.With(context.Background(), "ana")
	if _, err := ta.answerText(ana, id, "sim"); err == nil || !pending(ta, id) {
		t.Fatal("someone else answered the owner's question")
	}
	code, out = ta.do(t, "POST", "/api/questions/"+id+"/answer", map[string]any{"text": "nao"})
	if code != 200 || !strings.Contains(fmt.Sprint(out["text"]), "Não") || pending(ta, id) {
		t.Fatalf("%d %v", code, out)
	}
	if code, _ := ta.do(t, "POST", "/api/questions/"+id+"/answer", map[string]any{"text": "sim"}); code != 404 {
		t.Fatalf("answered twice: %d", code)
	}
	// The buttons keep answering by position.
	id = ask(t, ta, "Treinou hoje?", "Sim", "Não")
	if code, out := ta.do(t, "POST", "/api/questions/"+id+"/answer", map[string]any{"index": 0}); code != 200 || !strings.Contains(fmt.Sprint(out["text"]), "Sim") {
		t.Fatalf("%d %v", code, out)
	}
}

// On Telegram a reply to the question comes with its buttons; the handler
// checks the words against the options, and other notices are not its.
func TestTelegramReplyInWords(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := people.With(context.Background(), people.OwnerID)
	id := ask(t, ta, "Que treino?", "Corrida", "Bike", "Natação", "Descanso")
	choices := []explore.Action{{Label: "Corrida", Data: "answer:" + id + ".0"}, {Label: "Natação", Data: "answer:" + id + ".2"}}
	h := handler{ta.App}
	if out, ok := h.Reply(ctx, choices, "yoga"); !ok || !strings.Contains(out, "4 = Descanso") || !pending(ta, id) {
		t.Fatalf("%q %v", out, ok)
	}
	if out, ok := h.Reply(ctx, choices, "nata"); !ok || !strings.Contains(out, "Natação") || pending(ta, id) {
		t.Fatalf("%q %v", out, ok)
	}
	if out, ok := h.Reply(ctx, choices, "bike"); !ok || !strings.Contains(out, "já foi respondida") {
		t.Fatalf("answered twice: %q %v", out, ok)
	}
	if _, ok := h.Reply(ctx, []explore.Action{{Label: "Sim", Data: "compile:e1"}}, "sim"); ok {
		t.Fatal("a reply to a notice that is not a question was taken")
	}
}

// On Signal, Discord and Slack a reply in words to the question is
// checked; words sent without replying answer only when they are exactly
// an option of the question just asked, and nothing was said since.
func TestLinkAnswersInWords(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	l := &replyLink{}
	run := &linkRun{link: l, cancel: func() {}}
	ta.links = map[string]*linkRun{"signal": run}
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+551199", Text: "pimpo " + ta.Channel.PairingCode()})
	say := func(replyTo, text string) string {
		ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+551199", Text: text, ReplyTo: replyTo})
		return l.last()
	}

	id := ask(t, ta, "Treinou hoje?", "Sim", "Não")
	waitFor(t, func() bool { return strings.Contains(l.last(), "Treinou hoje?") })
	sent := fmt.Sprintf("m%d", l.n)
	if out := say(sent, "mais ou menos"); !strings.Contains(out, "1 = Sim · 2 = Não") || !pending(ta, id) {
		t.Fatalf("%q", out)
	}
	if out := say(sent, "não!"); !strings.Contains(out, "Anotado: Não") || pending(ta, id) {
		t.Fatalf("%q", out)
	}

	id = ask(t, ta, "Treinou hoje?", "Sim", "Não")
	waitFor(t, func() bool { return strings.Contains(l.last(), "Treinou hoje?") })
	if out := say("", "Sim"); !strings.Contains(out, "Anotado: Sim") || pending(ta, id) {
		t.Fatalf("a bare answer: %q", out)
	}

	// After a message on the channel, the same word is an ordinary one.
	id = ask(t, ta, "Treinou hoje?", "Sim", "Não")
	waitFor(t, func() bool { return strings.Contains(l.last(), "Treinou hoje?") })
	say("", "me lembra de beber água")
	ta.Explore.Wait()
	say("", "sim")
	ta.Explore.Wait()
	if !pending(ta, id) {
		t.Fatal("a word in a conversation answered the question")
	}
}

// On WhatsApp a question with more options than three buttons comes as a
// list; a tapped row answers, and a reply in words is checked.
func TestWhatsAppQuestionAsAList(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	var mu sync.Mutex
	var sent []map[string]any
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		sent = append(sent, m)
		n := len(sent)
		mu.Unlock()
		fmt.Fprintf(w, `{"messages":[{"id":"wamid.out%d"}]}`, n)
	}))
	defer graph.Close()
	ta.WhatsAppAPI = graph.URL
	ta.do(t, "PUT", "/api/connections/whatsapp", map[string]string{"token": "tok", "phone_id": "99", "app_secret": "shh"})
	ta.Events.Put(context.Background(), "whatsapp.owner", "5511900000000")
	last := func() map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return sent[len(sent)-1]
	}
	lastText := func() string {
		m := last()
		if tx, ok := m["text"].(map[string]any); ok {
			return fmt.Sprint(tx["body"])
		}
		return ""
	}
	post := func(msg map[string]any) {
		msg["from"], msg["id"] = "5511900000000", waMessageID()
		body, _ := json.Marshal(map[string]any{"entry": []any{map[string]any{"changes": []any{map[string]any{"value": map[string]any{"messages": []any{msg}}}}}}})
		req, _ := http.NewRequest("POST", ta.srv.URL+"/webhook/whatsapp", bytes.NewReader(body))
		mac := hmac.New(sha256.New, []byte("shh"))
		mac.Write(body)
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	id := ask(t, ta, "Que treino?", "Corrida", "Bike", "Natação", "Descanso")
	in := last()["interactive"].(map[string]any)
	rows := in["action"].(map[string]any)["sections"].([]any)[0].(map[string]any)["rows"].([]any)
	if in["type"] != "list" || len(rows) != 4 || in["action"].(map[string]any)["button"] != "Escolher" {
		t.Fatalf("not a list: %v", last())
	}
	post(map[string]any{"type": "interactive", "interactive": map[string]any{"type": "list_reply", "list_reply": map[string]string{"id": "answer:" + id + ".3"}}})
	waitFor(t, func() bool { return lastText() != "" })
	if !strings.Contains(lastText(), "Anotado: Descanso") || pending(ta, id) {
		t.Fatalf("a tapped row: %v", last())
	}

	id = ask(t, ta, "Treinou hoje?", "Sim", "Não")
	if in := last()["interactive"].(map[string]any); in["type"] != "button" {
		t.Fatalf("two options are buttons: %v", in)
	}
	mu.Lock()
	question := fmt.Sprintf("wamid.out%d", len(sent))
	mu.Unlock()
	reply := func(text string) {
		post(map[string]any{"type": "text", "text": map[string]string{"body": text}, "context": map[string]string{"id": question}})
	}
	reply("quase")
	waitFor(t, func() bool { return lastText() != "" })
	if !strings.Contains(lastText(), "1 = Sim · 2 = Não") || !pending(ta, id) {
		t.Fatalf("a wrong answer: %q", lastText())
	}
	reply("s")
	waitFor(t, func() bool { return strings.Contains(lastText(), "Anotado") })
	if !strings.Contains(lastText(), "Anotado: Sim") || pending(ta, id) {
		t.Fatalf("a typed answer: %q", lastText())
	}
}
