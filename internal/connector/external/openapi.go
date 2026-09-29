package external

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
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// OpenAPI import: read a service's OpenAPI 3 or Swagger 2 description
// (JSON or YAML) and turn the operations the owner picks into a
// declarative connector. Only local $refs are followed; operations that
// need a file upload or cookies are listed as unsupported.

// API is what an OpenAPI description offers.
type API struct {
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Base        string      `json:"base"`
	Keys        []APIKey    `json:"keys"`
	Operations  []Operation `json:"operations"`
	Unsupported []Skipped   `json:"unsupported"`

	headers map[string]string
	query   map[string]string
}

// APIKey is a secret the service asks for.
type APIKey struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Skipped is an operation Pimpo cannot call.
type Skipped struct {
	ID  string `json:"id"`
	Why string `json:"why"`
}

// Operation is one call the service offers, ready to become a capability.
type Operation struct {
	ID      string `json:"id"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Summary string `json:"summary"`
	// Risk is Pimpo's suggestion: GET reads, anything else asks first.
	Risk string `json:"risk"`

	signature string
	returns   string
	schema    json.RawMessage
	request   Request
	result    *Result
	contract  *Case
}

// MaxImported caps the capabilities one import may add.
const MaxImported = 100

const maxSpec = 30 << 20

// FetchSpec downloads an OpenAPI description from a public https address.
func FetchSpec(ctx context.Context, address string) ([]byte, error) {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !isLoopback(u.Hostname())) {
		return nil, errors.New("give the https address of the OpenAPI description (a .json or .yaml file)")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, application/yaml, text/yaml, */*")
	req.Header.Set("User-Agent", "Pimpo/1")
	resp, err := httpClient(u, 60*time.Second).Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", u.Hostname(), errors.Unwrap(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s answered %d", u.Hostname(), resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxSpec+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxSpec {
		return nil, errors.New("the description is larger than 30 MB")
	}
	return raw, nil
}

type specParser struct {
	root map[string]any
}

// ParseOpenAPI reads a description; specURL (may be empty) resolves a
// relative server address.
func ParseOpenAPI(raw []byte, specURL string) (*API, error) {
	var doc any
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		if err := json.Unmarshal(trimmed, &doc); err != nil {
			return nil, fmt.Errorf("not valid JSON: %w", err)
		}
	} else if err := yaml.Unmarshal(trimmed, &doc); err != nil {
		return nil, fmt.Errorf("not valid JSON or YAML: %w", err)
	}
	root, _ := normalize(doc).(map[string]any)
	if root == nil {
		return nil, errors.New("this is not an OpenAPI description")
	}
	swagger := str(root["swagger"]) != ""
	if !swagger && !strings.HasPrefix(str(root["openapi"]), "3") {
		return nil, errors.New("this is not an OpenAPI 3 or Swagger 2 description")
	}
	p := &specParser{root: root}
	info, _ := root["info"].(map[string]any)
	api := &API{Title: clean(str(info["title"]), 80), Description: clean(str(info["description"]), 400), Keys: []APIKey{}, Operations: []Operation{}, Unsupported: []Skipped{},
		headers: map[string]string{}, query: map[string]string{}}
	var err error
	if api.Base, err = p.base(swagger, specURL); err != nil {
		return nil, err
	}
	p.security(api, swagger)

	paths, _ := root["paths"].(map[string]any)
	var keys []string
	for k := range paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	seen := map[string]bool{}
	for _, path := range keys {
		item, _ := p.deref(paths[path]).(map[string]any)
		if item == nil {
			continue
		}
		shared, _ := item["parameters"].([]any)
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			op, _ := item[method].(map[string]any)
			if op == nil {
				continue
			}
			o, why := p.operation(method, path, op, shared, swagger, seen)
			if why != "" {
				api.Unsupported = append(api.Unsupported, Skipped{ID: o.ID, Why: why})
				continue
			}
			api.Operations = append(api.Operations, o)
		}
	}
	if len(api.Operations) == 0 {
		return nil, errors.New("the description has no operations Pimpo can call")
	}
	return api, nil
}

// normalize turns YAML's maps into JSON's.
func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = normalize(e)
		}
		return x
	case map[any]any:
		out := map[string]any{}
		for k, e := range x {
			out[fmt.Sprint(k)] = normalize(e)
		}
		return out
	case []any:
		for i, e := range x {
			x[i] = normalize(e)
		}
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case uint64:
		return float64(x)
	}
	return v
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return scalar(x)
	}
	return ""
}

var spaces = regexp.MustCompile(`\s+`)
var markdownLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

// clean flattens text to one line of at most n characters.
func clean(s string, n int) string {
	s = markdownLink.ReplaceAllString(s, "$1")
	s = strings.TrimSpace(spaces.ReplaceAllString(s, " "))
	if r := []rune(s); len(r) > n {
		s = strings.TrimSpace(string(r[:n-1])) + "…"
	}
	return s
}

// deref follows a local $ref (several in a row, at most 20).
func (p *specParser) deref(v any) any {
	for range 20 {
		m, ok := v.(map[string]any)
		if !ok {
			return v
		}
		ref, ok := m["$ref"].(string)
		if !ok {
			return v
		}
		if !strings.HasPrefix(ref, "#/") {
			return map[string]any{}
		}
		var cur any = p.root
		for _, part := range strings.Split(ref[2:], "/") {
			part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
			part, _ = url.PathUnescape(part)
			mm, _ := cur.(map[string]any)
			cur = mm[part]
		}
		if cur == nil {
			return map[string]any{}
		}
		v = cur
	}
	return map[string]any{}
}

func (p *specParser) base(swagger bool, specURL string) (string, error) {
	var raw string
	if swagger {
		host := str(p.root["host"])
		scheme := "https"
		if schemes, _ := p.root["schemes"].([]any); len(schemes) > 0 {
			scheme = str(schemes[0])
			for _, s := range schemes {
				if str(s) == "https" {
					scheme = "https"
				}
			}
		}
		if host != "" {
			raw = scheme + "://" + host
		}
		raw += str(p.root["basePath"])
	} else if servers, _ := p.root["servers"].([]any); len(servers) > 0 {
		for _, s := range servers {
			m, _ := s.(map[string]any)
			u := str(m["url"])
			vars, _ := m["variables"].(map[string]any)
			for name, v := range vars {
				vm, _ := v.(map[string]any)
				u = strings.ReplaceAll(u, "{"+name+"}", str(vm["default"]))
			}
			if raw == "" || strings.HasPrefix(u, "https://") && !strings.HasPrefix(raw, "https://") {
				raw = u
			}
		}
	}
	// A relative address (OpenAPI 3's default is "/") is relative to
	// where the description came from.
	if rel, err := url.Parse(raw); specURL != "" && err == nil && rel.Host == "" {
		if from, err := url.Parse(specURL); err == nil {
			if raw == "" {
				rel, _ = url.Parse("/")
			}
			raw = from.ResolveReference(rel).String()
		}
	}
	u, err := url.Parse(raw)
	if raw == "" || err != nil || u.Host == "" {
		return "", errors.New("the description does not say the service's address; give the address of the description itself, or edit http.base afterwards")
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		u.Scheme = "https"
	}
	u.RawQuery, u.Fragment = "", ""
	return strings.TrimRight(u.String(), "/"), nil
}

var envUnsafe = regexp.MustCompile(`[^A-Z0-9]+`)

func envName(scheme string) string {
	n := strings.Trim(envUnsafe.ReplaceAllString(strings.ToUpper(scheme), "_"), "_")
	if n == "" || n[0] < 'A' || n[0] > 'Z' {
		n = "KEY_" + n
	}
	if !strings.HasSuffix(n, "KEY") && !strings.HasSuffix(n, "TOKEN") {
		n += "_KEY"
	}
	return n
}

// security turns the description's ways to authenticate into keys the
// owner fills in. Several can coexist: an empty key sends nothing.
func (p *specParser) security(api *API, swagger bool) {
	var schemes map[string]any
	if swagger {
		schemes, _ = p.root["securityDefinitions"].(map[string]any)
	} else {
		comps, _ := p.root["components"].(map[string]any)
		schemes, _ = comps["securitySchemes"].(map[string]any)
	}
	var names []string
	for k := range schemes {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		s, _ := p.deref(schemes[name]).(map[string]any)
		env := envName(name)
		desc := clean(str(s["description"]), 200)
		kind, in, field := str(s["type"]), str(s["in"]), str(s["name"])
		switch {
		case kind == "apiKey" && in == "header" && field != "":
			if _, dup := api.headers[field]; dup {
				continue
			}
			api.headers[field] = "{{env." + env + "}}"
		case kind == "apiKey" && in == "query" && field != "":
			if _, dup := api.query[field]; dup {
				continue
			}
			api.query[field] = "{{env." + env + "}}"
		case kind == "http" && strings.EqualFold(str(s["scheme"]), "basic"), kind == "basic":
			if _, dup := api.headers["Authorization"]; dup {
				continue
			}
			api.headers["Authorization"] = "Basic {{env." + env + "}}"
			desc = strings.TrimSpace("user:password in base64. " + desc)
		case kind == "http" || kind == "oauth2" || kind == "openIdConnect":
			if _, dup := api.headers["Authorization"]; dup {
				continue
			}
			api.headers["Authorization"] = "Bearer {{env." + env + "}}"
			if kind != "http" {
				desc = strings.TrimSpace("an access token (Pimpo does not run the sign-in). " + desc)
			}
		default:
			continue
		}
		if desc == "" {
			desc = name
		}
		api.Keys = append(api.Keys, APIKey{Name: env, Description: desc})
	}
}

var argUnsafe = regexp.MustCompile(`[^A-Za-z0-9_]+`)

func argName(s string) string {
	n := strings.Trim(argUnsafe.ReplaceAllString(s, "_"), "_")
	if n == "" || n[0] >= '0' && n[0] <= '9' {
		n = "p_" + n
	}
	return n
}

var camel = regexp.MustCompile(`([a-z0-9])([A-Z])`)
var methodUnsafe = regexp.MustCompile(`[^a-z0-9_]+`)

func opMethod(method, path string, op map[string]any, seen map[string]bool) string {
	id := str(op["operationId"])
	if id == "" {
		var parts []string
		for _, seg := range strings.Split(path, "/") {
			if seg != "" && !strings.HasPrefix(seg, "{") {
				parts = append(parts, seg)
			}
		}
		id = method + "_" + strings.Join(parts, "_")
	}
	m := strings.Trim(methodUnsafe.ReplaceAllString(strings.ToLower(camel.ReplaceAllString(id, "${1}_${2}")), "_"), "_")
	if len(m) > 60 {
		m = strings.Trim(m[:60], "_")
	}
	if m == "" {
		m = method
	}
	if m[0] >= '0' && m[0] <= '9' {
		m = "op_" + m
	}
	base := m
	for i := 2; seen[m]; i++ {
		m = fmt.Sprintf("%s_%d", base, i)
	}
	seen[m] = true
	return m
}

type param struct {
	name, arg, in string
	required      bool
	schema        map[string]any
	description   string
}

func (p *specParser) operation(method, path string, op map[string]any, shared []any, swagger bool, seen map[string]bool) (Operation, string) {
	o := Operation{ID: opMethod(method, path, op, seen), Method: strings.ToUpper(method), Path: path}
	o.Summary = clean(str(op["summary"]), 160)
	if o.Summary == "" {
		o.Summary = clean(str(op["description"]), 160)
	}
	if b, _ := op["deprecated"].(bool); b {
		o.Summary = strings.TrimSpace("(deprecated) " + o.Summary)
	}
	o.Risk = "irreversible"
	if o.Method == "GET" {
		o.Risk = "read"
	}

	// Parameters: the operation's replace the path's with the same name.
	byKey := map[string]param{}
	var order []string
	own, _ := op["parameters"].([]any)
	for _, raw := range append(append([]any{}, shared...), own...) {
		m, _ := p.deref(raw).(map[string]any)
		if m == nil {
			continue
		}
		pr := param{name: str(m["name"]), in: str(m["in"]), description: clean(str(m["description"]), 200)}
		pr.required, _ = m["required"].(bool)
		if s, ok := m["schema"]; ok {
			pr.schema = p.schema(s, 0)
		} else {
			pr.schema = p.schema(m, 0) // Swagger 2 puts type on the parameter
		}
		key := pr.in + ":" + pr.name
		if _, dup := byKey[key]; !dup {
			order = append(order, key)
		}
		byKey[key] = pr
	}

	props := map[string]any{}
	var required []string
	used := map[string]bool{}
	addArg := func(name string, schema map[string]any, desc string, req bool) string {
		arg := argName(name)
		for base, i := arg, 2; used[arg]; i++ {
			arg = fmt.Sprintf("%s_%d", base, i)
		}
		used[arg] = true
		s := map[string]any{}
		for k, v := range schema {
			s[k] = v
		}
		if desc != "" && s["description"] == nil {
			s["description"] = desc
		}
		props[arg] = s
		if req {
			required = append(required, arg)
		}
		return arg
	}

	o.request = Request{Method: o.Method, Path: path, Query: map[string]string{}, Headers: map[string]string{}}
	fixedPath := path
	example := map[string]any{}
	fillable := true
	var body map[string]any
	var form bool
	for _, key := range order {
		pr := byKey[key]
		switch pr.in {
		case "path":
			arg := addArg(pr.name, pr.schema, pr.description, true)
			fixedPath = strings.ReplaceAll(fixedPath, "{"+pr.name+"}", "{{"+arg+"}}")
			v, ok := sample(pr.schema)
			fillable = fillable && ok
			example[arg] = v
		case "query":
			arg := addArg(pr.name, pr.schema, pr.description, pr.required)
			o.request.Query[pr.name] = "{{" + arg + "}}"
			if pr.required {
				v, ok := sample(pr.schema)
				fillable = fillable && ok
				example[arg] = v
			}
		case "header":
			if strings.EqualFold(pr.name, "authorization") || strings.EqualFold(pr.name, "content-type") || strings.EqualFold(pr.name, "accept") {
				continue
			}
			arg := addArg(pr.name, pr.schema, pr.description, pr.required)
			o.request.Headers[pr.name] = "{{" + arg + "}}"
			if pr.required {
				fillable = false
			}
		case "cookie":
			if pr.required {
				return o, "needs a cookie"
			}
		case "body": // Swagger 2
			body = map[string]any{"schema": pr.schema, "required": pr.required}
		case "formData":
			if t := str(pr.schema["type"]); t == "file" {
				return o, "uploads a file"
			}
			if body == nil {
				body = map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{}}, "required": true}
			}
			s := body["schema"].(map[string]any)
			s["properties"].(map[string]any)[pr.name] = pr.schema
			if pr.required {
				rq, _ := s["required"].([]any)
				s["required"] = append(rq, pr.name)
			}
			form = true
		}
	}
	if !swagger {
		if rb, _ := p.deref(op["requestBody"]).(map[string]any); rb != nil {
			content, _ := rb["content"].(map[string]any)
			req, _ := rb["required"].(bool)
			switch {
			case content["application/json"] != nil:
				mt, _ := content["application/json"].(map[string]any)
				body = map[string]any{"schema": p.schema(mt["schema"], 0), "required": req}
			case content["application/x-www-form-urlencoded"] != nil:
				mt, _ := content["application/x-www-form-urlencoded"].(map[string]any)
				body = map[string]any{"schema": p.schema(mt["schema"], 0), "required": req}
				form = true
			default:
				for ct := range content {
					if strings.Contains(ct, "json") {
						mt, _ := content[ct].(map[string]any)
						body = map[string]any{"schema": p.schema(mt["schema"], 0), "required": req}
						break
					}
				}
				if body == nil && len(content) > 0 {
					if req {
						return o, "sends " + firstKey(content)
					}
				}
			}
		}
	}
	o.request.Path = fixedPath
	if body != nil {
		schema, _ := body["schema"].(map[string]any)
		req, _ := body["required"].(bool)
		sprops, _ := schema["properties"].(map[string]any)
		if str(schema["type"]) == "object" && len(sprops) > 0 || form {
			need := map[string]bool{}
			if rq, _ := schema["required"].([]any); req {
				for _, r := range rq {
					need[str(r)] = true
				}
			}
			tpl := map[string]any{}
			for _, k := range sortedKeys(sprops) {
				ps, _ := sprops[k].(map[string]any)
				if ro, _ := ps["readOnly"].(bool); ro {
					continue
				}
				arg := addArg(k, ps, "", need[k])
				tpl[k] = "{{" + arg + "}}"
			}
			o.request.Body = tpl
		} else {
			o.request.Body = "{{" + addArg("body", schema, "the request body", req) + "}}"
		}
		o.request.Form = form
		fillable = false
	}
	if len(o.request.Query) == 0 {
		o.request.Query = nil
	}
	if len(o.request.Headers) == 0 {
		o.request.Headers = nil
	}

	sch := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		sort.Strings(required)
		sch["required"] = required
	}
	o.schema, _ = json.Marshal(sch)
	o.signature = sigText(props, required)

	resp := p.response(op, swagger)
	shape := shapeText(resp)
	desc := o.Summary
	if d := clean(str(op["description"]), 300); d != "" && !strings.HasPrefix(d, strings.TrimSuffix(desc, "…")) {
		desc = strings.TrimSpace(strings.TrimRight(desc, ".") + ". " + d)
	}
	o.returns = strings.TrimSpace(shape + " " + desc)
	if o.returns == "" {
		o.returns = "the service's answer"
	}
	if str(resp["type"]) == "array" {
		o.result = &Result{Max: 50}
	}
	if o.Method == "GET" && fillable {
		o.contract = &Case{Args: example, Keys: []string{}}
	}
	return o, ""
}

func firstKey(m map[string]any) string {
	for k := range m {
		return k
	}
	return ""
}

func sortedKeys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// sample is a value the contract can call with: a default or the first
// allowed value, never an invented one.
func sample(schema map[string]any) (any, bool) {
	if d, ok := schema["default"]; ok {
		return d, true
	}
	if e, _ := schema["enum"].([]any); len(e) > 0 {
		return e[0], true
	}
	return nil, false
}

func sigText(props map[string]any, required []string) string {
	req := map[string]bool{}
	for _, r := range required {
		req[r] = true
	}
	var names []string
	for _, k := range sortedKeys(props) {
		if !req[k] {
			k += "?"
		}
		names = append(names, k)
	}
	return "({" + strings.Join(names, ", ") + "})"
}

// response is the schema of the first successful answer.
func (p *specParser) response(op map[string]any, swagger bool) map[string]any {
	resps, _ := op["responses"].(map[string]any)
	for _, code := range []string{"200", "201", "202", "2XX", "default"} {
		r, _ := p.deref(resps[code]).(map[string]any)
		if r == nil {
			continue
		}
		if swagger {
			return p.schema(r["schema"], 0)
		}
		content, _ := r["content"].(map[string]any)
		for _, ct := range append([]string{"application/json"}, sortedKeys(content)...) {
			if mt, _ := content[ct].(map[string]any); mt != nil && strings.Contains(ct, "json") {
				return p.schema(mt["schema"], 0)
			}
		}
		return map[string]any{}
	}
	return map[string]any{}
}

// shapeText renders a response schema like [{id, name, status}].
func shapeText(s map[string]any) string {
	keys := func(s map[string]any) string {
		props, _ := s["properties"].(map[string]any)
		names := sortedKeys(props)
		if len(names) == 0 {
			return ""
		}
		if len(names) > 12 {
			names = append(names[:12], "…")
		}
		return "{" + strings.Join(names, ", ") + "}"
	}
	if str(s["type"]) == "array" {
		items, _ := s["items"].(map[string]any)
		if k := keys(items); k != "" {
			return "[" + k + "]"
		}
		return "[…]"
	}
	return keys(s)
}

// schema inlines a JSON schema, keeping what the model needs, 4 levels
// deep at most.
func (p *specParser) schema(v any, depth int) map[string]any {
	m, _ := p.deref(v).(map[string]any)
	out := map[string]any{}
	if m == nil {
		return out
	}
	if all, _ := m["allOf"].([]any); len(all) > 0 {
		merged := map[string]any{"type": "object", "properties": map[string]any{}}
		var req []any
		for _, part := range all {
			s := p.schema(part, depth)
			if props, _ := s["properties"].(map[string]any); props != nil {
				for k, pv := range props {
					merged["properties"].(map[string]any)[k] = pv
				}
			}
			if r, _ := s["required"].([]any); r != nil {
				req = append(req, r...)
			}
			if s["type"] != nil && s["type"] != "object" {
				return s
			}
		}
		if len(req) > 0 {
			merged["required"] = req
		}
		return merged
	}
	for _, alt := range []string{"oneOf", "anyOf"} {
		if list, _ := m[alt].([]any); len(list) > 0 {
			for _, e := range list {
				if s := p.schema(e, depth); str(s["type"]) != "null" {
					return s
				}
			}
		}
	}
	for _, k := range []string{"type", "format", "enum", "default", "minimum", "maximum", "pattern"} {
		if x, ok := m[k]; ok {
			out[k] = x
		}
	}
	if t, ok := m["type"].([]any); ok { // 3.1 allows ["string", "null"]
		for _, e := range t {
			if str(e) != "null" {
				out["type"] = str(e)
				break
			}
		}
	}
	if b, _ := m["readOnly"].(bool); b {
		out["readOnly"] = true
	}
	if d := clean(str(m["description"]), 160); d != "" {
		out["description"] = d
	}
	if depth >= 4 {
		return out
	}
	if props, _ := m["properties"].(map[string]any); props != nil {
		if out["type"] == nil {
			out["type"] = "object"
		}
		np := map[string]any{}
		for k, pv := range props {
			np[k] = p.schema(pv, depth+1)
		}
		out["properties"] = np
		if r, _ := m["required"].([]any); r != nil {
			out["required"] = r
		}
	}
	if items, ok := m["items"]; ok {
		out["items"] = p.schema(items, depth+1)
	}
	return out
}

// CustomKey is the secret for a header the description does not declare
// (many APIs need a token without saying so).
const CustomKey = "AUTH_HEADER"

// AddHeader sends header on every request, its whole value a key the
// owner fills in (for example "Bearer ghp_…").
func (a *API) AddHeader(header string) error {
	if !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{0,60}$`).MatchString(header) {
		return fmt.Errorf("%q is not a header name", header)
	}
	for k := range a.headers {
		if strings.EqualFold(k, header) {
			delete(a.headers, k)
		}
	}
	a.headers[header] = "{{env." + CustomKey + "}}"
	for _, k := range a.Keys {
		if k.Name == CustomKey {
			return nil
		}
	}
	a.Keys = append(a.Keys, APIKey{Name: CustomKey, Description: "the whole value of the " + header + " header"})
	return nil
}

