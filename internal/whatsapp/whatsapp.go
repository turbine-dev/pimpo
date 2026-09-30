// Package whatsapp talks to the official WhatsApp Business Cloud API:
// sending text and reply buttons, and reading the signed webhook Meta
// posts when someone writes to the business number.
package whatsapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const graph = "https://graph.facebook.com/v21.0"

type Client struct {
	Token   string
	PhoneID string
	// BaseURL replaces the Graph API in tests.
	BaseURL string
	HTTP    *http.Client
}

// Button is a reply button; WhatsApp shows at most three per message and
// titles of up to 20 characters.
type Button struct {
	ID    string
	Title string
}

func (c Client) base() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return graph
}

// Send delivers text to a WhatsApp id (the phone number in international
// form, digits only), with up to three reply buttons.
func (c Client) Send(ctx context.Context, to, text string, buttons ...Button) error {
	msg := map[string]any{"messaging_product": "whatsapp", "recipient_type": "individual", "to": digits(to)}
	if len(buttons) == 0 {
		msg["type"] = "text"
		msg["text"] = map[string]any{"body": cut(text, 4096), "preview_url": false}
	} else {
		if len(buttons) > 3 {
			buttons = buttons[:3]
		}
		var bs []map[string]any
		for _, b := range buttons {
			bs = append(bs, map[string]any{"type": "reply", "reply": map[string]string{"id": b.ID, "title": cut(b.Title, 20)}})
		}
		msg["type"] = "interactive"
		msg["interactive"] = map[string]any{"type": "button", "body": map[string]string{"text": cut(text, 1024)}, "action": map[string]any{"buttons": bs}}
	}
	body, _ := json.Marshal(msg)
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/%s/messages", c.base(), c.PhoneID), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&e)
		return fmt.Errorf("whatsapp: %s", firstNonEmpty(e.Error.Message, resp.Status))
	}
	return nil
}

// Inbound is one message someone sent to the business number.
type Inbound struct {
	// ID is Meta's message id (wamid…), the same on a retry or replay.
	ID string
	// Time is when the person sent it.
	Time time.Time
	From string
	Name string
	Text string
	// Button is the id of a reply button they tapped.
	Button string
}

var ErrSignature = errors.New("webhook signature does not match")

// Verify checks Meta's X-Hub-Signature-256 header against the app secret.
func Verify(body []byte, header, appSecret string) error {
	sig, ok := strings.CutPrefix(header, "sha256=")
	if !ok || appSecret == "" {
		return ErrSignature
	}
	want, err := hex.DecodeString(sig)
	if err != nil {
		return ErrSignature
	}
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(body)
	if !hmac.Equal(mac.Sum(nil), want) {
		return ErrSignature
	}
	return nil
}

// Parse reads the messages out of a webhook body; status updates and
// other events are skipped.
func Parse(body []byte) ([]Inbound, error) {
	var p struct {
		Entry []struct {
			Changes []struct {
				Value struct {
					Contacts []struct {
						WaID    string `json:"wa_id"`
						Profile struct {
							Name string `json:"name"`
						} `json:"profile"`
					} `json:"contacts"`
					Messages []struct {
						ID        string `json:"id"`
						Timestamp string `json:"timestamp"`
						From      string `json:"from"`
						Type      string `json:"type"`
						Text      struct {
							Body string `json:"body"`
						} `json:"text"`
						Interactive struct {
							ButtonReply struct {
								ID string `json:"id"`
							} `json:"button_reply"`
						} `json:"interactive"`
						Button struct {
							Payload string `json:"payload"`
						} `json:"button"`
					} `json:"messages"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	var out []Inbound
	for _, e := range p.Entry {
		for _, ch := range e.Changes {
			names := map[string]string{}
			for _, c := range ch.Value.Contacts {
				names[c.WaID] = c.Profile.Name
			}
			for _, m := range ch.Value.Messages {
				in := Inbound{ID: m.ID, From: m.From, Name: names[m.From]}
				if sec, err := strconv.ParseInt(m.Timestamp, 10, 64); err == nil {
					in.Time = time.Unix(sec, 0)
				}
				switch m.Type {
				case "text":
					in.Text = m.Text.Body
				case "interactive":
					in.Button = m.Interactive.ButtonReply.ID
				case "button":
					in.Button = m.Button.Payload
				default:
					continue
				}
				out = append(out, in)
			}
		}
	}
	return out, nil
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Normalize turns a typed phone number into a WhatsApp id.
func Normalize(phone string) string { return digits(phone) }

func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
