// Package runtime runs compiled routines. A routine is plain JavaScript that
// defines `async function run()`. It sees only the capability objects its
// manifest declares, plus now() and log(). There is no network, disk,
// process or timer access: everything goes through the Host.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"

	"github.com/turbine-dev/pimpo/internal/capability"
)

// Host performs capability calls on behalf of a routine. It is where the
// policy engine, recording and real connectors live.
type Host interface {
	Call(ctx context.Context, capability, scope string, args any) (any, error)
	// Judge answers a declared judgment about one item with a probability.
	Judge(ctx context.Context, name, question string, item any) (float64, error)
}

// Writer composes short text for a routine: a summary, what an email
// asks, a draft reply. Only hosts that can reach a model implement it.
type Writer interface {
	Write(ctx context.Context, name, instruction string, input any) (string, error)
}

// MaxWrites bounds the text a single run may ask a model to write.
const MaxWrites = 20

type Manifest struct {
	Schedule     string            `json:"schedule"`
	Capabilities []string          `json:"capabilities"`
	Judgments    map[string]string `json:"judgments,omitempty"`
	// Writes are texts a small model composes for each item, by name:
	// the instruction, in the routine's language.
	Writes map[string]string `json:"writes,omitempty"`
	// Locale is the language of the routine's messages, e.g. pt-BR or en-US.
	Locale string `json:"locale,omitempty"`
	// Params are the settings the owner can change without code.
	Params []Param `json:"params,omitempty"`
	// Watch, when set, runs the routine when a read capability returns
	// items it has not seen, instead of (or besides) the schedule.
	Watch *Watch `json:"watch,omitempty"`
	// Webhook says the routine is started by another service calling its
	// address (see event.webhook) rather than by a clock.
	Webhook bool `json:"webhook,omitempty"`
	// Uses are other routines, by id, this one may run as helpers with
	// routines.run(id, params). Everything they touch must also be in
	// Capabilities: a helper never widens what the owner approved.
	Uses []string `json:"uses,omitempty"`
}

// Watch is what a routine waits for: Pimpo calls Capability with Args
// every Every, without a model, and runs the routine with the items whose
// Key it has not seen before, as event.items.
type Watch struct {
	Capability string         `json:"capability"`
	Args       map[string]any `json:"args,omitempty"`
	Key        string         `json:"key"`
	Every      string         `json:"every,omitempty"`
}

// Starts reports whether something starts the routine: a schedule or a
// watch. Routines are saved only when it does.
func (m Manifest) Starts() error {
	if m.Watch == nil && !m.Webhook && strings.TrimSpace(m.Schedule) == "" {
		return errors.New("a routine needs a schedule, something to watch, or a webhook")
	}
	return nil
}

// Interval is how often the watch polls, between 5 minutes and a day.
func (w Watch) Interval() time.Duration {
	d, err := time.ParseDuration(w.Every)
	if err != nil || d == 0 {
		return 10 * time.Minute
	}
	return min(max(d, 5*time.Minute), 24*time.Hour)
}

// ArgsWith fills {{name}} in string arguments with parameter values, so
// a watched query can follow a setting the owner changes.
func (w Watch) ArgsWith(params map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range w.Args {
		if s, ok := v.(string); ok {
			for name, pv := range params {
				s = strings.ReplaceAll(s, "{{"+name+"}}", fmt.Sprint(pv))
			}
			v = s
		}
		out[k] = v
	}
	return out
}

type Options struct {
	Now time.Time
	// Zone and Locale drive the dates and money helpers; they default to
	// the machine's zone and Portuguese.
	Zone   *time.Location
	Locale string
	// Timeout bounds the routine's own work: time spent waiting on a
	// capability, a judgment, a text or another routine does not count,
	// since those have limits of their own and the caller's context bounds
	// the whole run.
	Timeout time.Duration
	// MaxCalls bounds capability calls per run, so a runaway loop stops.
	MaxCalls int
	// Params are the owner's values for the manifest's parameters; missing
	// ones take their defaults.
	Params map[string]any
	// Event is what woke a watching routine: {items: [...]}.
	Event any
	// State is what the routine kept from earlier runs (state.get/set).
	State map[string]any
	// Library finds the routines listed in the manifest's uses.
	Library func(ctx context.Context, id string) (Helper, error)
	// ID is the running routine's id, so it cannot end up running itself.
	ID string

	chain []string
}

