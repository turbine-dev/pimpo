package external

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// A declarative connector needs no program: connector.json describes each
// capability as one HTTP request to a service's API, and Pimpo makes it.
//
//	"http": {"base": "https://api.example.com", "headers": {"Authorization": "Bearer {{env.EXAMPLE_KEY}}"}},
//	"capabilities": [{"name": "example.find", "risk": "read", …,
//	  "request": {"method": "GET", "path": "/v1/items/{{id}}", "query": {"q": "{{query}}"}},
//	  "result": {"path": "data.items", "fields": {"title": "name", "link": "html_url"}}}]
//
// {{name}} is an argument of the call and {{env.NAME}} a declared secret.
// Every request stays on the base's host, redirects too; private addresses
// are refused unless the base itself is this computer.

// HTTPSpec is the service a declarative connector calls.
type HTTPSpec struct {
	Base    string            `json:"base"`
	Headers map[string]string `json:"headers,omitempty"`
	Query   map[string]string `json:"query,omitempty"`
}

// Request is one capability's call.
type Request struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Query   map[string]string `json:"query,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	// Body is JSON sent as is, with "{{arg}}" strings replaced by the
	// argument's value (keeping its type) and {{…}} inside longer strings
	// filled in as text.
	Body any `json:"body,omitempty"`
	// Form sends the body as form fields instead of JSON.
	Form bool `json:"form,omitempty"`
	// Safe says a request other than GET only reads (a search sent as
	// POST), so the capability may be declared read.
	Safe bool `json:"safe,omitempty"`
}

// Result picks what the capability returns from the response.
type Result struct {
	// Path is a dot path to the part to return (data.items); empty is all.
	Path string `json:"path,omitempty"`
	// Fields keeps only these, named as the keys, taken from each object
	// (or each item of a list) by dot path; labels.*.name collects a field
	// from every item of a nested list.
	Fields map[string]string `json:"fields,omitempty"`
	// Max caps a list.
	Max int `json:"max,omitempty"`
}

const httpMaxBody = 5 << 20

var placeholder = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_.]*)\s*\}\}`)