// Manifest builds a connector from the chosen operations (id → risk).
// A non-GET operation chosen as read is marked safe.
func (a *API) Manifest(name, description, source string, chosen map[string]string) (Manifest, error) {
	if len(chosen) == 0 {
		return Manifest{}, errors.New("choose at least one operation")
	}
	if len(chosen) > MaxImported {
		return Manifest{}, fmt.Errorf("choose at most %d operations; each one is a capability the model reads about", MaxImported)
	}
	if description == "" {
		description = a.Title
		if a.Description != "" {
			description = clean(a.Title+": "+a.Description, 300)
		}
	}
	m := Manifest{Name: name, Description: description, Imported: true, Source: source,
		HTTP: &HTTPSpec{Base: a.Base, Headers: a.headers, Query: a.query}}
	if len(m.HTTP.Headers) == 0 {
		m.HTTP.Headers = nil
	}
	if len(m.HTTP.Query) == 0 {
		m.HTTP.Query = nil
	}
	for _, k := range a.Keys {
		m.Env = append(m.Env, k.Name)
	}
	left := map[string]string{}
	for k, v := range chosen {
		left[k] = v
	}
	for _, o := range a.Operations {
		risk, ok := chosen[o.ID]
		if !ok {
			continue
		}
		delete(left, o.ID)
		if _, known := risks[risk]; !known {
			return m, fmt.Errorf("%s: risk must be read, notify, reversible or irreversible", o.ID)
		}
		req := o.request
		req.Safe = risk == "read" && o.Method != "GET"
		capName := name + "." + o.ID
		m.Capabilities = append(m.Capabilities, Capability{Name: capName, Risk: risk, Signature: capName + o.signature, Returns: o.returns,
			Schema: o.schema, Request: &req, Result: o.result})
		if o.contract != nil && risk == "read" && len(m.Contract) < 5 {
			c := *o.contract
			c.Capability = capName
			m.Contract = append(m.Contract, c)
		}
	}
	for id := range left {
		return m, fmt.Errorf("the description has no operation %s", id)
	}
	return m, nil
}
