package mail

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

var now = time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)

func raw(from, subject, body string, date time.Time, html bool) []byte {
	ct := "text/plain; charset=utf-8"
	if html {
		ct = "text/html; charset=utf-8"
	}
	id := "<" + strings.ReplaceAll(strings.ToLower(subject), " ", "-") + "@test>"
	return []byte("From: " + from + "\r\nTo: eu@exemplo.com\r\nSubject: " + subject + "\r\nMessage-ID: " + id + "\r\nDate: " + date.Format(time.RFC1123Z) + "\r\nContent-Type: " + ct + "\r\n\r\n" + body + "\r\n")
}

func startServer(t *testing.T) string {
	t.Helper()
	mem := imapmemserver.New()
	user := imapmemserver.NewUser("eu@exemplo.com", "app-password")
	user.Create("INBOX", nil)
	// Messages arrive oldest first, as in a real mailbox.
	add := func(from, subject, body string, received time.Time, html bool, flags ...imap.Flag) {
		msg := raw(from, subject, body, received, html)
		if _, err := user.Append("INBOX", literal{bytes.NewReader(msg), int64(len(msg))}, &imap.AppendOptions{Flags: flags, Time: received}); err != nil {
			t.Fatal(err)
		}
	}
	add("Bruno <bruno@acme.com>", "Relatório antigo", "Já respondido.", now.Add(-10*24*time.Hour), false, imap.FlagSeen, imap.FlagAnswered)
	add("Loja <promo@loja.com>", "50% OFF hoje", "<html><style>p{}</style><p>Aproveite <b>50% de desconto</b></p></html>", now.Add(-3*time.Hour), true)
	add("Mariana <mariana@acme.com>", "Fotos do churrasco", "Olha as fotos!", now.Add(-150*time.Minute), false, imap.FlagSeen)
	add("Ana Souza <ana@acme.com>", "Contrato Q4", "Oi, segue o contrato revisado para assinatura.", now.Add(-2*time.Hour), false)
	mem.AddUser(user)
	srv := imapserver.New(&imapserver.Options{NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
		return mem.NewSession(), nil, nil
	}, InsecureAuth: true, Caps: imap.CapSet{imap.CapIMAP4rev2: {}, imap.CapIMAP4rev1: {}, imap.CapMove: {}}})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().String()
}

// startServerWith starts an IMAP server holding exactly the given raw messages.
func startServerWith(t *testing.T, raws ...string) string {
	t.Helper()
	mem := imapmemserver.New()
	user := imapmemserver.NewUser("eu@exemplo.com", "app-password")
	user.Create("INBOX", nil)
	for _, r := range raws {
		user.Append("INBOX", literal{bytes.NewReader([]byte(r)), int64(len(r))}, &imap.AppendOptions{Time: now})
	}
	mem.AddUser(user)
	srv := imapserver.New(&imapserver.Options{NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
		return mem.NewSession(), nil, nil
	}, InsecureAuth: true, Caps: imap.CapSet{imap.CapIMAP4rev2: {}, imap.CapIMAP4rev1: {}, imap.CapMove: {}}})
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().String()
}

type literal struct {
	*bytes.Reader
	n int64
}

func (l literal) Size() int64 { return l.n }

func newMail(addr string) *Mail {
	return &Mail{Account: Account{Addr: addr, Username: "eu@exemplo.com", Insecure: true, Now: func() time.Time { return now },
		Password: func(context.Context) (string, error) { return "app-password", nil }}}
}

func subjects(v any) string {
	var out []string
	for _, m := range v.([]Message) {
		out = append(out, m.Subject)
	}
	return strings.Join(out, ",")
}

func TestSearchWithGmailSyntax(t *testing.T) {
	m := newMail(startServer(t))
	ctx := context.Background()
	for q, want := range map[string]string{
		"is:unread":                      "Contrato Q4,50% OFF hoje",
		"from:ana@acme.com":              "Contrato Q4",
		"is:answered":                    "Relatório antigo",
		"newer_than:3d":                  "Contrato Q4,Fotos do churrasco,50% OFF hoje",
		"contrato OR desconto":           "Contrato Q4,50% OFF hoje",
		"-from:promo@loja.com is:unread": "Contrato Q4",
	} {
		got, err := m.Call(ctx, "gmail.search", "", map[string]any{"query": q})
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if subjects(got) != want {
			t.Errorf("%q: got %q, want %q", q, subjects(got), want)
		}
	}
	got, _ := m.Call(ctx, "gmail.search", "", map[string]any{"query": "", "max": 1})
	msgs := got.([]Message)
	if len(msgs) != 1 || msgs[0].FromName != "Ana Souza" || msgs[0].From != "ana@acme.com" || !msgs[0].Unread {
		t.Fatalf("newest message %+v", msgs)
	}
	if !strings.Contains(msgs[0].Snippet, "contrato revisado") {
		t.Fatalf("snippet %q", msgs[0].Snippet)
	}
	got, _ = m.Call(ctx, "gmail.search", "", map[string]any{"query": "from:promo@loja.com"})
	if s := got.([]Message)[0].Snippet; s != "Aproveite 50% de desconto" {
		t.Fatalf("html snippet %q", s)
	}
	got, _ = m.Call(ctx, "gmail.search", "", map[string]any{"query": "is:answered"})
	if !got.([]Message)[0].Replied {
		t.Fatal("answered flag not reported as replied")
	}
}

func TestArchiveAndLabel(t *testing.T) {
	m := newMail(startServer(t))
	ctx := context.Background()
	got, _ := m.Call(ctx, "gmail.search", "", map[string]any{"query": "from:promo@loja.com"})
	id := got.([]Message)[0].ID
	if _, err := m.Call(ctx, "gmail.label", "", map[string]any{"id": id, "label": "Promoções"}); err != nil {
		t.Fatal(err)
	}
	res, err := m.Call(ctx, "gmail.archive", "", map[string]any{"id": id})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["moved_to"] != "Archive" {
		t.Fatalf("archive result %+v", res)
	}
	got, _ = m.Call(ctx, "gmail.search", "", map[string]any{"query": "from:promo@loja.com"})
	if len(got.([]Message)) != 0 {
		t.Fatal("archived message still in the inbox")
	}
	if _, err := m.Call(ctx, "gmail.archive", "", map[string]any{"id": "nonsense"}); err == nil {
		t.Fatal("accepted a bad id")
	}
}

func TestWrongPasswordIsExplained(t *testing.T) {
	m := newMail(startServer(t))
	m.Account.Password = func(context.Context) (string, error) { return "wrong", nil }
	_, err := m.Call(context.Background(), "gmail.search", "", map[string]any{"query": ""})
	if err == nil || !strings.Contains(err.Error(), "app password") {
		t.Fatalf("got %v", err)
	}
}

func TestCriteriaErrors(t *testing.T) {
	for _, q := range []string{`"unclosed`, "newer_than:xd", "after:tomorrow"} {
		if _, err := Criteria(q, now); err == nil {
			t.Errorf("%q accepted", q)
		}
	}
}
