package vault

// A secret's stored value can be a reference to a secret kept in an
// outside password manager instead of the secret itself:
//
//	op://Vault/Item/field            1Password (op CLI or Connect)
//	op://Vault/Item/section/field
//	vault://secret/data/path#field   HashiCorp Vault, KV version 2
//
// Get resolves a reference when a connector asks for the secret, so every
// consumer works unchanged. Resolved values stay only in memory, for a few
// minutes, and are never written to disk. The credentials that read them
// live in this vault too, one set for the house and one per person: a
// person's secrets (person.<id>.…) resolve only with that person's own
// credentials, never the house's.

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
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	opPrefix = "op://"
	hvPrefix = "vault://"
	// cacheFor is how long a resolved value is kept in memory; a failure
	// is kept for less, so a fix shows soon.
	cacheFor   = 5 * time.Minute
	failureFor = 30 * time.Second
	maxAnswer  = 1 << 20
)

// opTimeout bounds each run of the op CLI.
var opTimeout = 15 * time.Second

// Managers are the outside password managers of one scope (the house, or
// one person). They are kept as one secret, never shown back.
type Managers struct {
	OnePassword *OnePassword `json:"onepassword,omitempty"`
	HashiCorp   *HashiCorp   `json:"hashicorp,omitempty"`
}

// OnePassword reads with a Connect server when ConnectURL is set, with the
// op CLI and a service-account token when Token is set, or, for the house
// only, with the op CLI signed in through the desktop app on this computer.
type OnePassword struct {
	Token        string `json:"token,omitempty"`
	Desktop      bool   `json:"desktop,omitempty"`
	ConnectURL   string `json:"connect_url,omitempty"`
	ConnectToken string `json:"connect_token,omitempty"`
}

// Mode names how 1Password is read: connect, service or desktop.
func (o *OnePassword) Mode() string {
	switch {
	case o == nil:
		return ""
	case o.ConnectURL != "":
		return "connect"
	case o.Token != "":
		return "service"
	case o.Desktop:
		return "desktop"
	}
	return ""
}

