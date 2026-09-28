// Package telegram is a small client for the Telegram Bot API: messages,
// inline buttons, and long polling for replies and button taps.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"
)

type Bot struct {
	Token string
	// BaseURL defaults to https://api.telegram.org and is replaced in tests.
	BaseURL string
	HTTP    *http.Client
	// Health hears how each poll went (nil when it worked), so a bot that
	// keeps failing does not go unnoticed.
	Health func(err error)
}

type Button struct {
	Text string `json:"text"`
	Data string `json:"callback_data,omitempty"`
	URL  string `json:"url,omitempty"`
}

type Message struct {
	ID   int64  `json:"message_id"`
	Text string `json:"text"`
	// Photo holds the sizes of a photo, smallest first.
	Photo []struct {
		FileID string `json:"file_id"`
		Width  int    `json:"width"`
	} `json:"photo,omitempty"`
	Caption string `json:"caption,omitempty"`
	// Voice is set for voice notes.
	Voice *struct {
		FileID   string `json:"file_id"`
		Duration int    `json:"duration"`
	} `json:"voice,omitempty"`
	Chat struct {
		ID    int64  `json:"id"`
		Title string `json:"title,omitempty"`
	} `json:"chat"`
	From struct {
		ID        int64  `json:"id"`
		FirstName string `json:"first_name"`
		Username  string `json:"username"`
	} `json:"from"`
}

type Callback struct {
	ID      string   `json:"id"`
	Data    string   `json:"data"`
	Message *Message `json:"message"`
	From    struct {
		ID int64 `json:"id"`
	} `json:"from"`
}

type Update struct {
	ID       int64     `json:"update_id"`
	Message  *Message  `json:"message"`
	Callback *Callback `json:"callback_query"`
}

func (b Bot) call(ctx context.Context, method string, body, out any) error {
	base := b.BaseURL
	if base == "" {
		base = "https://api.telegram.org"
	}
	client := b.HTTP
	if client == nil {
		client = &http.Client{Timeout: 70 * time.Second}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/bot"+b.Token+"/"+method, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		// The URL contains the token; never let it reach logs.
		var uerr interface{ Unwrap() error }
		if errors.As(err, &uerr) {
			return fmt.Errorf("telegram %s: %w", method, uerr.Unwrap())
		}
		return fmt.Errorf("telegram %s failed", method)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("telegram %s: unreadable response (%d)", method, resp.StatusCode)
	}
	if !env.OK {
		return fmt.Errorf("telegram %s: %s", method, env.Description)
	}
	if out != nil {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

func (b Bot) Me(ctx context.Context) (username string, err error) {
	var me struct {
		Username string `json:"username"`
	}
	err = b.call(ctx, "getMe", map[string]any{}, &me)
	return me.Username, err
}

// Send posts a message with optional rows of inline buttons.
// Typing shows "typing…" in a chat for about five seconds.
func (b Bot) Typing(ctx context.Context, chat int64) error {
	return b.call(ctx, "sendChatAction", map[string]any{"chat_id": chat, "action": "typing"}, nil)
}

func (b Bot) Send(ctx context.Context, chat int64, text string, rows ...[]Button) (Message, error) {
	body := map[string]any{"chat_id": chat, "text": text, "disable_web_page_preview": true}
	if len(rows) > 0 {
		body["reply_markup"] = map[string]any{"inline_keyboard": rows}
	}
	var m Message
	err := b.call(ctx, "sendMessage", body, &m)
	return m, err
}

// Edit replaces a message's text and removes its buttons, used to show the
// outcome of an approval where the question was.
func (b Bot) Edit(ctx context.Context, chat, message int64, text string) error {
	return b.call(ctx, "editMessageText", map[string]any{"chat_id": chat, "message_id": message, "text": text}, nil)
}

func (b Bot) Answer(ctx context.Context, callbackID, text string) error {
	return b.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": callbackID, "text": text}, nil)
}

// Download fetches a file someone sent, such as a voice note, up to 20 MB.
func (b Bot) Download(ctx context.Context, fileID string) ([]byte, error) {
	var f struct {
		Path string `json:"file_path"`
	}
	if err := b.call(ctx, "getFile", map[string]any{"file_id": fileID}, &f); err != nil {
		return nil, err
	}
	base := b.BaseURL
	if base == "" {
		base = "https://api.telegram.org"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/file/bot"+b.Token+"/"+f.Path, nil)
	if err != nil {
		return nil, err
	}
	client := b.HTTP
	if client == nil {
		client = &http.Client{Timeout: 70 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("telegram file download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("telegram file download: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 20<<20))
}

// Poll long-polls for updates and calls handle for each until ctx ends.
// Network errors back off and retry.
func (b Bot) Poll(ctx context.Context, offset int64, handle func(Update)) error {
	backoff := time.Second
	for ctx.Err() == nil {
		var updates []Update
		err := b.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": 50, "allowed_updates": []string{"message", "callback_query"}}, &updates)
		if b.Health != nil && ctx.Err() == nil {
			b.Health(err)
		}
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
			}
			backoff = min(backoff*2, time.Minute)
			continue
		}
		backoff = time.Second
		for _, u := range updates {
			handle(u)
			offset = u.ID + 1
		}
	}
	return ctx.Err()
}

// SendAudio sends an audio file the chat shows with a player, its title
// and a caption.
func (b Bot) SendAudio(ctx context.Context, chat int64, filename string, audio []byte, title, caption string, seconds int) (Message, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("chat_id", strconv.FormatInt(chat, 10))
	if title != "" {
		mw.WriteField("title", title)
		mw.WriteField("performer", "Pimpo")
	}
	if caption != "" {
		if r := []rune(caption); len(r) > 1000 {
			caption = string(r[:999]) + "…"
		}
		mw.WriteField("caption", caption)
	}
	if seconds > 0 {
		mw.WriteField("duration", strconv.Itoa(seconds))
	}
	fw, err := mw.CreateFormFile("audio", filename)
	if err != nil {
		return Message{}, err
	}
	fw.Write(audio)
	mw.Close()
	base := b.BaseURL
	if base == "" {
		base = "https://api.telegram.org"
	}
	client := b.HTTP
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Minute}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/bot"+b.Token+"/sendAudio", &buf)
	if err != nil {
		return Message{}, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		var uerr interface{ Unwrap() error }
		if errors.As(err, &uerr) {
			return Message{}, fmt.Errorf("telegram sendAudio: %w", uerr.Unwrap())
		}
		return Message{}, errors.New("telegram sendAudio failed")
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env struct {
		OK          bool    `json:"ok"`
		Result      Message `json:"result"`
		Description string  `json:"description"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return Message{}, fmt.Errorf("telegram sendAudio: unreadable response (%d)", resp.StatusCode)
	}
	if !env.OK {
		return Message{}, fmt.Errorf("telegram sendAudio: %s", env.Description)
	}
	return env.Result, nil
}
