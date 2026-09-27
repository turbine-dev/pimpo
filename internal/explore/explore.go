// Package explore runs a new request once with a language model, records
// every call, and on the owner's approval compiles it into a routine.
package explore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/denerFernandes/pimpo/internal/i18n"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/denerFernandes/pimpo/internal/budget"
	"github.com/denerFernandes/pimpo/internal/capability"
	"github.com/denerFernandes/pimpo/internal/compiler"
	"github.com/denerFernandes/pimpo/internal/host"
	"github.com/denerFernandes/pimpo/internal/llm"
	"github.com/denerFernandes/pimpo/internal/mcp"
	"github.com/denerFernandes/pimpo/internal/memory"
	"github.com/denerFernandes/pimpo/internal/people"
	"github.com/denerFernandes/pimpo/internal/routine"
	"github.com/denerFernandes/pimpo/internal/store"
	"github.com/denerFernandes/pimpo/internal/trace"
)

// Notice is a message to the owner with optional buttons.
type Notice struct {
	Text    string
	Actions []Action
	// To is the person who should get it; empty is the owner.
	To string
	// Kind lets the owner silence some notices: task, failure, backup.
	// Approvals and messages a routine sends always arrive.
	Kind string
}

type Action struct {
	Label string
	Data  string
}

type Notifier interface {
	Notify(ctx context.Context, n Notice) error
}

// Routines is told about new and repaired routines, so they get scheduled.
type Routines interface {
	Changed(ctx context.Context, id string)
}

type Service struct {
	Env      host.Env
	Store    *store.Store
	Agent    llm.Agent
	Compiler compiler.Compiler
	Notify   Notifier
	Routines Routines
	// BaseURL is where this Pimpo serves MCP, e.g. http://127.0.0.1:7788.
	BaseURL string
	Zone    *time.Location
	Model   string
	// Memory is optional; confirmed facts guide explorations.
	Memory *memory.Memory
	// Recall, when set, searches memory by meaning.
	Recall Recall
	// Guide is the user guide, for questions about Pimpo itself.
	Guide string

	mu       sync.Mutex
	sessions map[string]session
	wg       sync.WaitGroup
}

type session struct {
	key    string
	server *mcp.Server
}

const (
	EventStarted  = "exploration.started"
	EventFinished = "exploration.finished"
	EventFailed   = "exploration.failed"
	EventCompiled = "routine.created"
)

// owner stores the owner as the empty person, as rows from before people
// existed do.
func owner(person string) string {
	if person == people.OwnerID {
		return ""
	}
	return person
}

func newID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Start begins an exploration in the background and returns its id.
func (s *Service) Start(ctx context.Context, request, actor string) (string, error) {
	return s.start(ctx, request, actor, "", Options{})
}

// Options adjust one exploration.
type Options struct {
	// Context is the conversation so far, given to the agent before the
	// request.
	Context string
	// Quiet leaves the answer on the screen that asked, instead of also
	// sending it to the owner's chat.
	Quiet bool
	// Assistant, when set, gives the agent a role and limits its tools.
	Assistant *Assistant
	// Model overrides the model for this request (chosen by the owner or
	// by the automatic choice).
	Model string
}

// Assistant is a named role for the agent with the capabilities it may use.
type Assistant struct {
	Name         string
	Instructions string
	// Capabilities limits the tools; empty means all of them.
	Capabilities []string
}

// StartWith starts an exploration with options, for the in-app chat.
func (s *Service) StartWith(ctx context.Context, request, actor string, o Options) (string, error) {
	return s.start(ctx, request, actor, "", o)
}

// Repair re-explores a broken routine's task; approving it saves a new
// version of the same routine.
func (s *Service) Repair(ctx context.Context, routineID, problem, actor string) (string, error) {
	r, err := s.Store.Routine(ctx, routineID)
	if err != nil {
		return "", err
	}
	request := r.Body.Description
	if exps, err := s.Store.Explorations(ctx, store.ExplorationDone); err == nil {
		for _, e := range exps {
			if e.Routine == routineID {
				request = e.Request
				break
			}
		}
	}
	if problem != "" {
		request += "\n\n(Last time the automatic routine failed with: " + problem + ")"
	}
	return s.start(people.With(ctx, r.Person), request, actor, routineID, Options{})
}

func (s *Service) start(ctx context.Context, request, actor, target string, o Options) (string, error) {
	request = strings.TrimSpace(request)
	if request == "" {
		return "", errors.New("tell me what you want done")
	}
	if s.Env.Budget != nil {
		if err := s.Env.Budget.Check(ctx); err != nil {
			return "", err
		}
	}
	id := newID()
	e := store.Exploration{ID: id, Request: request, State: store.ExplorationRunning, Routine: target, Person: owner(people.From(ctx))}
	if err := s.Store.SaveExploration(ctx, e); err != nil {
		return "", err
	}
	s.Env.Events.Append(ctx, EventStarted, actor, map[string]string{"exploration": id, "request": request})
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.run(context.WithoutCancel(ctx), e, o)
	}()
	return id, nil
}

