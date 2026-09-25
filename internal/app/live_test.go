//go:build live

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

type literal struct {
	*bytes.Reader
	n int64
}

func (l literal) Size() int64 { return l.n }

// TestLiveMorningBrief runs the real pipeline with Claude Code: explore a
// morning brief over MCP, compile it, and run the routine without a model.
// External services are local fakes. Run with: go test -tags live -run Live ./internal/app
func TestLiveMorningBrief(t *testing.T) {
	now := time.Now()
	// Mailbox: one important email, one promotion.
	mem := imapmemserver.New()
	user := imapmemserver.NewUser("eu@exemplo.com", "pw")
	user.Create("INBOX", nil)
	for _, m := range []struct{ from, subj, body string }{
		{"Ana Souza <ana@acme.com>", "Contrato Q4 precisa da sua assinatura hoje", "Oi! O jurídico precisa da assinatura até as 17h."},
		{"Loja Mega <promo@megaloja.com>", "Só hoje: 70% OFF em tudo", "Aproveite as ofertas imperdíveis."},
	} {
		raw := []byte("From: " + m.from + "\r\nTo: eu@exemplo.com\r\nSubject: " + m.subj + "\r\nDate: " + now.Add(-2*time.Hour).Format(time.RFC1123Z) + "\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + m.body + "\r\n")
		user.Append("INBOX", literal{bytes.NewReader(raw), int64(len(raw))}, &imap.AppendOptions{Time: now.Add(-2 * time.Hour)})
	}
	mem.AddUser(user)
	imapSrv := imapserver.New(&imapserver.Options{NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
		return mem.NewSession(), nil, nil
	}, InsecureAuth: true, Caps: imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}}})
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go imapSrv.Serve(ln)
	defer imapSrv.Close()

	// Calendar: a standup today.
	day := now.Format("20060102")
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nX-WR-CALNAME:Trabalho\r\nBEGIN:VEVENT\r\nUID:s1\r\nDTSTART:" + day + "T120000Z\r\nDTEND:" + day + "T123000Z\r\nSUMMARY:Standup do time\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	calSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(ics)) }))
	defer calSrv.Close()

	// Telegram Bot API: records messages to the owner.
	var mu sync.Mutex
	var sent []string
	tgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			mu.Lock()
			sent = append(sent, body["text"].(string))
			mu.Unlock()
			w.Write([]byte(`{"ok":true,"result":{"message_id":1,"chat":{"id":42}}}`))
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			time.Sleep(time.Second)
			w.Write([]byte(`{"ok":true,"result":[]}`))
		default:
			w.Write([]byte(`{"ok":true,"result":{"username":"zodim_test_bot"}}`))
		}
	}))
	defer tgSrv.Close()

	ta := newApp(t, nil, nil)
	ta.Agent, ta.LLM = claude{ta.App}, claude{ta.App}
	ta.TelegramAPI, ta.MailInsecure = tgSrv.URL, true
	ctx := context.Background()
	ta.Vault.Set(ctx, "telegram.token", "t")
	ta.Events.Put(ctx, "telegram.chat", "42")
	ta.Events.Put(ctx, "mail.addr", ln.Addr().String())
	ta.Events.Put(ctx, "mail.user", "eu@exemplo.com")
	ta.Vault.Set(ctx, "mail.password", "pw")
	ta.Vault.Set(ctx, "calendar.feeds", calSrv.URL+"/basic.ics")
	s := ta.Settings(ctx)
	s.JudgeBackend = "llm"
	ta.SaveSettings(ctx, s, "test")
	ta.Budget.SetLimit(ctx, 3, "test")

	_, out := ta.do(t, "POST", "/api/explorations", map[string]string{"request": "Todo dia às 7h me manda no Telegram a agenda de hoje e os e-mails não lidos que forem importantes (ignore promoções)."})
	id := out["id"].(string)
	ta.Explore.Wait()
	_, out = ta.do(t, "GET", "/api/explorations/"+id, nil)
	exp := out["exploration"].(map[string]any)
	t.Logf("exploration: state=%v cost=%v actions=%d\nsummary: %v", exp["state"], exp["cost_usd"], len(out["actions"].([]any)), exp["summary"])
	if exp["state"] != "ready" {
		t.Fatalf("exploration did not finish: %v", exp["error"])
	}
	mu.Lock()
	t.Logf("sent during exploration: %q", sent)
	sent = nil
	mu.Unlock()

	code, out := ta.do(t, "POST", "/api/explorations/"+id+"/compile", nil)
	if code != 200 {
		e, _ := ta.Store.Exploration(ctx, id)
		b, _ := json.MarshalIndent(e.Trace, "", " ")
		if e.Candidate != nil {
			t.Logf("candidate code:\n%s\nmanifest: %+v", e.Candidate.Code, e.Candidate.Manifest)
		}
		t.Fatalf("compile failed: %v\ntrace: %s", out, b)
	}
	rid := out["id"].(string)
	_, detail := ta.do(t, "GET", "/api/routines/"+rid, nil)
	t.Logf("routine %s:\n%s", rid, detail["routine"].(map[string]any)["code"])

	_, run := ta.do(t, "POST", "/api/routines/"+rid+"/run", nil)
	if run["error"] != "" {
		t.Fatalf("routine run failed: %v", run["error"])
	}
	mu.Lock()
	defer mu.Unlock()
	t.Logf("routine sent: %q", sent)
	joined := strings.ToLower(strings.Join(sent, "\n"))
	if !strings.Contains(joined, "standup") || !strings.Contains(joined, "contrato") || strings.Contains(joined, "70%") {
		t.Fatalf("routine message misses the brief or includes the promotion: %q", sent)
	}
}
