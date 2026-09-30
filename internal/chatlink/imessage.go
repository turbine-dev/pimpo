package chatlink

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// IMessage talks through the Messages app on this Mac: it reads new
// messages from Messages' own database, read-only, and answers with
// AppleScript. Reading the database needs Full Disk Access for Pimpo.
type IMessage struct {
	// DB is Messages' database; empty is ~/Library/Messages/chat.db.
	DB string
	// Every is how often new messages are looked for.
	Every time.Duration
	// send replaces AppleScript in tests.
	send func(ctx context.Context, to, text string) error
}

var ErrNoDiskAccess = errors.New("Pimpo cannot read Messages: in System Settings › Privacy & Security › Full Disk Access, turn it on for Pimpo, then restart Pimpo")

func (m *IMessage) Name() string { return "imessage" }

func (m *IMessage) path() string {
	if m.DB != "" {
		return m.DB
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Messages", "chat.db")
}

func (m *IMessage) open() (*sql.DB, error) {
	if m.DB == "" && runtime.GOOS != "darwin" {
		return nil, errors.New("iMessage works only on a Mac")
	}
	if _, err := os.Stat(m.path()); err != nil {
		if os.IsPermission(err) {
			return nil, ErrNoDiskAccess
		}
		return nil, fmt.Errorf("Messages has no database here (%s); sign in to Messages on this Mac first", m.path())
	}
	db, err := sql.Open("sqlite", "file:"+m.path()+"?mode=ro&_pragma=busy_timeout(3000)")
	if err != nil {
		return nil, err
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM message LIMIT 1`).Scan(&n); err != nil {
		db.Close()
		if strings.Contains(err.Error(), "authorization denied") || strings.Contains(err.Error(), "unable to open") || strings.Contains(err.Error(), "operation not permitted") {
			return nil, ErrNoDiskAccess
		}
		return nil, err
	}
	return db, nil
}

func (m *IMessage) Check(ctx context.Context) error {
	db, err := m.open()
	if err != nil {
		return err
	}
	return db.Close()
}

// Run looks for messages that arrive after it starts; older ones are never
// answered. Only iMessage counts: an SMS sender's number is easy to fake,
// so texts relayed from the iPhone are ignored.
func (m *IMessage) Run(ctx context.Context, on func(Inbound)) error {
	db, err := m.open()
	if err != nil {
		return err
	}
	defer db.Close()
	var last int64
	if err := db.QueryRowContext(ctx, `SELECT coalesce(max(ROWID), 0) FROM message`).Scan(&last); err != nil {
		return err
	}
	every := m.Every
	if every == 0 {
		every = 3 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
		rows, err := db.QueryContext(ctx, `SELECT m.ROWID, coalesce(h.id, ''), coalesce(m.text, ''), m.attributedBody
			FROM message m LEFT JOIN handle h ON h.ROWID = m.handle_id
			WHERE m.ROWID > ? AND m.is_from_me = 0 AND m.service = 'iMessage' ORDER BY m.ROWID`, last)
		if err != nil {
			return err
		}
		var got []Inbound
		for rows.Next() {
			var id int64
			var from, text string
			var body []byte
			if err := rows.Scan(&id, &from, &text, &body); err != nil {
				rows.Close()
				return err
			}
			last = id
			if text == "" {
				text = attributedText(body)
			}
			if text = strings.TrimSpace(strings.ReplaceAll(text, "￼", "")); text != "" && from != "" {
				got = append(got, Inbound{From: from, Text: text})
			}
		}
		rows.Close()
		for _, in := range got {
			on(in)
		}
	}
}

// attributedText reads the text of a message that newer macOS keeps only
// in attributedBody (an archived NSAttributedString): the string follows
// the NSString class name, after a length.
func attributedText(b []byte) string {
	i := bytes.Index(b, []byte("NSString"))
	if i < 0 {
		return ""
	}
	b = b[i+len("NSString"):]
	j := bytes.IndexByte(b, '+')
	if j < 0 || j+1 >= len(b) {
		return ""
	}
	b = b[j+1:]
	n, skip := int(b[0]), 1
	switch b[0] {
	case 0x81:
		if len(b) < 3 {
			return ""
		}
		n, skip = int(b[1])|int(b[2])<<8, 3
	case 0x82:
		if len(b) < 5 {
			return ""
		}
		n, skip = int(b[1])|int(b[2])<<8|int(b[3])<<16|int(b[4])<<24, 5
	}
	if skip+n > len(b) {
		return ""
	}
	return string(b[skip : skip+n])
}

const sendScript = `on run argv
	tell application "Messages"
		set s to 1st account whose service type = iMessage
		send (item 1 of argv) to participant (item 2 of argv) of s
	end tell
end run`

// Send answers through Messages; the text and the address travel as
// arguments, never inside the script.
func (m *IMessage) Send(ctx context.Context, to, text string) error {
	if m.send != nil {
		return m.send(ctx, to, text)
	}
	out, err := exec.CommandContext(ctx, "osascript", "-e", sendScript, text, to).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Messages did not send: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