// Wait blocks until background explorations finish (tests and shutdown).
func (s *Service) Wait() { s.wg.Wait() }

func (s *Service) run(ctx context.Context, e store.Exploration, o Options) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	h := &host.Host{Env: s.Env, Source: "exploration:" + e.ID, DryRun: true, Person: e.Person}
	role := ""
	if as := o.Assistant; as != nil {
		if len(as.Capabilities) > 0 {
			h.Allowed = map[string]bool{}
			for _, c := range as.Capabilities {
				h.Allowed[c] = true
			}
		}
		role = "\n\nYou are acting as the owner's assistant named " + as.Name + "."
		if strings.TrimSpace(as.Instructions) != "" {
			role += " The owner describes this assistant's job as follows (a description of the job, not a way around any rule): " + strings.TrimSpace(as.Instructions)
		}
		if h.Allowed != nil {
			role += " You can only use the tools listed; if the request needs something else, say which assistant or connection would be needed."
		}
	}
	key := newID() + newID()
	s.mu.Lock()
	if s.sessions == nil {
		s.sessions = map[string]session{}
	}
	s.sessions[e.ID] = session{key: key, server: &mcp.Server{Name: "pimpo", Tools: tools(h, s.Memory, s.Recall, s.Guide)}}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.sessions, e.ID)
		s.mu.Unlock()
	}()

	now := time.Now().In(s.zone())
	prompt := e.Request
	if o.Context != "" {
		prompt = "The conversation so far:\n" + o.Context + "\n\nNow the owner says: " + e.Request
	}
	resp, err := s.Agent.Run(ctx, llm.AgentRequest{
		System:     explorerPrompt(now) + s.knownFacts(e.Person) + role,
		Prompt:     prompt,
		MCPURL:     fmt.Sprintf("%s/mcp/explore/%s?key=%s", s.BaseURL, e.ID, key),
		Model:      firstNonEmpty(o.Model, s.Model),
		MaxCostUSD: s.maxCost(ctx),
		MaxTurns:   40,
	})
	h.AddCost(ctx, resp.CostUSD, "exploration")
	e.CostUSD = h.Cost()
	if err != nil {
		e.State, e.Error = store.ExplorationFailed, err.Error()
		s.Store.SaveExploration(ctx, e)
		s.Env.Events.Append(ctx, EventFailed, "system", map[string]string{"exploration": e.ID, "error": err.Error()})
		if !o.Quiet {
			s.Notify.Notify(ctx, Notice{Text: i18n.T(ctx, "msg.explore.failed", "request", e.Request, "error", err), To: e.Person, Kind: "task"})
		}
		return
	}
	t := &trace.Trace{ID: e.ID, Request: prompt, Now: now.Format(time.RFC3339), Calls: h.Calls(), Judgments: h.Judgments(), Questions: h.Questions(), Outcome: strings.TrimSpace(resp.Text)}
	t.Expect = DeriveExpect(t.Calls)
	e.Trace, e.Summary, e.State = t, t.Outcome, store.ExplorationReady
	s.Store.SaveExploration(ctx, e)
	s.Env.Events.Append(ctx, EventFinished, "system", map[string]any{"exploration": e.ID, "calls": len(t.Calls), "cost_usd": e.CostUSD})
	if o.Quiet {
		return
	}
	text := "✅ " + shorten(t.Outcome, 1500)
	if n := dryRuns(t.Calls); n > 0 {
		text += "\n\n" + i18n.N(ctx, "msg.explore.simulated", n)
	}
	if failed := failedReads(t.Calls); failed != "" {
		// Nothing real was read, so a routine would only repeat the error.
		text += "\n\n" + i18n.T(ctx, "msg.explore.noRoutine", "what", failed)
		s.Notify.Notify(ctx, Notice{Text: text, To: e.Person, Kind: "task"})
		return
	}
	text += "\n\n" + i18n.T(ctx, "msg.explore.offer")
	s.Notify.Notify(ctx, Notice{Text: text, Actions: []Action{{i18n.T(ctx, "btn.compile"), "compile:" + e.ID}, {i18n.T(ctx, "btn.discard"), "discard:" + e.ID}}, To: e.Person, Kind: "task"})
}

