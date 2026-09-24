package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/denerFernandes/vigia/internal/approval"
	"github.com/denerFernandes/vigia/internal/host"
	"github.com/denerFernandes/vigia/internal/llm"
	"github.com/denerFernandes/vigia/internal/memory"
	"github.com/denerFernandes/vigia/internal/people"
)

// callTool calls an MCP tool and returns its text, or the error text.
func callTool(url, tool string, args any) (string, bool) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err.Error(), true
	}
	defer resp.Body.Close()
	var out struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
			IsError bool                    `json:"isError"`
		} `json:"result"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	var text []string
	for _, c := range out.Result.Content {
		text = append(text, c.Text)
	}
	return strings.Join(text, "\n"), out.Result.IsError
}

func message(subject string) []byte {
	return []byte(fmt.Sprintf("From: X <x@y.com>\r\nTo: eu@exemplo.com\r\nSubject: %s\r\nMessage-ID: <%s@y.com>\r\nDate: Mon, 01 Jan 2024 10:00:00 +0000\r\n\r\n%s\r\n", subject, subject, subject))
}

type seenRun struct {
	prompt, mail, memory, note string
	mailErr                    bool
}

// Gate F6: people in one house never reach each other's mail, memory,
// approvals or chats.
func TestGateFamilyIsolation(t *testing.T) {
	ta := newApp(t, nil, &llm.Fake{})
	ctx := context.Background()
	mailboxWith(t, ta, [][]byte{message("OWNER-SECRET-MAIL")})
	ta.Vault.Set(ctx, "telegram.token", "1:t")
	ta.Events.Put(ctx, "telegram.chat", "100")
	var mu sync.Mutex
	chats := map[string]int{}
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ChatID int64 `json:"chat_id"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		chats[fmt.Sprint(body.ChatID)]++
		mu.Unlock()
		w.Write([]byte(`{"ok":true,"result":{"message_id":1,"chat":{"id":1}}}`))
	}))
	defer tg.Close()
	ta.TelegramAPI = tg.URL

	ana, _ := ta.People.Add(ctx, "Ana", people.Member, "")
	bia, _ := ta.People.Add(ctx, "Bia", people.Guest, "ana")
	ta.People.Pair(ctx, ana.Invite, 200)
	ta.People.Pair(ctx, bia.Invite, 300)
	anaAddr := imapServer(t, [][]byte{message("ANA-OWN-MAIL")})
	if code, _ := ta.do(t, "PUT", "/api/people/ana/connections/mail", map[string]string{"addr": anaAddr, "user": "eu@exemplo.com", "password": "pw"}); code != 200 {
		t.Fatalf("ana mail %d", code)
	}
	ta.Memory.Add("OWNER-FACT the safe code hint is 7", "casa", "owner", memory.High)
	ta.Memory.AddFor("ANA-FACT likes jazz", "gostos", "owner", memory.High, "ana")
	ta.Memory.AddFor("BIA-FACT allergic to nuts", "saúde", "owner", memory.High, "bia")
	ta.Memory.AddFor("HOUSE-FACT wifi is on the fridge", "casa", "owner", memory.High, people.Household)

	runs := map[string]seenRun{}
	ta.Agent = llm.FakeAgent{Script: func(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
		var s seenRun
		s.prompt = r.System
		s.mail, s.mailErr = callTool(r.MCPURL, "gmail_search", map[string]any{"query": "", "max": 10})
		s.memory, _ = callTool(r.MCPURL, "memory_search", map[string]any{"query": "fact"})
		s.note, _ = callTool(r.MCPURL, "memory_note", map[string]any{"fact": "NOTE-BY-" + r.Prompt})
		callTool(r.MCPURL, "telegram_send", map[string]any{"text": "oi"})
		mu.Lock()
		runs[r.Prompt] = s
		mu.Unlock()
		return llm.Response{Text: "ok"}, nil
	}}
	for _, who := range []string{"owner", "ana", "bia"} {
		if _, err := (handler{ta.App}).Request(people.With(ctx, who), who); err != nil {
			t.Fatal(err)
		}
	}
	ta.Explore.Wait()

	sees := func(who, what, where, text string, want bool) {
		t.Helper()
		if strings.Contains(text, what) != want {
			t.Errorf("%s sees %s in %s: %v, want %v", who, what, where, !want, want)
		}
	}
	o, a, b := runs["owner"], runs["ana"], runs["bia"]
	sees("owner", "OWNER-SECRET-MAIL", "mail", o.mail, true)
	sees("ana", "ANA-OWN-MAIL", "mail", a.mail, true)
	sees("ana", "OWNER-SECRET-MAIL", "mail", a.mail, false)
	if !b.mailErr || strings.Contains(b.mail, "MAIL") {
		t.Errorf("bia, with no mailbox, read one: %q", b.mail)
	}
	for who, r := range map[string]seenRun{"owner": o, "ana": a, "bia": b} {
		for _, f := range []string{"OWNER-FACT", "ANA-FACT", "BIA-FACT"} {
			own := strings.HasPrefix(f, strings.ToUpper(who))
			sees(who, f, "prompt", r.prompt, own)
			sees(who, f, "search", r.memory, own)
		}
		sees(who, "HOUSE-FACT", "prompt", r.prompt, true)
		sees(who, "HOUSE-FACT", "search", r.memory, true)
	}
	facts, _ := ta.Memory.List()
	for _, f := range facts {
		if strings.HasPrefix(f.Text, "NOTE-BY-") && people.Norm(f.Person) != strings.TrimPrefix(f.Text, "NOTE-BY-") {
			t.Errorf("a note was kept for the wrong person: %+v", f)
		}
	}
	mu.Lock()
	// Each gets their own message and the notice that their exploration ended.
	if len(chats) != 3 || chats["100"] != 2 || chats["200"] != 2 || chats["300"] != 2 {
		t.Errorf("each person's messages should reach only their own chat: %v", chats)
	}
	mu.Unlock()

	// A guest's change waits for their responsible, who alone (with the
	// owner) may answer it.
	ta.Approvals.Timeout = 5 * time.Second
	var asked approval.Request
	got := make(chan struct{})
	sub := ta.Events.Subscribe(ctx)
	go func() {
		for e := range sub {
			if e.Type == approval.EventRequested {
				e.Decode(&asked)
				close(got)
				return
			}
		}
	}()
	h := &host.Host{Env: ta.Explore.Env, Source: "routine:bia-r#1", Person: "bia"}
	done := make(chan error, 1)
	go func() {
		_, err := h.Call(ctx, "gmail.label", "", map[string]any{"id": "1", "label": "x"})
		done <- err
	}()
	<-got
	if asked.Responsible != "ana" || asked.Action.Role != "guest" {
		t.Fatalf("approval went to %q for role %q", asked.Responsible, asked.Action.Role)
	}
	if _, err := (handler{ta.App}).Button(people.With(ctx, "bia"), "approve", asked.ID); err == nil {
		t.Fatal("a guest approved her own request")
	}
	if reply, err := (handler{ta.App}).Button(people.With(ctx, "ana"), "always", asked.ID); err != nil || reply != "Permitido." {
		t.Fatalf("ana could not answer for bia: %q %v", reply, err)
	}
	<-done
	for _, r := range ta.Rules.Rules(ctx) {
		if strings.HasPrefix(r.ID, "always-") {
			t.Fatal("a member created a lasting rule")
		}
	}
	mu.Lock()
	if chats["200"] < 3 || chats["300"] != 2 {
		t.Errorf("the approval did not reach ana's chat: %v", chats)
	}
	mu.Unlock()

	// Buttons for the owner's things do nothing for others.
	exps, _ := ta.Store.Explorations(ctx)
	for _, e := range exps {
		if e.Person == "" {
			if _, err := (handler{ta.App}).Button(people.With(ctx, "ana"), "discard", e.ID); err == nil {
				t.Fatal("ana discarded the owner's exploration")
			}
		}
	}
	if _, err := (handler{ta.App}).Button(people.With(ctx, "ana"), "run", "anything"); err == nil {
		t.Fatal("ana ran a routine")
	}

	// Removing someone takes their accounts with them.
	ta.do(t, "DELETE", "/api/people/ana", nil)
	if _, err := ta.Vault.Get(ctx, "person.ana.mail.password"); err == nil {
		t.Fatal("a removed person's password stayed in the vault")
	}
}
