package external

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Endpoint says how to reach an MCP server: a command to start, or a
// remote URL. Env and Headers carry the values themselves.
type Endpoint struct {
	Name    string
	Dir     string
	Command string
	Args    []string
	Env     map[string]string
	URL     string
	Headers map[string]string
}

type transport interface {
	rpc(ctx context.Context, method string, params any) (json.RawMessage, error)
	notify(method string)
	close()
}

// HTTP, when set, is used for remote servers; tests point it at fakes.
var HTTP *http.Client

// errBroken means the connection is unusable and must be reopened.
var errBroken = errors.New("connection lost")

const protocolVersion = "2025-06-18"

func dial(ctx context.Context, e Endpoint, timeout time.Duration) (transport, error) {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	var t transport
	if e.URL != "" {
		client := HTTP
		if client == nil {
			client = &http.Client{Timeout: timeout}
		}
		t = &httpConn{name: e.Name, url: e.URL, headers: e.Headers, timeout: timeout, client: client}
	} else {
		s, err := startProcess(e, timeout)
		if err != nil {
			return nil, err
		}
		t = s
	}
	if _, err := t.rpc(ctx, "initialize", map[string]any{"protocolVersion": protocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "pimpo", "version": "1"}}); err != nil {
		t.close()
		return nil, fmt.Errorf("%s did not initialize: %w", e.Name, err)
	}
	t.notify("notifications/initialized")
	return t, nil
}

// Tool is what an MCP server says about one of its tools.
type Tool struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	Annotations struct {
		ReadOnly    *bool `json:"readOnlyHint,omitempty"`
		Destructive *bool `json:"destructiveHint,omitempty"`
	} `json:"annotations"`
}

func listTools(ctx context.Context, t transport) ([]Tool, error) {
	var out []Tool
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := t.rpc(ctx, "tools/list", params)
		if err != nil {
			return nil, err
		}
		var page struct {
			Tools []Tool `json:"tools"`
			Next  string `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Tools...)
		if page.Next == "" || len(out) > 500 {
			return out, nil
		}
		cursor = page.Next
	}
}

// Probe connects to a server the owner is about to add and lists its tools.
func Probe(ctx context.Context, e Endpoint) ([]Tool, error) {
	t, err := dial(ctx, e, 60*time.Second)
	if err != nil {
		return nil, err
	}
	defer t.close()
	return listTools(ctx, t)
}

// SuggestedRisk reads the tool's own hints; without them the tool is
// treated as irreversible, the MCP default, so it asks before running.
func (t Tool) SuggestedRisk() string {
	switch {
	case t.Annotations.ReadOnly != nil && *t.Annotations.ReadOnly:
		return "read"
	case t.Annotations.Destructive != nil && !*t.Annotations.Destructive:
		return "reversible"
	}
	return "irreversible"
}

type rpcResponse struct {
	ID     *int            `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (r rpcResponse) value() (json.RawMessage, error) {
	if r.Error != nil {
		return nil, errors.New(r.Error.Message)
	}
	return r.Result, nil
}

// stdio: a child process with a clean environment.
type stdioConn struct {
	name    string
	timeout time.Duration
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	lines   *bufio.Scanner
	nextID  int
}

func startProcess(e Endpoint, timeout time.Duration) (*stdioConn, error) {
	cmd := exec.Command(e.Command, e.Args...)
	cmd.Dir = e.Dir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "LANG=C.UTF-8"}
	for k, v := range e.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s: %w", e.Name, err)
	}
	s := &stdioConn{name: e.Name, timeout: timeout, cmd: cmd, stdin: in, lines: bufio.NewScanner(out)}
	s.lines.Buffer(make([]byte, 1<<20), 16<<20)
	return s, nil
}

func (s *stdioConn) notify(method string) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method})
	s.stdin.Write(append(b, '\n'))
}

