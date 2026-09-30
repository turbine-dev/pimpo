package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/connector/services"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/policy"
	"github.com/turbine-dev/pimpo/internal/server"
)

// Settings history: every change to rules, the budget, connections,
// models, people and the other settings, with who made it and when, kept
// in the event log so it is as tamper-evident as the rest. A secret is
// never recorded, not even encrypted: its field only says it was added,
// replaced or removed, and undo cannot bring it back.
//
// Whose history an entry is: the house settings are the owner's; a
// member's own accounts are that member's alone, even when the owner set
// them up for someone who had not signed in yet.

const historyEvent = "settings.history"

// Where a change is stored, which says how to read it again and undo it.
const (
	histSettings = "settings" // fields of Settings
	histKV       = "kv"       // stored values and vault secrets, by name
	histRules    = "rules"    // rules by id, the preset and web hosts
	histPerson   = "person"   // a person's name, role and responsible
)

type histField struct {
	Name   string          `json:"field"`
	Before json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage `json:"after,omitempty"`
	// Secret is added, replaced or removed; its values are never kept.
	Secret string `json:"secret,omitempty"`
}

type histEntry struct {
	Area   string      `json:"area"`
	Target string      `json:"target,omitempty"`
	Store  string      `json:"store"`
	Person string      `json:"person"`
	Fields []histField `json:"fields"`
	UndoOf int64       `json:"undo_of,omitempty"`
}

// snapVal is one field as it is now. A secret's value stays in memory,
// only to tell whether it changed.
type snapVal struct {
	raw    string
	secret bool
}

type snap map[string]snapVal

var secretWords = []string{"key", "token", "secret", "password", "passphrase", "refresh", "credential", "credentials", "feeds", "apikey", "invite"}

// secretName says whether a field's name says it holds a secret, whatever
// the store it came from.
func secretName(name string) bool {
	for _, part := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return r == '.' || r == '_' || r == '-' || r == ':' }) {
		for _, w := range secretWords {
			if part == w || strings.HasSuffix(part, w) && w != "feeds" {
				return true
			}
		}
	}
	return false
}

// scrubbed says whether a value holds a secret of its own: a field named
// like one inside it, or a link with a password in it.
func scrubbed(raw string) bool {
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return false
	}
	var walk func(any) bool
	walk = func(v any) bool {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				if secretName(k) || walk(e) {
					return true
				}
			}
		case []any:
			return slices.ContainsFunc(x, walk)
		case string:
			if u, err := url.Parse(x); err == nil && u.User != nil && strings.Contains(x, "://") {
				return true
			}
		}
		return false
	}
	return walk(v)
}

