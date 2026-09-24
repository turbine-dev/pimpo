// Package mail reads and files email over IMAP. With Gmail it uses an app
// password; any other IMAP provider works the same way.
package mail

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
	gomail "github.com/emersion/go-message/mail"

	"github.com/denerFernandes/vigia/internal/connector"
)

type Message struct {
	ID       string   `json:"id"`
	From     string   `json:"from"`
	FromName string   `json:"from_name"`
	To       []string `json:"to"`
	Subject  string   `json:"subject"`
	Snippet  string   `json:"snippet"`
	Date     string   `json:"date"`
	Labels   []string `json:"labels"`
	Replied  bool     `json:"replied"`
	Unread   bool     `json:"unread"`
}

// Account is one IMAP mailbox. Password is resolved from the vault per call.
type Account struct {
	Addr     string // host:port, implicit TLS
	Username string
	Password func(ctx context.Context) (string, error)
	// Insecure uses plain TCP; only for tests.
	Insecure bool
	Now      func() time.Time
}

type Mail struct {
	Account Account
}

func (m *Mail) Capabilities() []string {
	return []string{"gmail.search", "gmail.archive", "gmail.label"}
}

func (m *Mail) Call(ctx context.Context, capability, _ string, args any) (any, error) {
	switch capability {
	case "gmail.search":
		var a struct {
			Query  string `json:"query"`
			Days   int    `json:"days"`
			Unread bool   `json:"unread"`
			Max    int    `json:"max"`
		}
		if err := connector.Args(args, &a); err != nil {
			return nil, err
		}
		return m.search(ctx, a.Query, a.Days, a.Unread, a.Max)
	case "gmail.archive":
		id, err := idArg(args)
		if err != nil {
			return nil, err
		}
		to, err := m.archive(ctx, id)
		return map[string]any{"ok": err == nil, "moved_to": to}, err
	case "gmail.label":
		var a struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		}
		if err := connector.Args(args, &a); err != nil {
			return nil, err
		}
		if a.Label == "" {
			return nil, errors.New("label is required")
		}
		return map[string]any{"ok": true}, m.label(ctx, a.ID, a.Label)
	}
	return nil, fmt.Errorf("mail cannot do %s", capability)
}

func idArg(args any) (string, error) {
	var a struct {
		ID string `json:"id"`
	}
	if err := connector.Args(args, &a); err != nil {
		return "", err
	}
	if a.ID == "" {
		return "", errors.New("id is required")
	}
	return a.ID, nil
}

func (m *Mail) dial(ctx context.Context) (*imapclient.Client, error) {
	pw, err := m.Account.Password(ctx)
	if err != nil {
		return nil, err
	}
	var c *imapclient.Client
	opts := &imapclient.Options{}
	if m.Account.Insecure {
		conn, derr := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, "tcp", m.Account.Addr)
		if derr != nil {
			return nil, derr
		}
		c = imapclient.New(conn, opts)
	} else {
		c, err = imapclient.DialTLS(m.Account.Addr, opts)
		if err != nil {
			return nil, fmt.Errorf("cannot reach %s: %w", m.Account.Addr, err)
		}
	}
	if err := c.Login(m.Account.Username, pw).Wait(); err != nil {
		c.Close()
		return nil, fmt.Errorf("mail login refused; check the app password in Connections")
	}
	return c, nil
}

func (m *Mail) now() time.Time {
	if m.Account.Now != nil {
		return m.Account.Now()
	}
	return time.Now()
}