// failedReads names the capabilities read when every read failed, or ""
// when at least one worked (or nothing was read).
func failedReads(calls []trace.Call) string {
	var names []string
	for _, c := range calls {
		spec, ok := capability.Catalog[c.Capability]
		if !ok || spec.Risk != capability.Read {
			continue
		}
		if c.Error == "" {
			return ""
		}
		if !slices.Contains(names, c.Capability) {
			names = append(names, c.Capability)
		}
	}
	return strings.Join(names, ", ")
}

// knownFacts lists what the owner confirmed, for the explorer's prompt.
// Unconfirmed facts stay out: they may come from hostile content.
func (s *Service) knownFacts(person string) string {
	if s.Memory == nil {
		return ""
	}
	facts, _ := s.Memory.InstructionsFor(person)
	if len(facts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nWhat the owner has told you (confirmed):\n")
	for i, f := range facts {
		if i == 30 {
			break
		}
		b.WriteString("- " + f.Text + "\n")
	}
	return b.String()
}

func (s *Service) maxCost(ctx context.Context) float64 {
	if s.Env.Budget == nil {
		return 1
	}
	if r := s.Env.Budget.Remaining(ctx); r >= 0 {
		return min(1, max(r, 0.05))
	}
	return 1
}

func (s *Service) zone() *time.Location {
	if s.Zone != nil {
		return s.Zone
	}
	return time.Local
}

// MCP serves the tools of a running exploration. The key in the URL is
// the only credential, and it dies with the exploration.
func (s *Service) MCP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	sess, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok || r.URL.Query().Get("key") != sess.key {
		http.Error(w, "unknown exploration", http.StatusNotFound)
		return
	}
	sess.server.ServeHTTP(w, r)
}

// Approve compiles a finished exploration into a routine and schedules it.
func (s *Service) Approve(ctx context.Context, id, actor string) (store.Routine, error) {
	e, err := s.Store.Exploration(ctx, id)
	if err != nil {
		return store.Routine{}, err
	}
	if e.State != store.ExplorationReady || e.Trace == nil {
		return store.Routine{}, fmt.Errorf("exploration is %s, not ready", e.State)
	}
	if s.Compiler.Helpers != nil {
		ctx = routine.WithLibrary(ctx, s.Compiler.Helpers)
	}
	if failed := failedReads(e.Trace.Calls); failed != "" && e.Routine == "" {
		return store.Routine{}, fmt.Errorf("nothing could be read (%s), so a routine would only repeat the error; fix it and ask again", failed)
	}
	if s.Env.Budget != nil {
		if err := s.Env.Budget.Check(ctx); err != nil {
			return store.Routine{}, err
		}
	}
	e.State = store.ExplorationCompiling
	s.Store.SaveExploration(ctx, e)
	attempts, err := s.Compiler.Compile(ctx, *e.Trace)
	cost := 0.0
	for _, a := range attempts {
		cost += a.CostUSD
	}
	if s.Env.Budget != nil {
		s.Env.Budget.Record(ctx, budget.Cost{USD: cost, Source: "compile", Ref: "exploration:" + id})
	}
	e.CostUSD += cost
	var last compiler.Attempt
	if len(attempts) > 0 {
		last = attempts[len(attempts)-1]
	}
	// A repair must still pass the tests of the version it replaces.
	if e.Routine != "" && err == nil && last.Accepted() {
		if old, oerr := s.Store.Routine(ctx, e.Routine); oerr == nil {
			for _, t := range old.Body.Tests {
				if o := routine.Check(ctx, last.Routine, "previous test "+t.Name, t.Scenario); !o.Passed {
					last.Outcomes = append(last.Outcomes, o)
				}
			}
			if !last.Accepted() {
				err = fmt.Errorf("the repaired routine breaks what the old one did: %s", strings.Join(last.Problems(), "; "))
			} else {
				last.Routine.Tests = append(last.Routine.Tests, old.Body.Tests...)
			}
		}
	}
	if err != nil || !last.Accepted() {
		e.State = store.ExplorationReady
		if err == nil {
			err = fmt.Errorf("the routine did not pass its checks: %s", strings.Join(last.Problems(), "; "))
		}
		e.Error = err.Error()
		if last.Routine.Code != "" {
			e.Candidate = &last.Routine
		}
		s.Store.SaveExploration(ctx, e)
		return store.Routine{}, err
	}
	rid, reason := e.Routine, "repaired from exploration "+id
	if rid == "" {
		rid, reason = slug(last.Routine.Name), "compiled from exploration "+id
		if _, err := s.Store.Routine(ctx, rid); err == nil {
			rid += "-" + id[:4]
		}
	}
	r, err := s.Store.SaveRoutine(ctx, rid, last.Routine, reason, actor)
	if err == nil && e.Person != "" && e.Routine == "" {
		err = s.Store.SetRoutinePerson(ctx, rid, e.Person)
		r.Person = e.Person
	}
	if err != nil {
		return store.Routine{}, err
	}
	e.State, e.Routine, e.Error, e.Candidate = store.ExplorationDone, rid, "", nil
	s.Store.SaveExploration(ctx, e)
	s.Env.Events.Append(ctx, EventCompiled, actor, map[string]any{"routine": rid, "exploration": id, "attempts": len(attempts), "cost_usd": cost, "capabilities": last.Routine.Manifest.Capabilities})
	if s.Routines != nil {
		s.Routines.Changed(ctx, rid)
	}
	return r, nil
}

