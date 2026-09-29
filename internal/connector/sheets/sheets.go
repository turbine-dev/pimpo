// Package sheets reads and appends to the owner's Google Sheets with the
// Google sign-in they already use.
package sheets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/connector"
)

type Sheets struct {
	Token func(ctx context.Context) (string, error)
	// Granted says whether the owner allowed Scope when signing in.
	Granted func(ctx context.Context) bool
	// API replaces Google's address; tests only.
	API  string
	HTTP *http.Client
}

func (s *Sheets) Capabilities() []string {
	return []string{"sheets.read", "sheets.append", "sheets.clear"}
}

var idInURL = regexp.MustCompile(`/spreadsheets/d/([a-zA-Z0-9_-]{20,})`)
var plainID = regexp.MustCompile(`^[a-zA-Z0-9_-]{20,}$`)

// sheetID takes a spreadsheet's id or its address.
func sheetID(v string) (string, error) {
	v = strings.TrimSpace(v)
	if m := idInURL.FindStringSubmatch(v); m != nil {
		return m[1], nil
	}
	if plainID.MatchString(v) {
		return v, nil
	}
	return "", errors.New("sheet is the spreadsheet's address or id")
}

func (s *Sheets) Call(ctx context.Context, name, _ string, args any) (any, error) {
	var in struct {
		Sheet  string  `json:"sheet"`
		Range  string  `json:"range"`
		Values [][]any `json:"values"`
	}
	if err := connector.Args(args, &in); err != nil {
		return nil, err
	}
	if s.Granted != nil && !s.Granted(ctx) {
		return nil, errors.New("Pimpo may not use your spreadsheets yet: reconnect Google in Connections and allow Sheets")
	}
	id, err := sheetID(in.Sheet)
	if err != nil {
		return nil, err
	}
	rng := strings.TrimSpace(in.Range)
	if rng == "" {
		rng = "A:Z"
	}
	base := s.API
	if base == "" {
		base = "https://sheets.googleapis.com/v4"
	}
	values := base + "/spreadsheets/" + url.PathEscape(id) + "/values/" + url.PathEscape(rng)
	switch name {
	case "sheets.read":
		var r struct {
			Range  string  `json:"range"`
			Values [][]any `json:"values"`
		}
		if err := s.do(ctx, http.MethodGet, values+"?valueRenderOption=FORMATTED_VALUE", nil, &r); err != nil {
			return nil, err
		}
		if len(r.Values) > 2000 {
			r.Values = r.Values[:2000]
		}
		if r.Values == nil {
			r.Values = [][]any{}
		}
		return map[string]any{"range": r.Range, "values": r.Values}, nil
	case "sheets.append":
		if len(in.Values) == 0 || len(in.Values) > 500 {
			return nil, errors.New("values are 1 to 500 rows, each a list of cells")
		}
		var r struct {
			Updates struct {
				UpdatedRange string `json:"updatedRange"`
				UpdatedRows  int    `json:"updatedRows"`
			} `json:"updates"`
		}
		body := map[string]any{"values": in.Values}
		if err := s.do(ctx, http.MethodPost, values+":append?valueInputOption=USER_ENTERED&insertDataOption=INSERT_ROWS", body, &r); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "range": r.Updates.UpdatedRange, "rows": r.Updates.UpdatedRows,
			"undo": map[string]any{"capability": "sheets.clear", "args": map[string]any{"sheet": id, "range": r.Updates.UpdatedRange}}}, nil
	case "sheets.clear":
		if strings.TrimSpace(in.Range) == "" {
			return nil, errors.New("sheets.clear needs the exact range")
		}
		if err := s.do(ctx, http.MethodPost, values+":clear", map[string]any{}, nil); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	}
	return nil, fmt.Errorf("unknown capability %s", name)
}

func (s *Sheets) do(ctx context.Context, method, u string, body, out any) error {
	tok, err := s.Token(ctx)
	if err != nil {
		return err
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Google Sheets is unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error struct {
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		json.Unmarshal(raw, &e)
		switch {
		case strings.Contains(e.Error.Message, "has not been used in project") || strings.Contains(e.Error.Message, "is disabled"):
			return errors.New("turn on the Google Sheets API in your Google Cloud project, then try again")
		case resp.StatusCode == 403:
			return errors.New("Google refused: the spreadsheet is not shared with your account, or Pimpo may not use Sheets yet (reconnect Google in Connections)")
		case resp.StatusCode == 404:
			return errors.New("no such spreadsheet or range")
		}
		return fmt.Errorf("Google Sheets answered %d: %s", resp.StatusCode, e.Error.Message)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}