func (m *Mail) search(ctx context.Context, query string, days int, unread bool, max int) ([]Message, error) {
	crit, err := Criteria(query, m.now())
	if err != nil {
		return nil, err
	}
	if days > 0 {
		merge(crit, &imap.SearchCriteria{Since: m.now().AddDate(0, 0, -days)})
	}
	if unread {
		crit.NotFlag = append(crit.NotFlag, imap.FlagSeen)
	}
	if max <= 0 || max > 50 {
		max = min(max, 50)
		if max <= 0 {
			max = 20
		}
	}
	c, err := m.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if _, err := c.Select("INBOX", &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return nil, err
	}
	data, err := c.UIDSearch(crit, nil).Wait()
	if err != nil {
		return nil, err
	}
	uids := data.AllUIDs()
	sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
	if len(uids) > max {
		uids = uids[:max]
	}
	if len(uids) == 0 {
		return []Message{}, nil
	}
	section := &imap.FetchItemBodySection{Peek: true, Partial: &imap.SectionPartial{Offset: 0, Size: 64 << 10}}
	msgs, err := c.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{UID: true, Envelope: true, Flags: true, InternalDate: true, BodySection: []*imap.FetchItemBodySection{section}}).Collect()
	if err != nil {
		return nil, err
	}
	exact := exactAddresses(query)
	out := make([]Message, 0, len(msgs))
	for _, fm := range msgs {
		msg := toMessage(fm, section)
		if exact.match(msg) {
			out = append(out, msg)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out, nil
}

func toMessage(fm *imapclient.FetchMessageBuffer, section *imap.FetchItemBodySection) Message {
	msg := Message{ID: "INBOX/" + strconv.FormatUint(uint64(fm.UID), 10), Labels: []string{"INBOX"}, To: []string{}}
	if env := fm.Envelope; env != nil {
		msg.Subject = env.Subject
		if len(env.From) > 0 {
			msg.From = strings.ToLower(env.From[0].Addr())
			msg.FromName = env.From[0].Name
		}
		for _, a := range env.To {
			msg.To = append(msg.To, strings.ToLower(a.Addr()))
		}
		if !env.Date.IsZero() {
			msg.Date = env.Date.Format(time.RFC3339)
		}
	}
	if msg.Date == "" {
		msg.Date = fm.InternalDate.Format(time.RFC3339)
	}
	msg.Unread = true
	for _, f := range fm.Flags {
		switch f {
		case imap.FlagSeen:
			msg.Unread = false
		case imap.FlagAnswered:
			msg.Replied = true
		case imap.FlagFlagged:
			msg.Labels = append(msg.Labels, "STARRED")
		}
	}
	if !msg.Unread {
		msg.Labels = append(msg.Labels, "READ")
	} else {
		msg.Labels = append(msg.Labels, "UNREAD")
	}
	msg.Snippet = snippet(fm.FindBodySection(section))
	return msg
}

// IMAP searches FROM and TO by substring, so "ana@acme.com" also finds
// "mariana@acme.com". Gmail matches whole addresses; so do we, afterwards.
type addressFilter struct{ from, to []string }

func exactAddresses(query string) addressFilter {
	var f addressFilter
	toks, _ := tokenize(query)
	for i, t := range toks {
		if (i > 0 && strings.EqualFold(toks[i-1], "OR")) || (i+1 < len(toks) && strings.EqualFold(toks[i+1], "OR")) {
			continue
		}
		key, val, ok := strings.Cut(strings.TrimPrefix(t, "-"), ":")
		val = strings.ToLower(strings.Trim(val, `"`))
		if !ok || !strings.Contains(val, "@") || strings.HasPrefix(t, "-") {
			continue
		}
		switch strings.ToLower(key) {
		case "from":
			f.from = append(f.from, val)
		case "to":
			f.to = append(f.to, val)
		}
	}
	return f
}

func (f addressFilter) match(m Message) bool {
	for _, a := range f.from {
		if m.From != a {
			return false
		}
	}
	for _, a := range f.to {
		found := false
		for _, t := range m.To {
			found = found || t == a
		}
		if !found {
			return false
		}
	}
	return true
}

var spaces = regexp.MustCompile(`\s+`)
var tags = regexp.MustCompile(`(?s)<style.*?</style>|<script.*?</script>|<[^>]+>`)

// snippet returns the first readable text of a message, preferring the
// plain-text part.
func snippet(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	r, err := gomail.CreateReader(bufio.NewReader(strings.NewReader(string(raw))))
	var text, html string
	if err == nil || message.IsUnknownCharset(err) {
		for {
			p, err := r.NextPart()
			if err != nil {
				break
			}
			if h, ok := p.Header.(*gomail.InlineHeader); ok {
				ct, _, _ := h.ContentType()
				b, _ := io.ReadAll(io.LimitReader(p.Body, 32<<10))
				if ct == "text/plain" && text == "" {
					text = string(b)
				} else if ct == "text/html" && html == "" {
					html = string(b)
				}
			}
		}
	}
	if text == "" && html != "" {
		text = tags.ReplaceAllString(html, " ")
	}
	text = strings.TrimSpace(spaces.ReplaceAllString(text, " "))
	if len([]rune(text)) > 280 {
		text = string([]rune(text)[:280]) + "…"
	}
	return text
}

func parseID(id string) (string, imap.UID, error) {
	box, n, ok := strings.Cut(id, "/")
	u, err := strconv.ParseUint(n, 10, 32)
	if !ok || err != nil || box == "" {
		return "", 0, fmt.Errorf("%q is not a message id from gmail.search", id)
	}
	return box, imap.UID(u), nil
}

// archive moves a message out of the inbox: to "[Gmail]/All Mail" on
// Gmail, to the \Archive mailbox elsewhere. It returns where it went so the
// move can be undone.
func (m *Mail) archive(ctx context.Context, id string) (string, error) {
	box, uid, err := parseID(id)
	if err != nil {
		return "", err
	}
	c, err := m.dial(ctx)
	if err != nil {
		return "", err
	}
	defer c.Close()
	target, err := archiveBox(c)
	if err != nil {
		return "", err
	}
	if _, err := c.Select(box, nil).Wait(); err != nil {
		return "", err
	}
	if _, err := c.Move(imap.UIDSetNum(uid), target).Wait(); err != nil {
		return "", fmt.Errorf("archive %s: %w", id, err)
	}
	return target, nil
}

func archiveBox(c *imapclient.Client) (string, error) {
	boxes, err := c.List("", "*", &imap.ListOptions{ReturnSpecialUse: true}).Collect()
	if err != nil {
		return "", err
	}
	for _, want := range []imap.MailboxAttr{imap.MailboxAttrArchive, imap.MailboxAttrAll} {
		for _, b := range boxes {
			for _, a := range b.Attrs {
				if a == want {
					return b.Mailbox, nil
				}
			}
		}
	}
	for _, b := range boxes {
		if strings.EqualFold(b.Mailbox, "Archive") {
			return b.Mailbox, nil
		}
	}
	if err := c.Create("Archive", nil).Wait(); err != nil {
		return "", err
	}
	return "Archive", nil
}

// label files a copy under a mailbox named after the label. On Gmail a
// mailbox is a label, so this applies the label without moving the message.
func (m *Mail) label(ctx context.Context, id, label string) error {
	box, uid, err := parseID(id)
	if err != nil {
		return err
	}
	c, err := m.dial(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	exists := false
	boxes, _ := c.List("", label, nil).Collect()
	exists = len(boxes) > 0
	if !exists {
		if err := c.Create(label, nil).Wait(); err != nil {
			return err
		}
	}
	if _, err := c.Select(box, nil).Wait(); err != nil {
		return err
	}
	_, err = c.Copy(imap.UIDSetNum(uid), label).Wait()
	return err
}
