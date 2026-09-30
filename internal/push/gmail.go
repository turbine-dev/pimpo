package push

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GmailAPI is Gmail's REST address.
const GmailAPI = "https://gmail.googleapis.com/gmail/v1"

// Gmail asks Gmail to push changes of one mailbox to a Pub/Sub topic and
// reads what changed. Token is that mailbox's OAuth access token.
type Gmail struct {
	// API replaces GmailAPI; tests only.
	API   string
	HTTP  *http.Client
	Token func(ctx context.Context) (string, error)
}

// Watching is what Gmail answers when a watch starts or is renewed.
type Watching struct {
	HistoryID uint64
	Expires   time.Time
}

// ErrHistoryGone means Gmail no longer has changes that old; the caller
// checks the mailbox as a poll would.
var ErrHistoryGone = errors.New("Gmail no longer has that history")

// Watch starts, or renews, the push of the inbox's changes to topic
// (projects/<project>/topics/<topic>). A watch lasts about 7 days.
func (g Gmail) Watch(ctx context.Context, topic string) (Watching, error) {
	var out struct {
		HistoryID  flexInt `json:"historyId"`
		Expiration flexInt `json:"expiration"`
	}
	body := map[string]any{"topicName": topic, "labelIds": []string{"INBOX"}, "labelFilterBehavior": "include"}
	if err := g.do(ctx, http.MethodPost, "/users/me/watch", body, &out); err != nil {
		return Watching{}, err
	}
	return Watching{HistoryID: uint64(out.HistoryID), Expires: time.UnixMilli(int64(out.Expiration))}, nil
}

// Stop ends the mailbox's push.
func (g Gmail) Stop(ctx context.Context) error {
	return g.do(ctx, http.MethodPost, "/users/me/stop", map[string]any{}, nil)
}

// Added lists the ids of messages that reached the inbox after since, and
// the mailbox's latest history id.
func (g Gmail) Added(ctx context.Context, since uint64) ([]string, uint64, error) {
	var ids []string
	var latest uint64
	page := ""
	for range 5 {
		q := url.Values{"startHistoryId": {strconv.FormatUint(since, 10)}, "historyTypes": {"messageAdded"}, "labelId": {"INBOX"}}
		if page != "" {
			q.Set("pageToken", page)
		}
		var out struct {
			History []struct {
				MessagesAdded []struct {
					Message struct {
						ID string `json:"id"`
					} `json:"message"`
				} `json:"messagesAdded"`
			} `json:"history"`
			HistoryID     flexInt `json:"historyId"`
			NextPageToken string  `json:"nextPageToken"`
		}
		if err := g.do(ctx, http.MethodGet, "/users/me/history?"+q.Encode(), nil, &out); err != nil {
			return nil, 0, err
		}
		for _, h := range out.History {
			for _, m := range h.MessagesAdded {
				ids = append(ids, m.Message.ID)
			}
		}
		latest = max(latest, uint64(out.HistoryID))
		if page = out.NextPageToken; page == "" {
			break
		}
	}
	return ids, latest, nil
}

func (g Gmail) do(ctx context.Context, method, path string, body, out any) error {
	tok, err := g.Token(ctx)
	if err != nil {
		return err
	}
	api := g.API
	if api == "" {
		api = GmailAPI
	}
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(api, "/")+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	client := g.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("Gmail is unreachable")
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode == 404 && strings.Contains(path, "/history") {
		return ErrHistoryGone
	}
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal(raw, &e)
		return fmt.Errorf("Gmail refused (%d): %s", resp.StatusCode, e.Error.Message)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// Notification is what one Pub/Sub push from Gmail says: whose mailbox
// changed and up to which history id.
type Notification struct {
	Email     string
	HistoryID uint64
	MessageID string
}

// ParseNotification reads a Pub/Sub push body.
func ParseNotification(body []byte) (Notification, error) {
	var env struct {
		Message struct {
			Data      string `json:"data"`
			MessageID string `json:"messageId"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return Notification{}, errors.New("not a Pub/Sub push")
	}
	data, err := base64.StdEncoding.DecodeString(env.Message.Data)
	if err != nil {
		if data, err = base64.URLEncoding.DecodeString(env.Message.Data); err != nil {
			return Notification{}, errors.New("not a Pub/Sub push")
		}
	}
	var n struct {
		Email     string  `json:"emailAddress"`
		HistoryID flexInt `json:"historyId"`
	}
	if err := json.Unmarshal(data, &n); err != nil || n.Email == "" || n.HistoryID == 0 {
		return Notification{}, errors.New("not a Gmail notification")
	}
	return Notification{Email: strings.ToLower(n.Email), HistoryID: uint64(n.HistoryID), MessageID: env.Message.MessageID}, nil
}

// flexInt reads a number Google sends either bare or as a string.
type flexInt uint64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	n, err := strconv.ParseUint(s, 10, 64)
	*f = flexInt(n)
	return err
}
