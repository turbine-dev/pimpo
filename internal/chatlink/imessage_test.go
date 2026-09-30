package chatlink

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func messagesDB(t *testing.T) (string, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chat.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`CREATE TABLE handle (ROWID INTEGER PRIMARY KEY, id TEXT)`,
		`CREATE TABLE message (ROWID INTEGER PRIMARY KEY AUTOINCREMENT, text TEXT, attributedBody BLOB, handle_id INTEGER, is_from_me INTEGER, service TEXT DEFAULT 'iMessage')`,
		`INSERT INTO handle VALUES (1, '+5511999990000')`,
		`INSERT INTO message (text, handle_id, is_from_me) VALUES ('mensagem antiga', 1, 0)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return path, db
}

// archived is how newer macOS stores a message's text.
func archived(s string) []byte {
	b := append([]byte("\x04\x0bstreamtyped\x81\xe8\x03\x84\x01@\x84\x84\x84\x12NSAttributedString\x00\x84\x84\x08NSObject\x00\x85\x92\x84\x84\x84\x08NSString\x01\x94\x84\x01+"), byte(len(s)))
	return append(b, []byte(s)...)
}

func TestIMessageHandsOverNewMessagesOnly(t *testing.T) {
	path, db := messagesDB(t)
	m := &IMessage{DB: path, Every: 10 * time.Millisecond}
	if err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	var got []Inbound
	go m.Run(ctx, func(in Inbound) { mu.Lock(); got = append(got, in); mu.Unlock() })
	time.Sleep(50 * time.Millisecond)
	// Messages keeps its database in WAL mode, as here.
	for _, q := range [][]any{
		{`INSERT INTO message (text, handle_id, is_from_me) VALUES ('o que tenho amanhã?', 1, 0)`},
		{`INSERT INTO message (text, handle_id, is_from_me) VALUES ('resposta do Pimpo', 1, 1)`},
		// An SMS sender is easy to fake: never heard.
		{`INSERT INTO message (text, handle_id, is_from_me, service) VALUES ('pimpo por sms', 1, 0, 'SMS')`},
		{`INSERT INTO message (text, attributedBody, handle_id, is_from_me) VALUES (NULL, ?, 1, 0)`, archived("pimpo 123456")},
	} {
		if _, err := db.Exec(q[0].(string), q[1:]...); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 100; i++ {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 || got[0].Text != "o que tenho amanhã?" || got[1].Text != "pimpo 123456" || got[0].From != "+5511999990000" {
		t.Fatalf("%+v", got)
	}
}

func TestIMessageExplainsMissingAccess(t *testing.T) {
	m := &IMessage{DB: filepath.Join(t.TempDir(), "missing.db")}
	if err := m.Check(context.Background()); err == nil {
		t.Fatal("a missing database checked out")
	}
}

func TestAttributedText(t *testing.T) {
	long := string(make([]byte, 300))
	for _, s := range []string{"oi", "Olá, Pimpo! 😀"} {
		if got := attributedText(archived(s)); got != s {
			t.Fatalf("%q -> %q", s, got)
		}
	}
	b := append([]byte("NSString\x01\x94\x84\x01+\x81"), byte(len(long)&0xff), byte(len(long)>>8))
	if got := attributedText(append(b, long...)); len(got) != 300 {
		t.Fatalf("long text: %d", len(got))
	}
	if attributedText([]byte("nothing here")) != "" {
		t.Fatal("found text in nothing")
	}
}