// Helper is a routine another one runs: its code, manifest, parameters
// and kept state (which it can read but not change when run as a helper).
type Helper struct {
	Code     string
	Manifest Manifest
	Params   map[string]any
	State    map[string]any
}

type Result struct {
	Logs  []string
	Calls int
	// State is the routine's kept state after the run; it is saved only
	// when the run succeeds, so a failure never leaves half an update.
	State map[string]any
	// Changed reports whether state.set or state.delete was called.
	Changed bool
	// Return is what run() resolved to, for routines used as helpers.
	Return any
}

// MaxState bounds what a routine keeps between runs, as JSON.
const MaxState = 64 << 10

// MaxDepth bounds routines running routines.
const MaxDepth = 3

var ErrTimeout = errors.New("routine ran past its time limit")

// Validate checks a manifest against the capability catalog.
func (m Manifest) Validate() error {
	if len(m.Capabilities) == 0 {
		return errors.New("manifest declares no capabilities")
	}
	for _, c := range m.Capabilities {
		if _, _, err := capability.Parse(c); err != nil {
			return err
		}
	}
	for name := range m.Judgments {
		if !isIdent(name) {
			return fmt.Errorf("judgment name %q must be a JavaScript identifier", name)
		}
	}
	for name, instruction := range m.Writes {
		if !isIdent(name) {
			return fmt.Errorf("write name %q must be a JavaScript identifier", name)
		}
		if strings.TrimSpace(instruction) == "" || len([]rune(instruction)) > 500 {
			return fmt.Errorf("write %s needs an instruction of up to 500 characters", name)
		}
	}
	if w := m.Watch; w != nil {
		name := strings.SplitN(w.Capability, ":", 2)[0]
		spec, ok := capability.Catalog[name]
		if !ok || spec.Risk != capability.Read {
			return fmt.Errorf("a routine can only watch a read capability, not %q", w.Capability)
		}
		declared := false
		for _, c := range m.Capabilities {
			declared = declared || strings.SplitN(c, ":", 2)[0] == name
		}
		if !declared {
			return fmt.Errorf("the watched capability %s must be in capabilities", name)
		}
		if strings.TrimSpace(w.Key) == "" {
			return errors.New("a watch needs the key that identifies an item, e.g. id")
		}
		if w.Every != "" {
			if _, err := time.ParseDuration(w.Every); err != nil {
				return fmt.Errorf("watch.every %q is not a duration like 10m", w.Every)
			}
		}
	}
	for _, id := range m.Uses {
		if strings.TrimSpace(id) == "" {
			return errors.New("uses lists an empty routine id")
		}
	}
	seen := map[string]bool{}
	for _, p := range m.Params {
		if err := p.validate(); err != nil {
			return err
		}
		if seen[p.Name] {
			return fmt.Errorf("parameter %s is declared twice", p.Name)
		}
		seen[p.Name] = true
	}
	return nil
}

