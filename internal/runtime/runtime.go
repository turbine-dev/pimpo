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
	"time"

	"github.com/dop251/goja"

	"github.com/denerFernandes/zodim/internal/capability"
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
}

// Watch is what a routine waits for: Zodim calls Capability with Args
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
	if m.Watch == nil && strings.TrimSpace(m.Schedule) == "" {
		return errors.New("a routine needs a schedule or something to watch")
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
	Zone    *time.Location
	Locale  string
	Timeout time.Duration
	// MaxCalls bounds capability calls per run, so a runaway loop stops.
	MaxCalls int
	// Params are the owner's values for the manifest's parameters; missing
	// ones take their defaults.
	Params map[string]any
	// Event is what woke a watching routine: {items: [...]}.
	Event any
}

type Result struct {
	Logs  []string
	Calls int
}

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
			out, err := host.Call(ctx, name, scope, args)
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
				p, err := host.Judge(ctx, name, question, call.Argument(0).Export())
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
				text, err := writer.Write(ctx, name, instruction, call.Argument(0).Export())
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

	timer := time.AfterFunc(opt.Timeout, func() { vm.Interrupt(ErrTimeout) })
	defer timer.Stop()
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
	if p, ok := v.Export().(*goja.Promise); ok {
		switch p.State() {
		case goja.PromiseStateRejected:
			return *res, jsError(rejection(p.Result()))
		case goja.PromiseStatePending:
			return *res, errors.New("routine awaited something that never resolved")
		}
	}
	return *res, nil
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
