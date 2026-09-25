package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
)

// Outgoing is an email to someone else.
type Outgoing struct {
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
}

func (o *Outgoing) validate() error {
	if len(o.To) == 0 {
		return errors.New("to is required")
	}
	for _, a := range o.To {
		if _, err := mail.ParseAddress(a); err != nil {
			return fmt.Errorf("%q is not an email address", a)
		}
	}
	if strings.TrimSpace(o.Subject) == "" && strings.TrimSpace(o.Body) == "" {
		return errors.New("subject or body is required")
	}
	return nil
}

// UnmarshalJSON accepts `to` as one address or a list.
func (o *Outgoing) UnmarshalJSON(b []byte) error {
	var raw struct {
		To      any    `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := jsonUnmarshal(b, &raw); err != nil {
		return err
	}
	o.Subject, o.Body = raw.Subject, raw.Body
	switch t := raw.To.(type) {
	case string:
		for _, a := range strings.Split(t, ",") {
			if a = strings.TrimSpace(a); a != "" {
				o.To = append(o.To, a)
			}
		}
	case []any:
		for _, a := range t {
			if s, ok := a.(string); ok {
				o.To = append(o.To, s)
			}
		}
	}
	return nil
}

func (m *Mail) compose(o Outgoing) ([]byte, string) {
	b := make([]byte, 12)
	rand.Read(b)
	host := "zodim.local"
	if i := strings.LastIndex(m.Account.Username, "@"); i >= 0 {
		host = m.Account.Username[i+1:]
	}
	id := "<" + hex.EncodeToString(b) + "@" + host + ">"
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", m.Account.Username)
	fmt.Fprintf(&msg, "To: %s\r\n", strings.Join(o.To, ", "))
	fmt.Fprintf(&msg, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", o.Subject))
	fmt.Fprintf(&msg, "Date: %s\r\n", m.now().Format(time.RFC1123Z))
	fmt.Fprintf(&msg, "Message-ID: %s\r\n", id)
	msg.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	msg.WriteString(strings.ReplaceAll(o.Body, "\n", "\r\n"))
	msg.WriteString("\r\n")
	return msg.Bytes(), id
}

// draft saves a message in Drafts without sending it.
func (m *Mail) draft(ctx context.Context, o Outgoing) (any, error) {
	raw, id := m.compose(o)
	c, err := m.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	box, err := draftsBox(c)
	if err != nil {
		return nil, err
	}
	cmd := c.Append(box, int64(len(raw)), &imap.AppendOptions{Flags: []imap.Flag{imap.FlagDraft, imap.FlagSeen}})
	if _, err := cmd.Write(raw); err != nil {
		return nil, err
	}
	if err := cmd.Close(); err != nil {
		return nil, err
	}
	if _, err := cmd.Wait(); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "saved_in": box, "message_id": id}, nil
}

// send delivers a message over SMTP with the same app password.
func (m *Mail) send(ctx context.Context, o Outgoing) (any, error) {
	raw, id := m.compose(o)
	var pw, token string
	var err error
	if m.Account.Token != nil {
		token, err = m.Account.Token(ctx)
	} else {
		pw, err = m.Account.Password(ctx)
	}
	if err != nil {
		return nil, err
	}
	addr := m.Account.SMTP
	if addr == "" {
		addr = "smtp.gmail.com:465"
	}
	host, port, _ := net.SplitHostPort(addr)
	var conn net.Conn
	d := &net.Dialer{Timeout: 20 * time.Second}
	switch {
	case m.Account.Insecure:
		conn, err = d.DialContext(ctx, "tcp", addr)
	case port == "465":
		conn, err = tls.DialWithDialer(d, "tcp", addr, &tls.Config{ServerName: host})
	default:
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s: %w", addr, err)
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return nil, err
	}
	defer c.Close()
	if !m.Account.Insecure && port != "465" {
		if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return nil, err
		}
	}
	if !m.Account.Insecure {
		var auth smtp.Auth = smtp.PlainAuth("", m.Account.Username, pw, host)
		if token != "" {
			auth = xoauth2{user: m.Account.Username, token: token}
		}
		if err := c.Auth(auth); err != nil {
			return nil, errors.New("mail server refused the sign-in for sending")
		}
	}
	if err := c.Mail(m.Account.Username); err != nil {
		return nil, err
	}
	for _, to := range o.To {
		a, _ := mail.ParseAddress(to)
		if err := c.Rcpt(a.Address); err != nil {
			return nil, fmt.Errorf("recipient %s refused: %w", a.Address, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(raw); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	c.Quit()
	return map[string]any{"ok": true, "message_id": id, "to": o.To}, nil
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

// xoauth2 is Gmail's SMTP OAuth mechanism.
type xoauth2 struct{ user, token string }

func (a xoauth2) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "XOAUTH2", []byte("user=" + a.user + "\x01auth=Bearer " + a.token + "\x01\x01"), nil
}

func (a xoauth2) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		// The server sent an error challenge; an empty reply ends the exchange.
		return []byte{}, nil
	}
	return nil, nil
}
