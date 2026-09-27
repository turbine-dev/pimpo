package chatlink

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
)

// Signal talks through a signal-cli daemon on this machine
// (signal-cli -a +NUMBER daemon --http 127.0.0.1:8080), so messages stay
// end-to-end encrypted up to this computer.
type Signal struct {
	URL string
	// Account is the daemon's number, needed when it serves several.
	Account string

	id atomic.Int64
}

func (s *Signal) Name() string { return "signal" }

func (s *Signal) base() string {
	if s.URL == "" {
		return "http://127.0.0.1:8080"
	}
	return strings.TrimRight(s.URL, "/")
}

func (s *Signal) rpc(ctx context.Context, method string, params map[string]any, out any) error {
	if s.Account != "" {
		params["account"] = s.Account
	}
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": s.id.Add(1), "method": method, "params": params})
	req, err := http.NewRequestWithContext(ctx, "POST", s.base()+"/api/v1/rpc", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("signal-cli is not answering at %s; start it with: signal-cli -a +NUMBER daemon --http 127.0.0.1:8080", s.base())
	}
	defer resp.Body.Close()
	var r struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return fmt.Errorf("signal-cli answered %d", resp.StatusCode)
	}
	if r.Error != nil {
		return errors.New("signal-cli: " + r.Error.Message)
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

func (s *Signal) Check(ctx context.Context) error {
	return s.rpc(ctx, "version", map[string]any{}, nil)
}

func (s *Signal) Send(ctx context.Context, to, text string) error {
	return s.rpc(ctx, "send", map[string]any{"recipient": []string{to}, "message": text}, nil)
}

// Typing shows the typing indicator to a person.
func (s *Signal) Typing(ctx context.Context, to string) error {
	return s.rpc(ctx, "sendTyping", map[string]any{"recipient": []string{to}}, nil)
}

// Run reads the daemon's event stream.
func (s *Signal) Run(ctx context.Context, on func(Inbound)) error {
	req, err := http.NewRequestWithContext(ctx, "GET", s.base()+"/api/v1/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("signal-cli events answered %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	var data strings.Builder
	for sc.Scan() {
		line := sc.Text()
		if rest, ok := strings.CutPrefix(line, "data:"); ok {
			data.WriteString(strings.TrimPrefix(rest, " "))
			continue
		}
		if line != "" || data.Len() == 0 {
			continue
		}
		var ev struct {
			Method string `json:"method"`
			Params struct {
				Envelope struct {
					Source       string `json:"source"`
					SourceNumber string `json:"sourceNumber"`
					DataMessage  *struct {
						Message string `json:"message"`
					} `json:"dataMessage"`
				} `json:"envelope"`
			} `json:"params"`
		}
		if json.Unmarshal([]byte(data.String()), &ev) == nil && ev.Params.Envelope.DataMessage != nil {
			from := ev.Params.Envelope.SourceNumber
			if from == "" {
				from = ev.Params.Envelope.Source
			}
			if text := strings.TrimSpace(ev.Params.Envelope.DataMessage.Message); text != "" && from != "" {
				on(Inbound{From: from, Text: text})
			}
		}
		data.Reset()
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return io.EOF
}