// HashiCorp signs in with a token, or with AppRole (RoleID and SecretID).
type HashiCorp struct {
	Addr      string `json:"addr"`
	Token     string `json:"token,omitempty"`
	RoleID    string `json:"role_id,omitempty"`
	SecretID  string `json:"secret_id,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	// AuthMount is where AppRole is enabled; "approle" when empty.
	AuthMount string `json:"auth_mount,omitempty"`
}

// Auth names how HashiCorp Vault is signed in to: token or approle.
func (h *HashiCorp) Auth() string {
	switch {
	case h == nil:
		return ""
	case h.RoleID != "":
		return "approle"
	case h.Token != "":
		return "token"
	}
	return ""
}

// RefError is a reference that could not be read. It names the reference,
// never a value.
type RefError struct {
	Ref    string
	Reason string
}

func (e *RefError) Error() string {
	return fmt.Sprintf("the secret reference %s could not be read: %s", e.Ref, e.Reason)
}

// ErrBadReference is text that looks like a reference but is not one.
var ErrBadReference = errors.New("a reference looks like op://Vault/Item/field or vault://secret/data/path#field")

type ref struct {
	raw                           string
	op                            bool
	opVault, opItem, opSect, opFd string
	path, field                   string
}

// IsReference tells whether a stored value is a reference to an outside
// password manager rather than a secret.
func IsReference(s string) bool {
	_, err := parseRef(s)
	return err == nil
}

// LooksLikeReference is text that starts like a reference, well formed or
// not.
func LooksLikeReference(s string) bool {
	return strings.HasPrefix(s, opPrefix) || strings.HasPrefix(s, hvPrefix)
}

func parseRef(s string) (ref, error) {
	if strings.ContainsAny(s, "\x00\r\n\t\"") || strings.Contains(s, "..") {
		return ref{}, ErrBadReference
	}
	r := ref{raw: s}
	switch {
	case strings.HasPrefix(s, opPrefix):
		parts := strings.Split(strings.TrimPrefix(s, opPrefix), "/")
		if len(parts) != 3 && len(parts) != 4 {
			return ref{}, ErrBadReference
		}
		for _, p := range parts {
			if strings.TrimSpace(p) == "" || strings.ContainsAny(p, "?#") {
				return ref{}, ErrBadReference
			}
		}
		r.op, r.opVault, r.opItem, r.opFd = true, parts[0], parts[1], parts[len(parts)-1]
		if len(parts) == 4 {
			r.opSect = parts[2]
		}
	case strings.HasPrefix(s, hvPrefix):
		path, field, ok := strings.Cut(strings.TrimPrefix(s, hvPrefix), "#")
		if !ok || path == "" || field == "" || strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?# ") {
			return ref{}, ErrBadReference
		}
		r.path, r.field = path, field
	default:
		return ref{}, ErrBadReference
	}
	return r, nil
}

// ScopeOf is whose credentials resolve a secret: the person's id for
// person.<id>.…, or "" for the house.
func ScopeOf(name string) string {
	if rest, ok := strings.CutPrefix(name, "person."); ok {
		if id, _, ok := strings.Cut(rest, "."); ok {
			return id
		}
	}
	return ""
}

func managersName(scope string) string {
	if scope == "" {
		return "pm"
	}
	return "person." + scope + ".pm"
}

// outside reads references and keeps what it read, in memory only.
type outside struct {
	mu     sync.Mutex
	cache  map[string]cached
	tokens map[string]cached // AppRole sign-ins, by scope
	now    func() time.Time
	op     string
	client *http.Client
}

type cached struct {
	value string
	err   error
	until time.Time
}

func newOutside() *outside {
	return &outside{
		cache:  map[string]cached{},
		tokens: map[string]cached{},
		now:    time.Now,
		op:     "op",
		client: &http.Client{
			Timeout: 10 * time.Second,
			// A redirect would carry the token somewhere else.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (o *outside) forget(scope string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for k := range o.cache {
		if strings.HasPrefix(k, scope+"\x00") {
			delete(o.cache, k)
		}
	}
	delete(o.tokens, scope)
}

// Raw is a secret as stored: a reference stays a reference. Backups and
// exports use it, so they never hold what a reference points to.
func (v *Vault) Raw(ctx context.Context, name string) (string, error) {
	return v.stored(ctx, name)
}

// Has tells whether a secret is set, without reading what a reference
// points to, so a status page never waits on a password manager.
func (v *Vault) Has(ctx context.Context, name string) bool {
	s, err := v.stored(ctx, name)
	return err == nil && s != ""
}

// Managers gives the outside password managers of a scope ("" for the
// house, or a person's id).
func (v *Vault) Managers(ctx context.Context, scope string) (Managers, error) {
	var m Managers
	s, err := v.stored(ctx, managersName(scope))
	if errors.Is(err, ErrNotFound) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal([]byte(s), &m)
}

// SetManagers keeps a scope's password managers and forgets what was read
// with the old ones.
func (v *Vault) SetManagers(ctx context.Context, scope string, m Managers) error {
	if err := m.check(scope); err != nil {
		return err
	}
	defer v.out.forget(scope)
	if m.OnePassword.Mode() == "" && m.HashiCorp.Auth() == "" {
		return v.Delete(ctx, managersName(scope))
	}
	if m.OnePassword.Mode() == "" {
		m.OnePassword = nil
	}
	if m.HashiCorp.Auth() == "" {
		m.HashiCorp = nil
	}
	b, _ := json.Marshal(m)
	return v.Set(ctx, managersName(scope), string(b))
}

func (m Managers) check(scope string) error {
	if o := m.OnePassword; o != nil {
		if o.Desktop && o.Token == "" && o.ConnectURL == "" && scope != "" {
			return errors.New("the 1Password app on this computer belongs to the house; use a service account token or a Connect server")
		}
		if o.ConnectURL != "" {
			if err := checkAddr(o.ConnectURL, scope); err != nil {
				return err
			}
			if o.ConnectToken == "" {
				return errors.New("a 1Password Connect server needs its token")
			}
		}
	}
	if h := m.HashiCorp; h != nil && h.Auth() != "" {
		if err := checkAddr(h.Addr, scope); err != nil {
			return err
		}
		if h.RoleID != "" && h.SecretID == "" {
			return errors.New("AppRole needs both the role id and the secret id")
		}
	}
	return nil
}

// checkAddr allows https, and plain http only to this computer and only
// for the house.
func checkAddr(raw, scope string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%q is not a server address like https://vault.example.com:8200", raw)
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && scope == "" && loopback(u.Hostname()) {
		return nil
	}
	return errors.New("a password manager's address must start with https:// (plain http only for this computer, and only for the house)")
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Resolve reads what a reference points to, with the credentials of scope.
func (v *Vault) Resolve(ctx context.Context, scope, reference string) (string, error) {
	r, err := parseRef(reference)
	if err != nil {
		return "", err
	}
	key := scope + "\x00" + reference
	o := v.out
	o.mu.Lock()
	c, ok := o.cache[key]
	o.mu.Unlock()
	if ok && o.now().Before(c.until) {
		return c.value, c.err
	}
	value, err := v.read(ctx, scope, r)
	if err != nil && ctx.Err() != nil {
		return "", err // a cancelled request says nothing about the reference
	}
	c = cached{value: value, err: err, until: o.now().Add(cacheFor)}
	if err != nil {
		c.until = o.now().Add(failureFor)
	}
	o.mu.Lock()
	o.cache[key] = c
	o.mu.Unlock()
	return value, err
}

// Check resolves a reference afresh and keeps only whether it was found.
func (v *Vault) Check(ctx context.Context, scope, reference string) error {
	v.out.mu.Lock()
	delete(v.out.cache, scope+"\x00"+reference)
	v.out.mu.Unlock()
	_, err := v.Resolve(ctx, scope, reference)
	return err
}

func (v *Vault) read(ctx context.Context, scope string, r ref) (string, error) {
	fail := func(reason string) error { return &RefError{Ref: r.raw, Reason: reason} }
	m, err := v.Managers(ctx, scope)
	if err != nil {
		return "", fail("the password manager settings cannot be read")
	}
	if r.op {
		op := m.OnePassword
		switch op.Mode() {
		case "connect":
			return v.out.connectRead(ctx, op, r, fail)
		case "service", "desktop":
			if op.Mode() == "desktop" && scope != "" {
				return "", fail("1Password is not set up for this person")
			}
			return v.out.opRead(ctx, op.Token, r, fail)
		}
		return "", fail(notSetUp("1Password", scope))
	}
	if m.HashiCorp.Auth() == "" {
		return "", fail(notSetUp("HashiCorp Vault", scope))
	}
	return v.out.hvRead(ctx, scope, m.HashiCorp, r, fail)
}

func notSetUp(what, scope string) string {
	if scope == "" {
		return what + " is not set up for the house (Connections, Password managers)"
	}
	return what + " is not set up for this person (Account, Password managers)"
}

// opCmd runs the op CLI with a clean environment: only PATH, HOME and
// the service-account token when there is one.
func (o *outside) opCmd(ctx context.Context, token string, args ...string) ([]byte, string, error) {
	path, err := exec.LookPath(o.op)
	if err != nil {
		return nil, "the 1Password command line (op) is not installed on this computer", err
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	if token != "" {
		cmd.Env = append(cmd.Env, "OP_SERVICE_ACCOUNT_TOKEN="+token)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &limited{b: &out}, &limited{b: &errOut}
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, "1Password did not answer in time", ctx.Err()
	}
	if err != nil {
		return nil, opReason(errOut.String()), err
	}
	return out.Bytes(), "", nil
}

func (o *outside) opRead(ctx context.Context, token string, r ref, fail func(string) error) (string, error) {
	out, reason, err := o.opCmd(ctx, token, "read", "--no-newline", r.raw)
	if err != nil {
		return "", fail(reason)
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// opReason keeps the first line of what op said went wrong, without its
// timestamp. op names the reference there, never a value.
func opReason(stderr string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(stderr), "\n")
	if rest, ok := strings.CutPrefix(line, "[ERROR] "); ok {
		// "[ERROR] 2024/01/02 15:04:05 message"
		if f := strings.SplitN(rest, " ", 3); len(f) == 3 && strings.Count(f[0], "/") == 2 {
			rest = f[2]
		}
		line = rest
	}
	if len(line) > 200 {
		line = line[:200]
	}
	if line == "" {
		return "1Password refused"
	}
	return "1Password said: " + line
}

type limited struct{ b *bytes.Buffer }

func (l *limited) Write(p []byte) (int, error) {
	if room := maxAnswer - l.b.Len(); room > 0 {
		if len(p) > room {
			l.b.Write(p[:room])
		} else {
			l.b.Write(p)
		}
	}
	return len(p), nil
}

// call asks a password manager's HTTP API for JSON.
func (o *outside) call(ctx context.Context, method, u string, headers map[string]string, body, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return 0, err
	}
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, json.NewDecoder(io.LimitReader(resp.Body, maxAnswer)).Decode(out)
}

func unreachable(what string, err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return what + " could not be reached: " + err.Error()
}

func status(what string, code int) string {
	switch code {
	case 401, 403:
		return what + " refused the credentials (" + fmt.Sprint(code) + ")"
	case 404:
		return "not found in " + what
	}
	return what + " answered " + fmt.Sprint(code)
}

func (o *outside) connectRead(ctx context.Context, op *OnePassword, r ref, fail func(string) error) (string, error) {
	const what = "1Password Connect"
	base := strings.TrimRight(op.ConnectURL, "/")
	h := map[string]string{"Authorization": "Bearer " + op.ConnectToken}
	get := func(path string, out any) error {
		code, err := o.call(ctx, "GET", base+path, h, nil, out)
		if err != nil {
			return fail(unreachable(what, err))
		}
		if code != 200 {
			return fail(status(what, code))
		}
		return nil
	}
	filter := func(field, v string) string {
		return url.QueryEscape(field + ` eq "` + v + `"`)
	}
	var vaults []struct{ ID string }
	if err := get("/v1/vaults?filter="+filter("name", r.opVault), &vaults); err != nil {
		return "", err
	}
	if len(vaults) == 0 {
		return "", fail("there is no vault " + r.opVault + " in 1Password")
	}
	vid := url.PathEscape(vaults[0].ID)
	var items []struct{ ID string }
	if err := get("/v1/vaults/"+vid+"/items?filter="+filter("title", r.opItem), &items); err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "", fail("there is no item " + r.opItem + " in the vault " + r.opVault)
	}
	var item struct {
		Fields []struct {
			ID, Label, Value string
			Section          *struct{ ID, Label string }
		}
	}
	if err := get("/v1/vaults/"+vid+"/items/"+url.PathEscape(items[0].ID), &item); err != nil {
		return "", err
	}
	for _, f := range item.Fields {
		if f.Label != r.opFd && f.ID != r.opFd {
			continue
		}
		if r.opSect != "" && (f.Section == nil || (f.Section.Label != r.opSect && f.Section.ID != r.opSect)) {
			continue
		}
		return f.Value, nil
	}
	return "", fail("the item " + r.opItem + " has no field " + r.opFd)
}

func (o *outside) hvRead(ctx context.Context, scope string, hv *HashiCorp, r ref, fail func(string) error) (string, error) {
	const what = "HashiCorp Vault"
	tok, err := o.hvToken(ctx, scope, hv)
	if err != nil {
		return "", fail(err.Error())
	}
	var body struct {
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}
	code, err := o.call(ctx, "GET", strings.TrimRight(hv.Addr, "/")+"/v1/"+r.path, map[string]string{"X-Vault-Token": tok, "X-Vault-Namespace": hv.Namespace}, nil, &body)
	if err != nil {
		return "", fail(unreachable(what, err))
	}
	if code != 200 {
		return "", fail(status(what, code))
	}
	val, ok := body.Data.Data[r.field]
	if !ok {
		return "", fail("the secret has no field " + r.field + " (a KV version 2 path has /data/ after the mount)")
	}
	s, ok := val.(string)
	if !ok {
		return "", fail("the field " + r.field + " is not text")
	}
	return s, nil
}

// hvToken is the token to read with; AppRole signs in and keeps the
// token in memory while it lasts.
func (o *outside) hvToken(ctx context.Context, scope string, hv *HashiCorp) (string, error) {
	if hv.RoleID == "" {
		return hv.Token, nil
	}
	o.mu.Lock()
	c, ok := o.tokens[scope]
	o.mu.Unlock()
	if ok && o.now().Before(c.until) {
		return c.value, nil
	}
	mount := hv.AuthMount
	if mount == "" {
		mount = "approle"
	}
	var out struct {
		Auth struct {
			ClientToken   string `json:"client_token"`
			LeaseDuration int    `json:"lease_duration"`
		} `json:"auth"`
	}
	code, err := o.call(ctx, "POST", strings.TrimRight(hv.Addr, "/")+"/v1/auth/"+url.PathEscape(mount)+"/login",
		map[string]string{"X-Vault-Namespace": hv.Namespace}, map[string]string{"role_id": hv.RoleID, "secret_id": hv.SecretID}, &out)
	if err != nil {
		return "", errors.New(unreachable("HashiCorp Vault", err))
	}
	if code != 200 || out.Auth.ClientToken == "" {
		return "", errors.New("HashiCorp Vault did not accept the AppRole sign-in (" + fmt.Sprint(code) + ")")
	}
	life := time.Duration(out.Auth.LeaseDuration) * time.Second * 8 / 10
	if life <= 0 || life > 30*time.Minute {
		life = 30 * time.Minute
	}
	o.mu.Lock()
	o.tokens[scope] = cached{value: out.Auth.ClientToken, until: o.now().Add(life)}
	o.mu.Unlock()
	return out.Auth.ClientToken, nil
}

// TestManager checks that a scope's password manager answers with its
// credentials, reading no secret.
func (v *Vault) TestManager(ctx context.Context, scope, kind string) error {
	m, err := v.Managers(ctx, scope)
	if err != nil {
		return err
	}
	o := v.out
	switch kind {
	case "onepassword":
		op := m.OnePassword
		switch op.Mode() {
		case "connect":
			var vaults []any
			code, err := o.call(ctx, "GET", strings.TrimRight(op.ConnectURL, "/")+"/v1/vaults", map[string]string{"Authorization": "Bearer " + op.ConnectToken}, nil, &vaults)
			if err != nil {
				return errors.New(unreachable("1Password Connect", err))
			}
			if code != 200 {
				return errors.New(status("1Password Connect", code))
			}
			return nil
		case "service", "desktop":
			if op.Mode() == "desktop" && scope != "" {
				break
			}
			if _, reason, err := o.opCmd(ctx, op.Token, "whoami"); err != nil {
				return errors.New(reason)
			}
			return nil
		}
		return errors.New(notSetUp("1Password", scope))
	case "hashicorp":
		hv := m.HashiCorp
		if hv.Auth() == "" {
			return errors.New(notSetUp("HashiCorp Vault", scope))
		}
		o.forget(scope)
		tok, err := o.hvToken(ctx, scope, hv)
		if err != nil {
			return err
		}
		var self any
		code, err := o.call(ctx, "GET", strings.TrimRight(hv.Addr, "/")+"/v1/auth/token/lookup-self", map[string]string{"X-Vault-Token": tok, "X-Vault-Namespace": hv.Namespace}, nil, &self)
		if err != nil {
			return errors.New(unreachable("HashiCorp Vault", err))
		}
		if code != 200 {
			return errors.New(status("HashiCorp Vault", code))
		}
		return nil
	}
	return errors.New("unknown password manager")
}

// OPInstalled tells whether the op command is on this computer.
func (v *Vault) OPInstalled() bool {
	_, err := exec.LookPath(v.out.op)
	return err == nil
}