func (s *stdioConn) close() {
	if s.cmd != nil && s.cmd.Process != nil {
		s.stdin.Close()
		s.cmd.Process.Kill()
		s.cmd.Wait()
	}
	s.cmd = nil
}

func (s *stdioConn) rpc(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if s.cmd == nil {
		return nil, errBroken
	}
	s.nextID++
	id := s.nextID
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if _, err := s.stdin.Write(append(b, '\n')); err != nil {
		s.close()
		return nil, fmt.Errorf("%w: %v", errBroken, err)
	}
	type line struct {
		r   rpcResponse
		err error
	}
	got := make(chan line, 1)
	go func() {
		for s.lines.Scan() {
			var r rpcResponse
			if json.Unmarshal(s.lines.Bytes(), &r) == nil && r.ID != nil && *r.ID == id {
				got <- line{r: r}
				return
			}
		}
		err := s.lines.Err()
		if err == nil {
			err = io.EOF
		}
		got <- line{err: err}
	}()
	select {
	case l := <-got:
		if l.err != nil {
			s.close()
			return nil, fmt.Errorf("%s stopped: %w", s.name, errBroken)
		}
		return l.r.value()
	case <-time.After(s.timeout):
		s.close()
		return nil, fmt.Errorf("%s did not answer in %s: %w", s.name, s.timeout, errBroken)
	case <-ctx.Done():
		s.close()
		return nil, ctx.Err()
	}
}

// httpConn speaks MCP's streamable HTTP: one POST per message, answered
// with JSON or with a short event stream.
type httpConn struct {
	name    string
	url     string
	headers map[string]string
	timeout time.Duration
	client  *http.Client

	mu      sync.Mutex
	session string
	nextID  int
}

func (h *httpConn) post(ctx context.Context, msg map[string]any) (*http.Response, error) {
	b, _ := json.Marshal(msg)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	for k, v := range h.headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	h.mu.Lock()
	if h.session != "" {
		req.Header.Set("Mcp-Session-Id", h.session)
	}
	h.mu.Unlock()
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s is unreachable: %w", h.name, err)
	}
	if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
		h.mu.Lock()
		h.session = id
		h.mu.Unlock()
	}
	return resp, nil
}

func (h *httpConn) notify(method string) {
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()
	if resp, err := h.post(ctx, map[string]any{"jsonrpc": "2.0", "method": method}); err == nil {
		resp.Body.Close()
	}
}

func (h *httpConn) close() {
	h.mu.Lock()
	session := h.session
	h.session = ""
	h.mu.Unlock()
	if session == "" {
		return
	}
	req, _ := http.NewRequest(http.MethodDelete, h.url, nil)
	req.Header.Set("Mcp-Session-Id", session)
	for k, v := range h.headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	if resp, err := h.client.Do(req); err == nil {
		resp.Body.Close()
	}
}

func (h *httpConn) rpc(ctx context.Context, method string, params any) (json.RawMessage, error) {
	h.mu.Lock()
	h.nextID++
	id := h.nextID
	h.mu.Unlock()
	resp, err := h.post(ctx, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound && method != "initialize":
		return nil, fmt.Errorf("%s ended the session: %w", h.name, errBroken)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%s refused the key (%d); check the headers", h.name, resp.StatusCode)
	case resp.StatusCode/100 != 2:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("%s answered %d: %s", h.name, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return readEvents(resp.Body, id)
	}
	var r rpcResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("%s: unreadable answer: %w", h.name, err)
	}
	return r.value()
}

// readEvents reads server-sent events until the answer to id arrives.
func readEvents(body io.Reader, id int) (json.RawMessage, error) {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 1<<20), 32<<20)
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
		var r rpcResponse
		if json.Unmarshal([]byte(data.String()), &r) == nil && r.ID != nil && *r.ID == id {
			return r.value()
		}
		data.Reset()
	}
	return nil, fmt.Errorf("the event stream ended without an answer: %w", errBroken)
}
