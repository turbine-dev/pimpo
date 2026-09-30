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
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Discord is a bot that answers direct messages through the Gateway.
type Discord struct {
	Token string
	// API is Discord's REST base; tests replace it.
	API string

	mu  sync.Mutex
	dms map[string]string // user id -> DM channel id
}

func (d *Discord) Name() string { return "discord" }

func (d *Discord) api() string {
	if d.API != "" {
		return d.API
	}
	return "https://discord.com/api/v10"
}

func (d *Discord) rest(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.api()+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+d.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Discord is unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == 401 {
		return errors.New("Discord refused the bot token")
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Discord answered %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func (d *Discord) Check(ctx context.Context) error {
	var me struct {
		ID string `json:"id"`
	}
	return d.rest(ctx, "GET", "/users/@me", nil, &me)
}

type gatewayMsg struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
	S  *int            `json:"s"`
	T  string          `json:"t"`
}

const directMessages = 1 << 12

func (d *Discord) Run(ctx context.Context, on func(Inbound)) error {
	var gw struct {
		URL string `json:"url"`
	}
	if err := d.rest(ctx, "GET", "/gateway/bot", nil, &gw); err != nil {
		return err
	}
	conn, _, err := websocket.Dial(ctx, gw.URL+"/?v=10&encoding=json", nil)
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4 << 20)
	var hello struct {
		Interval float64 `json:"heartbeat_interval"`
	}
	var first gatewayMsg
	if err := wsjson.Read(ctx, conn, &first); err != nil || first.Op != 10 {
		return fmt.Errorf("Discord did not say hello: %v", err)
	}
	json.Unmarshal(first.D, &hello)
	identify := map[string]any{"op": 2, "d": map[string]any{"token": d.Token, "intents": directMessages,
		"properties": map[string]string{"os": "pimpo", "browser": "pimpo", "device": "pimpo"}}}
	if err := wsjson.Write(ctx, conn, identify); err != nil {
		return err
	}
	var seqMu sync.Mutex
	var seq *int
	hctx, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		t := time.NewTicker(time.Duration(max(hello.Interval, 1000)) * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				seqMu.Lock()
				s := seq
				seqMu.Unlock()
				if wsjson.Write(hctx, conn, map[string]any{"op": 1, "d": s}) != nil {
					conn.CloseNow()
					return
				}
			case <-hctx.Done():
				return
			}
		}
	}()
	for {
		var m gatewayMsg
		if err := wsjson.Read(ctx, conn, &m); err != nil {
			return err
		}
		if m.S != nil {
			seqMu.Lock()
			seq = m.S
			seqMu.Unlock()
		}
		switch m.Op {
		case 7, 9:
			return errors.New("Discord asked to reconnect")
		case 0:
			if m.T != "MESSAGE_CREATE" {
				continue
			}
			var msg struct {
				ChannelID string `json:"channel_id"`
				GuildID   string `json:"guild_id"`
				Content   string `json:"content"`
				Reference *struct {
					MessageID string `json:"message_id"`
				} `json:"message_reference"`
				Author struct {
					ID  string `json:"id"`
					Bot bool   `json:"bot"`
				} `json:"author"`
			}
			json.Unmarshal(m.D, &msg)
			if msg.GuildID != "" || msg.Author.Bot || strings.TrimSpace(msg.Content) == "" {
				continue
			}
			d.mu.Lock()
			if d.dms == nil {
				d.dms = map[string]string{}
			}
			d.dms[msg.Author.ID] = msg.ChannelID
			d.mu.Unlock()
			in := Inbound{From: msg.Author.ID, Chat: msg.ChannelID, Text: msg.Content}
			if msg.Reference != nil {
				in.ReplyTo = msg.Reference.MessageID
			}
			on(in)
		}
	}
}

// dm is the private channel with a person, opened once.
func (d *Discord) dm(ctx context.Context, to string) (string, error) {
	d.mu.Lock()
	ch := d.dms[to]
	d.mu.Unlock()
	if ch != "" {
		return ch, nil
	}
	var dm struct {
		ID string `json:"id"`
	}
	if err := d.rest(ctx, "POST", "/users/@me/channels", map[string]string{"recipient_id": to}, &dm); err != nil {
		return "", err
	}
	d.mu.Lock()
	if d.dms == nil {
		d.dms = map[string]string{}
	}
	d.dms[to] = dm.ID
	d.mu.Unlock()
	return dm.ID, nil
}

// Typing shows "typing…" in the private channel for about ten seconds.
func (d *Discord) Typing(ctx context.Context, to string) error {
	ch, err := d.dm(ctx, to)
	if err != nil {
		return err
	}
	return d.rest(ctx, "POST", "/channels/"+ch+"/typing", map[string]string{}, nil)
}

func (d *Discord) Send(ctx context.Context, to, text string) error {
	_, err := d.SendMessage(ctx, to, text)
	return err
}

func (d *Discord) SendMessage(ctx context.Context, to, text string) ([]string, error) {
	ch, err := d.dm(ctx, to)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, part := range chunks(text, 1900) {
		var sent struct {
			ID string `json:"id"`
		}
		if err := d.rest(ctx, "POST", "/channels/"+ch+"/messages", map[string]string{"content": part}, &sent); err != nil {
			return ids, err
		}
		ids = append(ids, sent.ID)
	}
	return ids, nil
}

// chunks splits text for services that limit a message's length.
func chunks(text string, n int) []string {
	r := []rune(text)
	var out []string
	for len(r) > n {
		cut := n
		for i := n; i > n/2; i-- {
			if r[i] == '\n' {
				cut = i
				break
			}
		}
		out = append(out, string(r[:cut]))
		r = r[cut:]
	}
	return append(out, string(r))
}

// Edit changes a message Pimpo sent in the private channel.
func (d *Discord) Edit(ctx context.Context, to, id, text string) error {
	ch, err := d.dm(ctx, to)
	if err != nil {
		return err
	}
	return d.rest(ctx, "PATCH", "/channels/"+ch+"/messages/"+id, map[string]string{"content": chunks(text, 1900)[0]}, nil)
}
