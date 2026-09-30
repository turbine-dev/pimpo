package mail

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/textproto"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/netguard"
)

// listUnsubscribe reads the List-Unsubscribe headers (RFC 2369, RFC 8058)
// and returns the best way out: a one-click https link, another https
// link, or a mailto address.
func listUnsubscribe(raw []byte) (target string, oneClick bool) {
	if len(raw) == 0 {
		return "", false
	}
	h, err := textproto.NewReader(bufio.NewReader(strings.NewReader(string(raw)))).ReadMIMEHeader()
	if err != nil && len(h) == 0 {
		return "", false
	}
	var https, mailto string
	for _, part := range strings.Split(h.Get("List-Unsubscribe"), ",") {
		v := strings.Trim(strings.TrimSpace(part), "<>")
		switch {
		case strings.HasPrefix(v, "https://") && https == "":
			https = v
		case strings.HasPrefix(strings.ToLower(v), "mailto:") && mailto == "":
			mailto = v
		}
	}
	oneClick = strings.Contains(strings.ToLower(h.Get("List-Unsubscribe-Post")), "list-unsubscribe=one-click")
	if https != "" && oneClick {
		return https, true
	}
	if mailto != "" {
		return mailto, false
	}
	return https, false
}

// authResults reads the Authentication-Results headers (RFC 8601) in
// the order they appear.
func authResults(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	h, _ := textproto.NewReader(bufio.NewReader(strings.NewReader(string(raw)))).ReadMIMEHeader()
	return h.Values("Authentication-Results")
}

// unsubscribeClient posts one-click unsubscribes: the link comes from a
// stranger's email, so it follows no redirect and never reaches this
// computer or a private network. Tests replace it.
var unsubscribeClient = &http.Client{
	Timeout:       20 * time.Second,
	Transport:     netguard.Transport(),
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func (m *Mail) unsubscribe(ctx context.Context, id string) (any, error) {
	found, err := m.findByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if found.unsubscribe == "" {
		return nil, errors.New("this sender offers no way to unsubscribe")
	}
	if found.oneClick {
		if _, err := netguard.ParseURL(found.unsubscribe); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, found.unsubscribe, strings.NewReader("List-Unsubscribe=One-Click"))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := unsubscribeClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("unsubscribe link unreachable")
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("the sender's unsubscribe link answered %d", resp.StatusCode)
		}
		return map[string]any{"ok": true, "method": "one-click", "sender": found.From}, nil
	}
	if strings.HasPrefix(strings.ToLower(found.unsubscribe), "mailto:") {
		addr := strings.TrimPrefix(strings.SplitN(found.unsubscribe[7:], "?", 2)[0], "//")
		if _, err := mail.ParseAddress(addr); err != nil {
			return nil, fmt.Errorf("the unsubscribe address %q is not valid", addr)
		}
		if _, err := m.send(ctx, Outgoing{To: []string{addr}, Subject: "unsubscribe", Body: "unsubscribe"}); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "method": "email", "sender": found.From}, nil
	}
	return nil, errors.New("this sender only offers an unsubscribe web page; open it yourself")
}

// findByID fetches one message with its headers.
func (m *Mail) findByID(ctx context.Context, id string) (Message, error) {
	msgs, err := m.search(ctx, "", 0, false, 50)
	if err != nil {
		return Message{}, err
	}
	for _, msg := range msgs {
		if msg.ID == id {
			return msg, nil
		}
	}
	return Message{}, fmt.Errorf("message %s is no longer in the inbox", id)
}