// diff lists what changed between two snapshots, secrets hidden. byName
// also hides fields named like a secret, for stores whose field names are
// the names things are kept under.
func diff(before, after snap, byName bool) []histField {
	names := []string{}
	for n := range before {
		names = append(names, n)
	}
	for n := range after {
		if _, ok := before[n]; !ok {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	out := []histField{}
	for _, n := range names {
		b, hadB := before[n]
		a, hadA := after[n]
		if hadB == hadA && b.raw == a.raw {
			continue
		}
		if b.secret || a.secret || (byName && secretName(n)) || (hadB && scrubbed(b.raw)) || (hadA && scrubbed(a.raw)) {
			state := "replaced"
			if !hadB {
				state = "added"
			} else if !hadA {
				state = "removed"
			}
			out = append(out, histField{Name: n, Secret: state})
			continue
		}
		f := histField{Name: n}
		if hadB {
			f.Before = json.RawMessage(b.raw)
		}
		if hadA {
			f.After = json.RawMessage(a.raw)
		}
		out = append(out, f)
	}
	return out
}

// track reads the fields before a change and returns what records it once
// the change is done; a change that fails is simply not recorded.
func (a *App) track(ctx context.Context, e histEntry) func() {
	before := a.current(ctx, e)
	return func() { a.record(ctx, e, before) }
}

func (a *App) record(ctx context.Context, e histEntry, before snap) {
	e.Fields = diff(before, a.current(ctx, e), e.Store == histKV || e.Store == histSettings)
	if len(e.Fields) == 0 {
		return
	}
	e.Person = people.Norm(e.Person)
	if e.UndoOf == 0 {
		e.UndoOf = undoOf(ctx)
	}
	a.Events.Append(ctx, historyEvent, actor(ctx), e)
}

type undoKey struct{}

func undoOf(ctx context.Context) int64 { id, _ := ctx.Value(undoKey{}).(int64); return id }

// Reading the fields of each store.

func (a *App) current(ctx context.Context, e histEntry) snap {
	switch e.Store {
	case histSettings:
		return a.settingsSnap(ctx, e.Area)
	case histRules:
		return a.rulesSnap(ctx)
	case histPerson:
		return a.personSnap(ctx, e.Target)
	}
	return a.kvSnap(ctx, e.Fields)
}

// modelSettings are the settings that belong to Models in the history.
var modelSettings = map[string]bool{"models": true, "explore_model": true, "compile_model": true, "judge_model": true, "fallbacks": true, "efforts": true,
	"ollama_url": true, "ollama_model": true, "lmstudio_url": true, "custom_url": true, "auto_off": true, "auto_light": true, "auto_strong": true,
	"judge_backend": true, "local_judge_url": true}

func (a *App) settingsSnap(ctx context.Context, area string) snap {
	b, _ := json.Marshal(a.Settings(ctx))
	var m map[string]json.RawMessage
	json.Unmarshal(b, &m)
	out := snap{}
	for k, v := range m {
		if modelSettings[k] == (area == "models") {
			out[k] = snapVal{raw: string(v)}
		}
	}
	return out
}

func (a *App) rulesSnap(ctx context.Context) snap {
	out := snap{}
	for _, r := range a.Rules.Rules(ctx) {
		b, _ := json.Marshal(r)
		out["rule:"+r.ID] = snapVal{raw: string(b)}
	}
	if p, _ := a.Events.Get(ctx, "setup.preset"); p != "" {
		out["preset"] = snapVal{raw: strconv.Quote(p)}
	}
	for h, ok := range a.Rules.Hosts(ctx) {
		out["host:"+h] = snapVal{raw: strconv.FormatBool(ok)}
	}
	return out
}

func (a *App) personSnap(ctx context.Context, id string) snap {
	p, err := a.People.Get(ctx, id)
	if err != nil {
		return snap{}
	}
	return snap{"name": {raw: strconv.Quote(p.Name)}, "role": {raw: strconv.Quote(string(p.Role))}, "responsible": {raw: strconv.Quote(p.Responsible)}}
}

// kvSnap reads stored values and vault secrets by name. A field marked
// secret, with its name starting "vault:", is read from the vault.
func (a *App) kvSnap(ctx context.Context, fields []histField) snap {
	out := snap{}
	for _, f := range fields {
		if name, ok := strings.CutPrefix(f.Name, "vault:"); ok {
			if v, err := a.Vault.Get(ctx, name); err == nil && v != "" {
				out[f.Name] = snapVal{raw: v, secret: true}
			}
			continue
		}
		if v, _ := a.Events.Get(ctx, f.Name); v != "" {
			out[f.Name] = snapVal{raw: strconv.Quote(v)}
		}
	}
	return out
}

// kvEntry is a change to stored values and vault secrets by name.
func kvEntry(area, target, person string, values []string, secrets ...string) histEntry {
	e := histEntry{Area: area, Target: target, Store: histKV, Person: person}
	for _, n := range values {
		e.Fields = append(e.Fields, histField{Name: n})
	}
	for _, n := range secrets {
		e.Fields = append(e.Fields, histField{Name: "vault:" + n})
	}
	return e
}

// connectionKeys are where each built-in connection keeps its settings.
func connectionKeys(kind string) (values, secrets []string) {
	switch kind {
	case "telegram":
		return nil, []string{"telegram.token"}
	case "mail":
		return []string{"mail.addr", "mail.user"}, []string{"mail.password"}
	case "calendar":
		return nil, []string{"calendar.feeds"}
	case "jev":
		return nil, []string{"typesafe.key"}
	case "whatsapp":
		return []string{"whatsapp.phone_id"}, []string{"whatsapp.token", "whatsapp.app_secret", "whatsapp.verify_token"}
	}
	return nil, nil
}

// catalogEntry is a change to a catalog connector of the person asking.
func (a *App) catalogEntry(ctx context.Context, kind string) histEntry {
	var values, secrets []string
	if c := a.externalConnector(kind); c != nil {
		for _, e := range append(append([]string{}, c.Env...), c.Headers...) {
			secrets = append(secrets, "connector."+c.Name+"."+e)
		}
	} else if k, ok := services.Get(kind); ok {
		for _, f := range k.Fields {
			name := personal(ctx, catalogKey(k.ID, f.Name))
			if f.Secret {
				secrets = append(secrets, name)
			} else {
				values = append(values, name)
			}
		}
	}
	return kvEntry("connections", kind, people.From(ctx), values, secrets...)
}

// The history API.

func (a *App) historyRoutes() {
	a.Server.Handle("GET /api/history", a.listHistory)
	a.Server.Handle("POST /api/history/{id}/undo", a.undoHistory)
}

type historyView struct {
	ID     int64       `json:"id"`
	Time   time.Time   `json:"ts"`
	Actor  string      `json:"actor"`
	Who    string      `json:"who"`
	Area   string      `json:"area"`
	Target string      `json:"target,omitempty"`
	Fields []histField `json:"fields"`
	UndoOf int64       `json:"undo_of,omitempty"`
	// Undoable says undo would put the earlier values back; Reenter are the
	// secrets it cannot, to type again.
	Undoable bool     `json:"undoable"`
	Undone   bool     `json:"undone"`
	Reenter  []string `json:"reenter"`
}

// myHistory is the history of the person asking, newest first.
func (a *App) myHistory(ctx context.Context) ([]event.Event, []histEntry, error) {
	evs, err := a.Events.List(ctx, event.Query{Types: []string{historyEvent}, Newest: true})
	if err != nil {
		return nil, nil, err
	}
	var keep []event.Event
	var entries []histEntry
	for _, e := range evs {
		var h histEntry
		if e.Decode(&h) != nil || !mine(ctx, h.Person) {
			continue
		}
		keep, entries = append(keep, e), append(entries, h)
	}
	return keep, entries, nil
}

func (a *App) listHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	evs, entries, err := a.myHistory(ctx)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	area := r.URL.Query().Get("area")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	undone := map[int64]bool{}
	for _, h := range entries {
		if h.UndoOf != 0 {
			undone[h.UndoOf] = true
		}
	}
	out := []historyView{}
	for i, e := range evs {
		h := entries[i]
		if area != "" && h.Area != area {
			continue
		}
		if len(out) == limit {
			break
		}
		ok, reenter := undoable(h)
		out = append(out, historyView{ID: e.ID, Time: e.Time, Actor: e.Actor, Who: a.actorName(ctx, e.Actor),
			Area: h.Area, Target: h.Target, Fields: h.Fields, UndoOf: h.UndoOf, Undoable: ok && !undone[e.ID], Undone: undone[e.ID], Reenter: reenter})
	}
	server.WriteJSON(w, 200, out)
}

// undoable says whether undo can put an entry's earlier values back, and
// which secrets it cannot.
func undoable(h histEntry) (bool, []string) {
	reenter := []string{}
	plain := 0
	for _, f := range h.Fields {
		if f.Secret != "" {
			reenter = append(reenter, strings.TrimPrefix(f.Name, "vault:"))
			continue
		}
		// Adding or removing a person is not undone: removing forgets
		// everything they kept.
		if h.Store == histPerson && (f.Before == nil || f.After == nil) {
			return false, reenter
		}
		plain++
	}
	return plain > 0, reenter
}

// actorName is how the history names who made a change.
func (a *App) actorName(ctx context.Context, actor string) string {
	id, ok := strings.CutPrefix(actor, "human:")
	if !ok {
		return ""
	}
	if people.Norm(id) == people.OwnerID {
		acc, _ := a.admin(ctx)
		return acc.Name
	}
	if p, err := a.People.Get(ctx, id); err == nil {
		return p.Name
	}
	return ""
}

type historyProblem struct {
	status  int
	problem string
	msg     string
}

func (a *App) undoHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	fail := func(p historyProblem) {
		server.WriteJSON(w, p.status, map[string]string{"error": p.msg, "problem": p.problem})
	}
	evs, entries, err := a.myHistory(ctx)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	i := slices.IndexFunc(evs, func(e event.Event) bool { return e.ID == id })
	if i < 0 {
		fail(historyProblem{404, "not_found", "no such change"})
		return
	}
	h := entries[i]
	if slices.ContainsFunc(entries, func(o histEntry) bool { return o.UndoOf == id }) {
		fail(historyProblem{409, "undone", "this change was already undone"})
		return
	}
	ok, reenter := undoable(h)
	if !ok {
		fail(historyProblem{409, "not_undoable", "this change cannot be undone; a secret is typed again, and a person is added or removed again"})
		return
	}
	// Undo only puts back what is still as the change left it; anything
	// changed since is undone from the newest change first.
	now := a.current(ctx, h)
	for _, f := range h.Fields {
		if f.Secret != "" {
			continue
		}
		cur, has := now[f.Name]
		if has != (f.After != nil) || (has && !sameJSON(cur.raw, string(f.After))) {
			fail(historyProblem{409, "changed_since", "this was changed again since; undo the newer change first"})
			return
		}
	}
	uctx := context.WithValue(ctx, undoKey{}, id)
	done := a.track(uctx, h)
	if err := a.restore(uctx, h); err != nil {
		fail(historyProblem{400, "failed", err.Error()})
		return
	}
	done()
	server.WriteJSON(w, 200, map[string]any{"state": "undone", "reenter": reenter})
}