func (h *HTTPSpec) validate(caps []Capability, env []string) error {
	u, err := url.Parse(h.Base)
	if err != nil || u.Host == "" {
		return errors.New("http.base must be an address like https://api.example.com")
	}
	if u.Scheme != "https" && !isLoopback(u.Hostname()) {
		return errors.New("http.base must be https (http only for this computer)")
	}
	if strings.Contains(h.Base, "{{") {
		return errors.New("http.base may not have placeholders")
	}
	declared := map[string]bool{}
	for _, e := range env {
		declared[e] = true
	}
	check := func(where, s string) error {
		for _, m := range placeholder.FindAllStringSubmatch(s, -1) {
			if name, ok := strings.CutPrefix(m[1], "env."); ok && !declared[name] {
				return fmt.Errorf("%s uses {{env.%s}}, which is not in env", where, name)
			}
		}
		return nil
	}
	for k, v := range h.Headers {
		if err := check("http.headers."+k, v); err != nil {
			return err
		}
	}
	for k, v := range h.Query {
		if err := check("http.query."+k, v); err != nil {
			return err
		}
	}
	for _, c := range caps {
		r := c.Request
		if r == nil {
			return fmt.Errorf("capability %q needs a request", c.Name)
		}
		switch strings.ToUpper(r.Method) {
		case "GET", "POST", "PUT", "PATCH", "DELETE":
		default:
			return fmt.Errorf("capability %q: method must be GET, POST, PUT, PATCH or DELETE", c.Name)
		}
		if strings.ToUpper(r.Method) != "GET" && risks[c.Risk] == risks["read"] && !r.Safe {
			return fmt.Errorf("capability %q uses %s, which changes things; its risk can be read only with \"safe\": true in its request", c.Name, r.Method)
		}
		if !strings.HasPrefix(r.Path, "/") || strings.Contains(r.Path, "://") || strings.Contains(r.Path, "..") {
			return fmt.Errorf("capability %q: path must start with / and stay on the base's host", c.Name)
		}
		if strings.Contains(r.Path, "{{env.") {
			return fmt.Errorf("capability %q: secrets may not go in the path", c.Name)
		}
		all := r.Path
		for _, v := range r.Query {
			all += v
		}
		for _, v := range r.Headers {
			all += v
		}
		b, _ := json.Marshal(r.Body)
		all += string(b)
		if err := check("capability "+c.Name, all); err != nil {
			return err
		}
	}
	return nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// fill replaces placeholders in s; path values are escaped for a URL path.
func fill(s string, args map[string]any, env map[string]string, path bool) (string, error) {
	var missing []string
	out := placeholder.ReplaceAllStringFunc(s, func(p string) string {
		name := placeholder.FindStringSubmatch(p)[1]
		if e, ok := strings.CutPrefix(name, "env."); ok {
			return env[e]
		}
		v, ok := lookup(args, name)
		if !ok || v == nil {
			missing = append(missing, name)
			return ""
		}
		text := scalar(v)
		if path {
			return url.PathEscape(text)
		}
		return text
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("missing %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// unset says whether s uses a secret that has no value.
func unset(s string, env map[string]string) bool {
	for _, m := range placeholder.FindAllStringSubmatch(s, -1) {
		if name, ok := strings.CutPrefix(m[1], "env."); ok && env[name] == "" {
			return true
		}
	}
	return false
}

func scalar(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// fillBody fills a JSON body: a string that is only "{{x}}" becomes x's
// value with its type; optional values left out drop the field.
func fillBody(v any, args map[string]any, env map[string]string) (any, error) {
	switch x := v.(type) {
	case string:
		if m := placeholder.FindStringSubmatch(x); m != nil && m[0] == strings.TrimSpace(x) && !strings.HasPrefix(m[1], "env.") {
			val, _ := lookup(args, m[1])
			return val, nil
		}
		return fill(x, args, env, false)
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			f, err := fillBody(e, args, env)
			if err != nil {
				return nil, err
			}
			if f != nil {
				out[k] = f
			}
		}
		return out, nil
	case []any:
		out := []any{}
		for _, e := range x {
			f, err := fillBody(e, args, env)
			if err != nil {
				return nil, err
			}
			if f != nil {
				out = append(out, f)
			}
		}
		return out, nil
	}
	return v, nil
}

// lookup follows a dot path through maps and lists; * takes the rest of
// the path from every item of a list (labels.*.name).
func lookup(v any, path string) (any, bool) {
	if path == "" {
		return v, true
	}
	cur := v
	parts := strings.Split(path, ".")
	for i, part := range parts {
		if part == "*" {
			list, ok := cur.([]any)
			if !ok {
				return nil, false
			}
			rest := strings.Join(parts[i+1:], ".")
			out := []any{}
			for _, it := range list {
				if got, ok := lookup(it, rest); ok {
					out = append(out, got)
				}
			}
			return out, true
		}
		switch x := cur.(type) {
		case map[string]any:
			n, ok := x[part]
			if !ok {
				return nil, false
			}
			cur = n
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(x) {
				return nil, false
			}
			cur = x[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// shape applies a capability's result rules.
func (r *Result) shape(v any) any {
	if r == nil {
		return v
	}
	if r.Path != "" {
		v, _ = lookup(v, r.Path)
	}
	pick := func(item any) any {
		if len(r.Fields) == 0 {
			return item
		}
		out := map[string]any{}
		for name, p := range r.Fields {
			out[name], _ = lookup(item, p)
		}
		return out
	}
	if list, ok := v.([]any); ok {
		if r.Max > 0 && len(list) > r.Max {
			list = list[:r.Max]
		}
		out := make([]any, len(list))
		for i, it := range list {
			out[i] = pick(it)
		}
		return out
	}
	return pick(v)
}

// httpClient keeps every request on the base's host and off private
// addresses (unless the base is this computer).
func httpClient(base *url.URL, timeout time.Duration) *http.Client {
	local := isLoopback(base.Hostname())
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, _ := net.SplitHostPort(address)
		ip := net.ParseIP(host)
		if local {
			if ip == nil || !ip.IsLoopback() {
				return fmt.Errorf("refusing %s: this connector only reaches this computer", host)
			}
			return nil
		}
		if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return fmt.Errorf("refusing to connect to private address %s", host)
		}
		return nil
	}}
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: timeout},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !strings.EqualFold(req.URL.Hostname(), base.Hostname()) {
				return fmt.Errorf("redirect to %s is outside %s", req.URL.Hostname(), base.Hostname())
			}
			if len(via) > 5 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
}

// callHTTP makes one declarative capability's request.
func (c *Connector) callHTTP(ctx context.Context, name string, args any) (any, error) {
	var capDef *Capability
	for i := range c.Manifest.Capabilities {
		if c.Manifest.Capabilities[i].Name == name {
			capDef = &c.Manifest.Capabilities[i]
		}
	}
	if capDef == nil {
		return nil, fmt.Errorf("%s is not declared", name)
	}
	a, _ := args.(map[string]any)
	if a == nil {
		b, _ := json.Marshal(args)
		json.Unmarshal(b, &a)
	}
	env := c.values(ctx, c.Env)
	h, r := c.Manifest.HTTP, capDef.Request
	base, _ := url.Parse(strings.TrimRight(h.Base, "/"))
	p, err := fill(r.Path, a, env, true)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	u := *base
	u.RawPath = strings.TrimRight(base.EscapedPath(), "/") + p
	if u.Path, err = url.PathUnescape(u.RawPath); err != nil {
		return nil, fmt.Errorf("%s: bad path", name)
	}
	q := u.Query()
	for _, m := range []map[string]string{h.Query, r.Query} {
		for k, v := range m {
			val, err := fill(v, a, env, false)
			if err != nil || unset(v, env) {
				continue // an optional argument left out drops its parameter
			}
			if val != "" {
				q.Set(k, val)
			}
		}
	}
	u.RawQuery = q.Encode()
	var body io.Reader
	contentType := ""
	if r.Body != nil {
		filled, err := fillBody(r.Body, a, env)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if filled == nil {
			// the whole body was an argument that was not given
		} else if r.Form {
			form := url.Values{}
			if m, ok := filled.(map[string]any); ok {
				for k, v := range m {
					form.Set(k, scalar(v))
				}
			}
			body, contentType = strings.NewReader(form.Encode()), "application/x-www-form-urlencoded"
		} else {
			b, _ := json.Marshal(filled)
			body, contentType = bytes.NewReader(b), "application/json"
		}
	}
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(r.Method), u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Pimpo/1")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for _, m := range []map[string]string{h.Headers, r.Headers} {
		for k, v := range m {
			if unset(v, env) {
				continue // an optional key not filled in drops its header
			}
			val, _ := fill(v, a, env, false)
			req.Header.Set(k, val)
		}
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	resp, err := httpClient(base, timeout).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %s is unreachable: %w", name, base.Hostname(), errors.Unwrap(err))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, httpMaxBody+1))
	if len(raw) > httpMaxBody {
		return nil, fmt.Errorf("%s: the answer is larger than 5 MB", name)
	}
	if resp.StatusCode/100 != 2 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		switch resp.StatusCode {
		case 401, 403:
			return nil, fmt.Errorf("%s: %s refused the key (%d)", name, base.Hostname(), resp.StatusCode)
		case 429:
			return nil, fmt.Errorf("%s: %s says too many requests; try later", name, base.Hostname())
		}
		return nil, fmt.Errorf("%s: %s answered %d: %s", name, base.Hostname(), resp.StatusCode, msg)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{"ok": true}, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		text := string(raw)
		if len(text) > 20000 {
			text = text[:20000]
		}
		return map[string]any{"text": text}, nil
	}
	return capDef.Result.shape(v), nil
}