// Run executes the routine once.
func Run(ctx context.Context, code string, m Manifest, host Host, opt Options) (Result, error) {
	if err := m.Validate(); err != nil {
		return Result{}, err
	}
	if opt.Timeout == 0 {
		opt.Timeout = 30 * time.Second
	}
	if opt.MaxCalls == 0 {
		opt.MaxCalls = 500
	}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	if opt.Zone == nil {
		opt.Zone = opt.Now.Location()
	}
	if m.Locale != "" {
		opt.Locale = m.Locale
	}
	if opt.Locale == "" {
		opt.Locale = "pt-BR"
	}
	opt.Now = opt.Now.In(opt.Zone)
	params, err := m.ResolveParams(opt.Params)
	if err != nil {
		return Result{}, err
	}
	vm := goja.New()
	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
	res := &Result{}

	fail := func(err error) {
		panic(vm.NewGoError(err))
	}
	clock := &workClock{limit: opt.Timeout, vm: vm}
	// waiting stops the clock while the routine waits on the host.
	waiting := func(fn func() goja.Value) goja.Value {
		clock.pause()
		defer clock.resume()
		return fn()
	}
	bind := func(obj *goja.Object, method string, fn func(goja.FunctionCall) goja.Value) {
		if err := obj.Set(method, fn); err != nil {
			fail(err)
		}
	}

	scopes := map[string][]string{}
	for _, entry := range m.Capabilities {
		spec, scope, _ := capability.Parse(entry)
		scopes[spec.Name] = append(scopes[spec.Name], scope)
	}
	objects := map[string]*goja.Object{}
	for name, allowed := range scopes {
		ns, method, _ := strings.Cut(name, ".")
		obj, ok := objects[ns]
		if !ok {
			obj = vm.NewObject()
			objects[ns] = obj
			vm.Set(ns, obj)
		}
		name, allowed := name, allowed
		bind(obj, method, func(call goja.FunctionCall) goja.Value {
			res.Calls++
			if res.Calls > opt.MaxCalls {
				fail(fmt.Errorf("routine made more than %d capability calls", opt.MaxCalls))
			}
			args := exportArgs(call)
			scope := ""
			if allowed[0] != "" {
				scope = scopeFor(name, args, allowed)
				if scope == "" {
					fail(fmt.Errorf("%s: %v is outside the manifest scope %v", name, args, allowed))
				}
			}
			var out any
			var err error
			waiting(func() goja.Value { out, err = host.Call(ctx, name, scope, args); return nil })
			if err != nil {
				fail(fmt.Errorf("%s: %w", name, err))
			}
			return toJS(vm, out)
		})
	}
	if len(m.Judgments) > 0 {
		judge := vm.NewObject()
		vm.Set("judge", judge)
		for name, question := range m.Judgments {
			name, question := name, question
			bind(judge, name, func(call goja.FunctionCall) goja.Value {
				res.Calls++
				var p float64
				var err error
				item := call.Argument(0).Export()
				waiting(func() goja.Value { p, err = host.Judge(ctx, name, question, item); return nil })
				if err != nil {
					fail(fmt.Errorf("judge.%s: %w", name, err))
				}
				return vm.ToValue(map[string]any{"p": p})
			})
		}
	}
	if len(m.Writes) > 0 {
		writer, ok := host.(Writer)
		if !ok {
			return Result{}, errors.New("this routine writes text, and no model is available to write it")
		}
		write := vm.NewObject()
		vm.Set("write", write)
		written := 0
		for name, instruction := range m.Writes {
			name, instruction := name, instruction
			bind(write, name, func(call goja.FunctionCall) goja.Value {
				written++
				if written > MaxWrites {
					fail(fmt.Errorf("routine asked for more than %d texts in one run", MaxWrites))
				}
				var text string
				var err error
				input := call.Argument(0).Export()
				waiting(func() goja.Value { text, err = writer.Write(ctx, name, instruction, input); return nil })
				if err != nil {
					fail(fmt.Errorf("write.%s: %w", name, err))
				}
				return vm.ToValue(map[string]any{"text": text})
			})
		}
	}
	vm.Set("now", func() string { return opt.Now.Format(time.RFC3339) })
	installStdlib(vm, opt.Zone, opt.Locale, opt.Now)
	raw, _ := json.Marshal(params)
	vm.Set("__params", string(raw))
	if _, err := vm.RunString(`var params = (function f(o) { Object.values(o).forEach(v => v && typeof v === "object" && f(v)); return Object.freeze(o) })(JSON.parse(__params)); delete globalThis.__params;`); err != nil {
		return Result{}, err
	}
	event := opt.Event
	if event == nil {
		event = map[string]any{"items": []any{}}
	}
	rawEvent, _ := json.Marshal(event)
	vm.Set("__event", string(rawEvent))
	if _, err := vm.RunString(`var event = (function f(o) { Object.values(o).forEach(v => v && typeof v === "object" && f(v)); return Object.freeze(o) })(JSON.parse(__event)); delete globalThis.__event;`); err != nil {
		return Result{}, err
	}
	vm.Set("log", func(msg string) { res.Logs = append(res.Logs, msg) })

	// state keeps small values between runs. Values are copied through
	// JSON, so what is kept is exactly what will come back next time.
	res.State = map[string]any{}
	for k, v := range opt.State {
		res.State[k] = v
	}
	st := vm.NewObject()
	vm.Set("state", st)
	bind(st, "get", func(call goja.FunctionCall) goja.Value {
		v, ok := res.State[call.Argument(0).String()]
		if !ok {
			return goja.Null()
		}
		return toJS(vm, v)
	})
	bind(st, "set", func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		b, err := json.Marshal(call.Argument(1).Export())
		if err != nil || key == "" {
			fail(fmt.Errorf("state.set(%q): the value must be plain data", key))
		}
		var v any
		json.Unmarshal(b, &v)
		next := map[string]any{}
		for k, old := range res.State {
			next[k] = old
		}
		next[key] = v
		if all, _ := json.Marshal(next); len(all) > MaxState {
			fail(fmt.Errorf("state would exceed %d KB; keep less between runs", MaxState>>10))
		}
		res.State, res.Changed = next, true
		return goja.Undefined()
	})
	bind(st, "delete", func(call goja.FunctionCall) goja.Value {
		delete(res.State, call.Argument(0).String())
		res.Changed = true
		return goja.Undefined()
	})
	bind(st, "keys", func(goja.FunctionCall) goja.Value {
		keys := make([]any, 0, len(res.State))
		for k := range res.State {
			keys = append(keys, k)
		}
		return vm.ToValue(keys)
	})

	if len(m.Uses) > 0 {
		routines := vm.NewObject()
		vm.Set("routines", routines)
		bind(routines, "run", func(call goja.FunctionCall) goja.Value {
			id := call.Argument(0).String()
			var params map[string]any
			if p, ok := call.Argument(1).Export().(map[string]any); ok {
				params = p
			}
			var out any
			var calls int
			var err error
			waiting(func() goja.Value {
				out, calls, err = runHelper(ctx, m, id, params, host, opt, opt.MaxCalls-res.Calls)
				return nil
			})
			res.Calls += calls
			if err != nil {
				fail(fmt.Errorf("routines.run(%q): %w", id, err))
			}
			return toJS(vm, out)
		})
	}

	clock.resume()
	defer clock.pause()
	stop := context.AfterFunc(ctx, func() { vm.Interrupt(ctx.Err()) })
	defer stop()

	if _, err := vm.RunString(code); err != nil {
		return *res, jsError(err)
	}
	run, ok := goja.AssertFunction(vm.Get("run"))
	if !ok {
		return *res, errors.New("routine does not define function run()")
	}
	v, err := run(goja.Undefined())
	if err != nil {
		return *res, jsError(err)
	}
	res.Return = v.Export()
	if p, ok := v.Export().(*goja.Promise); ok {
		switch p.State() {
		case goja.PromiseStateRejected:
			return *res, jsError(rejection(p.Result()))
		case goja.PromiseStatePending:
			return *res, errors.New("routine awaited something that never resolved")
		}
		res.Return = p.Result().Export()
	}
	return *res, nil
}

