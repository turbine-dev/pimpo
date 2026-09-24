package mail

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
)

func inbox(t *testing.T, m *Mail, q string) []Message {
	t.Helper()
	got, err := m.Call(context.Background(), "gmail.search", "", map[string]any{"query": q})
	if err != nil {
		t.Fatal(err)
	}
	return got.([]Message)
}

func TestTrashAndRestore(t *testing.T) {
	m := newMail(startServer(t))
	ctx := context.Background()
	msg := inbox(t, m, "from:promo@loja.com")[0]
	res, err := m.Call(ctx, "gmail.trash", "", map[string]any{"id": msg.ID})
	if err != nil {
		t.Fatal(err)
	}
	r := res.(map[string]any)
	if r["moved_to"] != "Trash" || r["message_id"] == "" {
		t.Fatalf("trash result %+v", r)
	}
	if len(inbox(t, m, "from:promo@loja.com")) != 0 {
		t.Fatal("still in inbox")
	}
	if err := m.Restore(ctx, "Trash", r["message_id"].(string)); err != nil {
		t.Fatal(err)
	}
	if len(inbox(t, m, "from:promo@loja.com")) != 1 {
		t.Fatal("not restored")
	}
	if err := m.Restore(ctx, "Trash", r["message_id"].(string)); err == nil {
		t.Fatal("restored twice")
	}
}

func TestLabelRemoveDraftDelete(t *testing.T) {
	m := newMail(startServer(t))
	ctx := context.Background()
	msg := inbox(t, m, "from:ana@acme.com")[0]
	if _, err := m.Call(ctx, "gmail.label", "", map[string]any{"id": msg.ID, "label": "Contratos"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(ctx, "Contratos", msg.MessageID); err != nil {
		t.Fatalf("remove label copy: %v", err)
	}
	res, err := m.Call(ctx, "gmail.draft", "", map[string]any{"to": "cliente@acme.com", "subject": "Proposta", "body": "Segue a proposta."})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["saved_in"] != "Drafts" {
		t.Fatalf("draft %+v", res)
	}
	if _, err := m.Call(ctx, "gmail.draft", "", map[string]any{"to": "not an address", "subject": "x"}); err == nil {
		t.Fatal("bad recipient accepted")
	}
	if _, err := m.Call(ctx, "gmail.delete", "", map[string]any{"id": msg.ID}); err != nil {
		t.Fatal(err)
	}
	if len(inbox(t, m, "from:ana@acme.com")) != 0 {
		t.Fatal("deleted message still there")
	}
}

// smtpSink is a minimal SMTP server that records one message.
func smtpSink(t *testing.T) (string, func() (string, []string)) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var mu sync.Mutex
	var data string
	var rcpt []string
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		w := func(s string) { conn.Write([]byte(s + "\r\n")) }
		w("220 sink")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				w("250 ok")
			case strings.HasPrefix(cmd, "RCPT TO:"):
				mu.Lock()
				rcpt = append(rcpt, strings.Trim(strings.TrimSpace(line)[8:], "<>"))
				mu.Unlock()
				w("250 ok")
			case cmd == "DATA":
				w("354 go")
				var b strings.Builder
				for {
					l, _ := r.ReadString('\n')
					if l == ".\r\n" {
						break
					}
					b.WriteString(l)
				}
				mu.Lock()
				data = b.String()
				mu.Unlock()
				w("250 queued")
			case cmd == "QUIT":
				w("221 bye")
				return
			default:
				w("250 ok")
			}
		}
	}()
	return ln.Addr().String(), func() (string, []string) { mu.Lock(); defer mu.Unlock(); return data, rcpt }
}

func TestSendOverSMTP(t *testing.T) {
	addr, got := smtpSink(t)
	m := newMail("unused")
	m.Account.SMTP = addr
	res, err := m.Call(context.Background(), "gmail.send", "", map[string]any{"to": "Cliente <cliente@acme.com>", "subject": "Orçamento", "body": "Olá!\nSegue."})
	if err != nil {
		t.Fatal(err)
	}
	data, rcpt := got()
	if len(rcpt) != 1 || !strings.Contains(rcpt[0], "cliente@acme.com") {
		t.Fatalf("rcpt %v", rcpt)
	}
	if !strings.Contains(data, "Subject: =?utf-8?q?Or=C3=A7amento?=") || !strings.Contains(data, "Segue.") || res.(map[string]any)["message_id"] == "" {
		t.Fatalf("data %q", data)
	}
}