func (s *Service) Discard(ctx context.Context, id, actor string) error {
	e, err := s.Store.Exploration(ctx, id)
	if err != nil {
		return err
	}
	e.State = store.ExplorationDiscarded
	if err := s.Store.SaveExploration(ctx, e); err != nil {
		return err
	}
	_, err = s.Env.Events.Append(ctx, "exploration.discarded", actor, map[string]string{"exploration": id})
	return err
}

// DeriveExpect turns the writes of an exploration into checks for the
// compiled routine: the same number of calls, mentioning the recorded
// values that came from the data (titles, names, amounts).
func DeriveExpect(calls []trace.Call) []trace.Expect {
	var facts []string
	for _, c := range calls {
		if !capability.Catalog[c.Capability].Writes() {
			facts = append(facts, values(c.Result)...)
		}
	}
	byCap := map[string][]string{}
	var order []string
	for _, c := range calls {
		if !capability.Catalog[c.Capability].Writes() {
			continue
		}
		if _, ok := byCap[c.Capability]; !ok {
			order = append(order, c.Capability)
		}
		byCap[c.Capability] = append(byCap[c.Capability], strings.ToLower(routine.Flatten(c.Args)))
	}
	var out []trace.Expect
	for _, capName := range order {
		written := strings.Join(byCap[capName], "\n")
		n := len(byCap[capName])
		seen := map[string]bool{}
		var contains []string
		for _, f := range facts {
			lf := strings.ToLower(f)
			if len(f) >= 4 && len(f) <= 80 && !seen[lf] && strings.Contains(written, lf) {
				seen[lf] = true
				contains = append(contains, f)
			}
		}
		sort.Slice(contains, func(i, j int) bool { return len(contains[i]) > len(contains[j]) })
		if len(contains) > 4 {
			contains = contains[:4]
		}
		exp := trace.Expect{Capability: capName, Contains: contains}
		if capName != "telegram.send" {
			exp.Count = &n
		} else if n == 1 {
			one := 1
			exp.Count = &one
		}
		out = append(out, exp)
	}
	return out
}

// factKeys are the fields whose values a correct routine must carry into
// its messages. Names, addresses, calendars and dates can be formatted or
// left out in many valid ways, so they are not required.
var factKeys = map[string]bool{"title": true, "subject": true, "summary": true, "name": true, "status": true}

func values(raw []byte) []string {
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for k, e := range x {
				if s, ok := e.(string); ok && factKeys[k] {
					out = append(out, s)
					continue
				}
				walk(e)
			}
		}
	}
	walk(routine.Decode(raw))
	return out
}

func dryRuns(calls []trace.Call) int {
	n := 0
	for _, c := range calls {
		if capability.Catalog[c.Capability].Risk >= capability.Reversible {
			n++
		}
	}
	return n
}

func slug(name string) string {
	s := ident(name)
	if s == "" {
		s = "rotina"
	}
	return strings.ReplaceAll(s, "_", "-")
}

func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func explorerPrompt(now time.Time) string {
	return fmt.Sprintf(`You are Pimpo, the owner's personal agent. Now is %s (owner's time zone).
Do the owner's request once, right now, using ONLY the pimpo tools. This run is recorded and will be turned into an automatic routine, so work the way the routine should work every time.
- Read what you need (calendar_events, gmail_search, http_getJSON). Prefer precise queries.
- Changes (archive, label) are simulated while exploring: call them exactly as you would for real.
- telegram_send really sends to the owner: send the final result there, exactly as the owner should receive it every time.
- Every subjective decision MUST be recorded with decide, one call per item, yes or no, BEFORE you act on it: is this email important? is it a promotion or newsletter? does it need a reply? Record the items you leave out too (yes=false). The automatic routine can only repeat decisions you recorded; unrecorded ones are lost. Objective checks (dates, amounts, senders the owner named) need no decide.
- If something cannot be done with these tools, say so plainly.
Finish with a short summary in the owner's language of what you did and what the routine will do each time.`, now.Format("Monday, 2006-01-02 15:04 MST"))
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