// runHelper runs a routine listed in uses, with the caller's host. It may
// only touch what the caller declares, may not start a cycle, and does not
// change its own kept state.
func runHelper(ctx context.Context, caller Manifest, id string, params map[string]any, host Host, opt Options, calls int) (any, int, error) {
	listed := false
	for _, u := range caller.Uses {
		listed = listed || u == id
	}
	if !listed {
		return nil, 0, fmt.Errorf("%s is not in this routine's uses", id)
	}
	if opt.Library == nil {
		return nil, 0, errors.New("no routines are available here")
	}
	chain := opt.chain
	if len(chain) == 0 && opt.ID != "" {
		chain = []string{opt.ID}
	}
	for _, c := range chain {
		if c == id {
			return nil, 0, fmt.Errorf("routines would run each other in a loop (%s)", strings.Join(append(chain, id), " → "))
		}
	}
	if len(chain) >= MaxDepth {
		return nil, 0, fmt.Errorf("routines may run others only %d levels deep", MaxDepth)
	}
	h, err := opt.Library(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	allowed := map[string]bool{}
	for _, c := range caller.Capabilities {
		allowed[c] = true
	}
	for _, c := range h.Manifest.Capabilities {
		if !allowed[c] {
			return nil, 0, fmt.Errorf("%s needs %s, which this routine does not declare", id, c)
		}
	}
	merged := map[string]any{}
	for k, v := range h.Params {
		merged[k] = v
	}
	for k, v := range params {
		merged[k] = v
	}
	sub := Options{Now: opt.Now, Zone: opt.Zone, Timeout: opt.Timeout, MaxCalls: max(calls, 1), Params: merged,
		State: h.State, Library: opt.Library, chain: append(append([]string{}, chain...), id)}
	res, err := Run(ctx, h.Code, h.Manifest, host, sub)
	return res.Return, res.Calls, err
}

func exportArgs(call goja.FunctionCall) any {
	if len(call.Arguments) == 0 {
		return map[string]any{}
	}
	if len(call.Arguments) == 1 {
		return call.Arguments[0].Export()
	}
	out := make([]any, len(call.Arguments))
	for i, a := range call.Arguments {
		out[i] = a.Export()
	}
	return out
}

// scopeFor returns the manifest scope that covers a call, or "" if none does.
// For http.getJSON the scope is the URL host.
func scopeFor(name string, args any, allowed []string) string {
	url, _ := args.(string)
	if m, ok := args.(map[string]any); ok {
		url, _ = m[capability.Catalog[name].ScopeArg].(string)
	}
	host := url
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	host, _, _ = strings.Cut(host, "/")
	host, _, _ = strings.Cut(host, "?")
	for _, a := range allowed {
		if strings.EqualFold(host, a) {
			return a
		}
	}
	return ""
}

// toJS round-trips through JSON so routines get plain objects and arrays
// with the field names connectors use.
func toJS(vm *goja.Runtime, v any) goja.Value {
	if v == nil {
		return goja.Null()
	}
	b, err := json.Marshal(v)
	if err != nil {
		return vm.ToValue(v)
	}
	var plain any
	if err := json.Unmarshal(b, &plain); err != nil {
		return vm.ToValue(v)
	}
	return vm.ToValue(plain)
}

func rejection(v goja.Value) error {
	if o, ok := v.(*goja.Object); ok {
		if msg := o.Get("message"); msg != nil && !goja.IsUndefined(msg) {
			return errors.New(msg.String())
		}
	}
	return fmt.Errorf("%v", v.Export())
}

func jsError(err error) error {
	var ex *goja.Exception
	if errors.As(err, &ex) {
		return errors.New(strings.TrimSpace(ex.Error()))
	}
	var intr *goja.InterruptedError
	if errors.As(err, &intr) {
		if e, ok := intr.Value().(error); ok {
			return e
		}
	}
	return err
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r == '_' || r == '$' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

// workClock measures the routine's own running time and interrupts it
// past the limit; it stands still while the routine waits on the host.
type workClock struct {
	mu    sync.Mutex
	limit time.Duration
	used  time.Duration
	since time.Time
	timer *time.Timer
	vm    *goja.Runtime
}

func (c *workClock) resume() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.since = time.Now()
	left := c.limit - c.used
	if left <= 0 {
		c.vm.Interrupt(ErrTimeout)
		return
	}
	c.timer = time.AfterFunc(left, func() { c.vm.Interrupt(ErrTimeout) })
}

func (c *workClock) pause() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timer == nil {
		return
	}
	c.timer.Stop()
	c.timer = nil
	c.used += time.Since(c.since)
}