func sameJSON(x, y string) bool {
	var a, b any
	if json.Unmarshal([]byte(x), &a) != nil || json.Unmarshal([]byte(y), &b) != nil {
		return x == y
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// restore puts an entry's earlier values back, secrets left as they are.
func (a *App) restore(ctx context.Context, h histEntry) error {
	var plain []histField
	for _, f := range h.Fields {
		if f.Secret == "" {
			plain = append(plain, f)
		}
	}
	switch h.Store {
	case histSettings:
		return a.restoreSettings(ctx, plain)
	case histRules:
		return a.restoreRules(ctx, plain)
	case histPerson:
		return a.restorePerson(ctx, h.Target, plain)
	}
	for _, f := range plain {
		var v string
		json.Unmarshal(f.Before, &v)
		if err := a.Events.Put(ctx, f.Name, v); err != nil {
			return err
		}
	}
	if h.Area == "connections" && slices.Contains(services.LinkKinds, h.Target) {
		a.restartLink(ctx, h.Target)
	}
	return nil
}

func (a *App) restoreSettings(ctx context.Context, fields []histField) error {
	b, _ := json.Marshal(a.Settings(ctx))
	var m map[string]json.RawMessage
	json.Unmarshal(b, &m)
	for _, f := range fields {
		if f.Before == nil {
			delete(m, f.Name)
		} else {
			m[f.Name] = f.Before
		}
	}
	b, _ = json.Marshal(m)
	var s Settings
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	return a.saveSettings(ctx, s, actor(ctx))
}

func (a *App) restoreRules(ctx context.Context, fields []histField) error {
	rules := a.Rules.Rules(ctx)
	for _, f := range fields {
		switch {
		case strings.HasPrefix(f.Name, "rule:"):
			id := strings.TrimPrefix(f.Name, "rule:")
			i := slices.IndexFunc(rules, func(r policy.Rule) bool { return r.ID == id })
			if f.Before == nil {
				if i >= 0 {
					rules = slices.Delete(rules, i, i+1)
				}
				continue
			}
			var r policy.Rule
			if err := json.Unmarshal(f.Before, &r); err != nil {
				return err
			}
			if i >= 0 {
				rules[i] = r
			} else {
				rules = append(rules, r)
			}
		case f.Name == "preset":
			var p string
			json.Unmarshal(f.Before, &p)
			a.Events.Put(ctx, "setup.preset", p)
		case strings.HasPrefix(f.Name, "host:"):
			host := strings.TrimPrefix(f.Name, "host:")
			var err error
			if f.Before == nil {
				err = a.Rules.ForgetHost(ctx, host, actor(ctx))
			} else {
				err = a.Rules.AllowHost(ctx, host, string(f.Before) == "true", actor(ctx))
			}
			if err != nil {
				return err
			}
		}
	}
	return a.Rules.SaveRules(ctx, rules, actor(ctx))
}

func (a *App) restorePerson(ctx context.Context, id string, fields []histField) error {
	p, err := a.People.Get(ctx, id)
	if err != nil {
		return err
	}
	role, resp := p.Role, p.Responsible
	for _, f := range fields {
		var v string
		json.Unmarshal(f.Before, &v)
		switch f.Name {
		case "role":
			role = people.Role(v)
		case "responsible":
			resp = v
		}
	}
	q, err := a.People.Update(ctx, id, role, resp)
	if err != nil {
		return err
	}
	// The person changed learns of it, as with any change of role.
	a.Events.Append(ctx, "person.role_changed", "system", map[string]any{"to": q.ID, "role": q.Role, "responsible": q.Responsible})
	return nil
}

// changeRules records whatever change leaves the rules, the preset or the
// web hosts different.
func (a *App) changeRules(ctx context.Context, change func() error) error {
	done := a.track(ctx, histEntry{Area: "rules", Store: histRules, Person: people.OwnerID})
	if err := change(); err != nil {
		return err
	}
	done()
	return nil
}
