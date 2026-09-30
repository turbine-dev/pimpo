package chatlink

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Slack answers direct messages to a Slack app through Socket Mode: no
// public address needed.
type Slack struct {
	// BotToken (xoxb-) reads and writes messages; AppToken (xapp-) opens
	// the socket.
	BotToken string
	AppToken string
	API      string
	// OnEvent, when set, hears messages in the channels the app is in
	// and mentions of it, for routines that watch Slack.
	OnEvent func(Event)

	mu  sync.Mutex
	dms map[string]string
}

// Event is a message in a Slack channel the app is in, or a mention of
// the app. Its text is someone else's words: data for routines.
type Event struct {
	Channel  string
	User     string
	Text     string
	TS       string
	ThreadTS string
	Mention  bool
}

func (s *Slack) Name() string { return "slack" }

func (s *Slack) api() string {
	if s.API != "" {
		return s.API
	}
	return "https://slack.com/api"
}

func (s *Slack) call(ctx context.Context, method, token string, body map[string]any, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", s.api()+"/"+method, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Slack is unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var ok struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	json.Unmarshal(raw, &ok)
	if !ok.OK {
		if ok.Error == "invalid_auth" || ok.Error == "not_authed" {
			return errors.New("Slack refused the token")
		}
		return fmt.Errorf("Slack refused %s: %s", method, ok.Error)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func (s *Slack) Check(ctx context.Context) error {
	if err := s.call(ctx, "auth.test", s.BotToken, map[string]any{}, nil); err != nil {
		return fmt.Errorf("bot token: %w", err)
	}
	if err := s.call(ctx, "apps.connections.open", s.AppToken, map[string]any{}, nil); err != nil {
		return fmt.Errorf("app token (connections:write, Socket Mode on): %w", err)
	}
	return nil
}

func (s *Slack) Run(ctx context.Context, on func(Inbound)) error {
	var open struct {
		URL string `json:"url"`
	}
	if err := s.call(ctx, "apps.connections.open", s.AppToken, map[string]any{}, &open); err != nil {
		return err
	}
	conn, _, err := websocket.Dial(ctx, open.URL, nil)
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4 << 20)
	for {
		var env struct {
			Type       string `json:"type"`
			EnvelopeID string `json:"envelope_id"`
			Payload    struct {
				Event struct {
					Type        string `json:"type"`
					ChannelType string `json:"channel_type"`
					Channel     string `json:"channel"`
					User        string `json:"user"`
					Text        string `json:"text"`
					ThreadTS    string `json:"thread_ts"`
					BotID       string `json:"bot_id"`
					Subtype     string `json:"subtype"`
					TS          string `json:"ts"`
				} `json:"event"`
			} `json:"payload"`
		}
		if err := wsjson.Read(ctx, conn, &env); err != nil {
			return err
		}
		if env.EnvelopeID != "" {
			if err := wsjson.Write(ctx, conn, map[string]string{"envelope_id": env.EnvelopeID}); err != nil {
				return err
			}
		}
		switch env.Type {
		case "disconnect":
			return errors.New("Slack asked to reconnect")
		case "events_api":
			e := env.Payload.Event
			if e.BotID != "" || e.Subtype != "" || strings.TrimSpace(e.Text) == "" {
				continue
			}
			if e.Type == "app_mention" || (e.Type == "message" && (e.ChannelType == "channel" || e.ChannelType == "group")) {
				if s.OnEvent != nil {
					s.OnEvent(Event{Channel: e.Channel, User: e.User, Text: e.Text, TS: e.TS, ThreadTS: e.ThreadTS, Mention: e.Type == "app_mention"})
				}
				continue
			}
			if e.Type != "message" || e.ChannelType != "im" {
				continue
			}
			s.mu.Lock()
			if s.dms == nil {
				s.dms = map[string]string{}
			}
			s.dms[e.User] = e.Channel
			s.mu.Unlock()
			// A reply in a notice's thread names the notice by its ts.
			on(Inbound{From: e.User, Chat: e.Channel, Text: e.Text, ReplyTo: e.ThreadTS})
		}
	}
}

func (s *Slack) Send(ctx context.Context, to, text string) error {
	_, err := s.SendMessage(ctx, to, text)
	return err
}

func (s *Slack) SendMessage(ctx context.Context, to, text string) ([]string, error) {
	s.mu.Lock()
	ch := s.dms[to]
	s.mu.Unlock()
	if ch == "" {
		var open struct {
			Channel struct {
				ID string `json:"id"`
			} `json:"channel"`
		}
		if err := s.call(ctx, "conversations.open", s.BotToken, map[string]any{"users": to}, &open); err != nil {
			return nil, err
		}
		ch = open.Channel.ID
	}
	var sent struct {
		TS string `json:"ts"`
	}
	if err := s.call(ctx, "chat.postMessage", s.BotToken, map[string]any{"channel": ch, "text": text}, &sent); err != nil {
		return nil, err
	}
	return []string{sent.TS}, nil
}
