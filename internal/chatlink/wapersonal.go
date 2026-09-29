package chatlink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/coder/websocket"
)

// WhatsAppPersonal talks through a WhatsApp Web bridge on this machine
// (WAHA, github.com/devlikeapro/waha), signed in to a personal number with
// its QR code. WhatsApp does not allow this: the number can be banned and
// the bridge breaks when WhatsApp changes. It is off until the owner turns
// it on knowing that, and it never approves anything: choices wait for
// the app or another channel.
type WhatsAppPersonal struct {
	URL     string
	Session string
	APIKey  string
}

func (w *WhatsAppPersonal) Name() string { return "wapersonal" }

func (w *WhatsAppPersonal) base() string {
	if w.URL == "" {
		return "http://127.0.0.1:3000"
	}
	return strings.TrimRight(w.URL, "/")
}

func (w *WhatsAppPersonal) session() string {
	if w.Session == "" {
		return "default"
	}
	return w.Session
}

func (w *WhatsAppPersonal) call(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, w.base()+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if w.APIKey != "" {
		req.Header.Set("X-Api-Key", w.APIKey)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("the WhatsApp bridge is not answering at %s; start WAHA and sign in with the QR code", w.base())
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Errorf("the WhatsApp bridge answered %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
	}
	return nil
}

// Check says whether the bridge's session is signed in.
func (w *WhatsAppPersonal) Check(ctx context.Context) error {
	var s struct {
		Status string `json:"status"`
	}
	if err := w.call(ctx, "GET", "/api/sessions/"+url.PathEscape(w.session()), nil, &s); err != nil {
		return err
	}
	if s.Status != "WORKING" {
		return fmt.Errorf("the WhatsApp session is %s; open the bridge and scan its QR code with the phone", strings.ToLower(s.Status))
	}
	return nil
}

func (w *WhatsAppPersonal) Send(ctx context.Context, to, text string) error {
	return w.call(ctx, "POST", "/api/sendText", map[string]string{"session": w.session(), "chatId": to, "text": text}, nil)
}

// Run reads the bridge's events. Only private messages from others are
// handed over; groups and Pimpo's own messages are not.
func (w *WhatsAppPersonal) Run(ctx context.Context, on func(Inbound)) error {
	u, err := url.Parse(w.base())
	if err != nil {
		return err
	}
	u.Scheme = map[string]string{"https": "wss"}[u.Scheme]
	if u.Scheme == "" {
		u.Scheme = "ws"
	}
	u.Path = "/ws"
	q := url.Values{"session": {w.session()}, "events": {"message"}}
	if w.APIKey != "" {
		q.Set("x-api-key", w.APIKey)
	}
	u.RawQuery = q.Encode()
	conn, _, err := websocket.Dial(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("the WhatsApp bridge is not answering at %s: %w", w.base(), err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4 << 20)
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		if in, ok := waInbound(data); ok {
			on(in)
		}
	}
}

func waInbound(data []byte) (Inbound, bool) {
	var ev struct {
		Event   string `json:"event"`
		Payload struct {
			From   string `json:"from"`
			FromMe bool   `json:"fromMe"`
			Body   string `json:"body"`
		} `json:"payload"`
	}
	if json.Unmarshal(data, &ev) != nil || ev.Event != "message" || ev.Payload.FromMe {
		return Inbound{}, false
	}
	from, text := ev.Payload.From, strings.TrimSpace(ev.Payload.Body)
	if !strings.HasSuffix(from, "@c.us") && !strings.HasSuffix(from, "@s.whatsapp.net") || text == "" {
		return Inbound{}, false // groups, broadcasts, status updates
	}
	return Inbound{From: from, Text: text}, true
}

// NoApprovals marks a link that must never approve an action.
type NoApprovals interface{ NoApprovals() bool }

func (w *WhatsAppPersonal) NoApprovals() bool { return true }
