// Package app wires Vigia together: storage, connectors, the policy, the
// explorer, the scheduler, the Telegram channel and the web API.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/denerFernandes/vigia/internal/approval"
	"github.com/denerFernandes/vigia/internal/budget"
	"github.com/denerFernandes/vigia/internal/compiler"
	"github.com/denerFernandes/vigia/internal/connector"
	"github.com/denerFernandes/vigia/internal/connector/calendar"
	"github.com/denerFernandes/vigia/internal/connector/external"
	"github.com/denerFernandes/vigia/internal/connector/mail"
	"github.com/denerFernandes/vigia/internal/connector/services"
	"github.com/denerFernandes/vigia/internal/connector/telegramcap"
	"github.com/denerFernandes/vigia/internal/connector/web"
	"github.com/denerFernandes/vigia/internal/event"
	"github.com/denerFernandes/vigia/internal/explore"
	"github.com/denerFernandes/vigia/internal/gallery"
	"github.com/denerFernandes/vigia/internal/host"
	"github.com/denerFernandes/vigia/internal/judge"
	"github.com/denerFernandes/vigia/internal/llm"
	"github.com/denerFernandes/vigia/internal/memory"
	"github.com/denerFernandes/vigia/internal/oauth"
	"github.com/denerFernandes/vigia/internal/outbox"
	"github.com/denerFernandes/vigia/internal/owner"
	"github.com/denerFernandes/vigia/internal/people"
	"github.com/denerFernandes/vigia/internal/policy"
	"github.com/denerFernandes/vigia/internal/scheduler"
	"github.com/denerFernandes/vigia/internal/server"
	"github.com/denerFernandes/vigia/internal/store"
	"github.com/denerFernandes/vigia/internal/telegram"
	"github.com/denerFernandes/vigia/internal/undo"
	"github.com/denerFernandes/vigia/internal/vault"
	"github.com/denerFernandes/vigia/internal/voice"
)

type Settings struct {
	Zone         string `json:"zone"`
	Locale       string `json:"locale"`
	JudgeBackend string `json:"judge_backend"` // local, jev, llm
	OllamaModel  string `json:"ollama_model"`
	// LocalJudgeURL is Vigia's own small judgment model (tools/judge/serve.py).
	LocalJudgeURL string `json:"local_judge_url"`
	ExploreModel  string `json:"explore_model"`
	CompileModel  string `json:"compile_model"`
	JudgeModel    string `json:"judge_model"`
	// GalleryURL is the routine gallery index; a local path works too.
	GalleryURL string `json:"gallery_url"`
	// EmailChannel lets the owner ask by writing to themselves with
	// "Vigia:" in the subject.
	EmailChannel bool `json:"email_channel"`
}

func defaultSettings() Settings {
	return Settings{Zone: time.Local.String(), Locale: "pt-BR", JudgeBackend: "local", OllamaModel: "qwen3:1.7b", LocalJudgeURL: "http://127.0.0.1:11500", ExploreModel: "sonnet", CompileModel: "sonnet", JudgeModel: "haiku", GalleryURL: gallery.DefaultIndex}
}

type App struct {
	Events    *event.Store
	Vault     *vault.Vault
	Store     *store.Store
	Budget    *budget.Budget
	Channel   *owner.Channel
	Explore   *explore.Service
	Scheduler *scheduler.Scheduler
	Server    *server.Server
	Policy    policy.Policy
	Rules     *policy.Engine
	Approvals *approval.Manager
	Outbox    *outbox.Outbox
	Undo      *undo.Undo
	Memory    *memory.Memory
	People    *people.Directory
	Google    *oauth.Google
	// LLM and Agent default to Claude Code; tests replace them.
	LLM   llm.Model
	Agent llm.Agent
	// TelegramAPI points at a self-hosted Bot API server; empty means Telegram's.
	TelegramAPI string
	// WhatsAppAPI replaces the Graph API; tests only.
	WhatsAppAPI string
	// VoiceModel is the whisper.cpp model used for voice notes.
	VoiceModel string
	// MailInsecure uses plain IMAP; tests only.
	MailInsecure bool
	// Router is shared by every run; the demo swaps connectors in it.
	Router *connector.Router
	// DemoJudge replaces the judgment backends in demo mode.
	DemoJudge judge.Judge

	mu           sync.Mutex
	listenFn     context.CancelFunc
	external     map[string]*external.Connector
	externalErrs []string
}

// New assembles an App on top of an open event store.
func New(ctx context.Context, events *event.Store, v *vault.Vault, token, baseURL string) (*App, error) {
	st, err := store.Open(events.DB())
	if err != nil {
		return nil, err
	}
	a := &App{Events: events, Vault: v, Store: st}
	a.Rules = &policy.Engine{Events: events}
	a.Policy = a.Rules
	a.LLM = claude{a}
	a.Agent = claude{a}
	set := a.Settings(ctx)
	zone := loadZone(set.Zone)
	a.Budget = &budget.Budget{Events: events, Zone: zone}
	a.Channel = &owner.Channel{Events: events, Bot: a.bot, Handler: handler{a}}
	a.People = &people.Directory{Events: events, OwnerChat: a.Channel.Chat, OwnerWhatsApp: a.ownerWhatsApp}
	a.Channel.People = a.People
	a.Channel.Mirror = a.mirrorWhatsApp
	a.Channel.Transcribe = func(ctx context.Context, audio []byte) (string, error) {
		return voice.Whisper{Model: a.VoiceModel, Language: strings.SplitN(a.Settings(ctx).Locale, "-", 2)[0]}.Transcribe(ctx, audio)
	}
	a.Approvals = &approval.Manager{Events: events, Notify: a.Channel, Describe: describeAction, Responsible: func(ctx context.Context, person string) (string, string) {
		asker, _ := a.People.Get(ctx, person)
		return a.People.Responsible(ctx, person).ID, asker.Name
	}}
	router := a.router()
	a.Router = router
	a.Outbox = &outbox.Outbox{DB: events.DB(), Events: events, Send: func(ctx context.Context, args any) (any, error) { return router.Call(ctx, "gmail.send", "", args) }}
	if err := a.Outbox.Init(); err != nil {
		return nil, err
	}
	router.Add(a.Outbox)
	a.Undo = &undo.Undo{Events: events, Outbox: a.Outbox, Mail: a.mailOps}
	env := host.Env{Router: router, Judge: judgeFunc(a.judge), Budget: a.Budget, Events: events, Policy: policyFunc(a.decide),
		Approver: approver{a.Approvals}, Remember: a.remember,
		RoleOf: func(ctx context.Context, person string) string { return string(a.People.Role(ctx, person)) }}
	a.Scheduler = &scheduler.Scheduler{Env: env, Store: st, Notify: a.Channel, Zone: zone}
	a.Explore = &explore.Service{Env: env, Store: st, Agent: agentFunc(a.runAgent), Compiler: compiler.Compiler{Model: modelFunc(a.generate), Attempts: 2},
		Notify: a.Channel, Routines: a.Scheduler, BaseURL: baseURL, Zone: zone}
	a.Server = server.New(events, token)
	a.googleRoutes()
	a.routes()
	a.safetyRoutes()
	a.memoryRoutes()
	a.migrateRoutes()
	a.pairingRoutes()
	a.peopleRoutes()
	a.whatsappRoutes()
	a.galleryRoutes()
	a.catalogRoutes()
	return a, nil
}

// Start runs the scheduler and the Telegram listener until ctx ends.
func (a *App) Start(ctx context.Context) error {
	if err := a.Scheduler.Start(ctx); err != nil {
		return err
	}
	go a.Outbox.Run(ctx, 15*time.Second)
	go a.emailChannel(ctx, time.Minute)
	a.restartListener(ctx)
	return nil
}

func (a *App) restartListener(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.listenFn != nil {
		a.listenFn()
	}
	lctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	a.listenFn = cancel
	if a.bot(ctx) == nil {
		return
	}
	go a.Channel.Listen(lctx)
}

func loadZone(name string) *time.Location {
	if l, err := time.LoadLocation(name); err == nil {
		return l
	}
	return time.Local
}

func (a *App) Settings(ctx context.Context) Settings {
	s := defaultSettings()
	if raw, _ := a.Events.Get(ctx, "settings"); raw != "" {
		json.Unmarshal([]byte(raw), &s)
	}
	return s
}

func (a *App) SaveSettings(ctx context.Context, s Settings, actor string) error {
	if _, err := time.LoadLocation(s.Zone); err != nil {
		return server.StatusError{Status: 400, Msg: "unknown time zone " + s.Zone}
	}
	switch s.JudgeBackend {
	case "local", "jev", "llm":
	default:
		return server.StatusError{Status: 400, Msg: "judge backend must be local, jev or llm"}
	}
	b, _ := json.Marshal(s)
	if err := a.Events.Put(ctx, "settings", string(b)); err != nil {
		return err
	}
	_, err := a.Events.Append(ctx, "settings.changed", actor, s)
	return err
}

func (a *App) secret(ctx context.Context, name string) (string, error) {
	v, err := a.Vault.Get(ctx, name)
	if errors.Is(err, vault.ErrNotFound) {
		return "", fmt.Errorf("%s is not set up; open Connections", strings.SplitN(name, ".", 2)[0])
	}
	return v, err
}

// bot builds a Telegram client from the stored token on every use, so a
// new token takes effect without a restart.
func (a *App) bot(ctx context.Context) owner.Bot {
	tok, err := a.Vault.Get(ctx, "telegram.token")
	if err != nil || tok == "" {
		return nil
	}
	return telegram.Bot{Token: tok, BaseURL: a.TelegramAPI}
}

type botSender struct{ a *App }

func (b botSender) Send(ctx context.Context, chat int64, text string, rows ...[]telegram.Button) (telegram.Message, error) {
	bot := b.a.bot(ctx)
	if bot == nil {
		return telegram.Message{}, errors.New("Telegram is not set up; open Connections")
	}
	return bot.Send(ctx, chat, text, rows...)
}

func (a *App) router() *connector.Router {
	cal := &calendar.Calendar{TTL: time.Minute, Feeds: func(ctx context.Context) ([]string, error) {
		raw, err := a.secret(ctx, personal(ctx, "calendar.feeds"))
		if err != nil {
			return nil, err
		}
		var urls []string
		for _, l := range strings.Split(raw, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				urls = append(urls, l)
			}
		}
		return urls, nil
	}}
	r := connector.NewRouter(
		zoned{a, cal},
		mailConn{a},
		&web.Web{},
		&telegramcap.Owner{Bot: botSender{a}, Chat: a.personChat},
		whatsappCap{a},
	)
	for _, k := range services.All() {
		r.Add(k.Connector(a.catalogConfig))
	}
	return r
}

// personChat is the Telegram chat of whoever the run works for.
func (a *App) personChat(ctx context.Context) (int64, error) {
	p, err := a.People.Get(ctx, people.From(ctx))
	if err != nil {
		return 0, err
	}
	return p.Chat, nil
}

// personal names a setting or secret of whoever ctx acts for. The owner's
// keep the names they had before people existed.
func personal(ctx context.Context, name string) string {
	if p := people.From(ctx); p != people.OwnerID {
		return "person." + p + "." + name
	}
	return name
}

// zoned gives the calendar the owner's current zone on every call.
type zoned struct {
	a   *App
	cal *calendar.Calendar
}

func (z zoned) Capabilities() []string { return z.cal.Capabilities() }
func (z zoned) Call(ctx context.Context, c, s string, args any) (any, error) {
	zone := loadZone(z.a.Settings(ctx).Zone)
	if src, _ := z.a.Events.Get(ctx, "calendar.source"); src == "google" && z.a.Google != nil && people.From(ctx) == people.OwnerID {
		return (&calendar.Google{Token: z.a.Google.Token, Zone: zone}).Call(ctx, c, s, args)
	}
	z.cal.Zone = zone
	return z.cal.Call(ctx, c, s, args)
}

// mailConn reads the account settings on every call.
type mailConn struct{ a *App }

func (m mailConn) Capabilities() []string { return (&mail.Mail{}).Capabilities() }
func (m mailConn) Call(ctx context.Context, c, s string, args any) (any, error) {
	// Each person reads only their own mailbox; a member without one set
	// up gets an error, never the owner's.
	addr, _ := m.a.Events.Get(ctx, personal(ctx, "mail.addr"))
	user, _ := m.a.Events.Get(ctx, personal(ctx, "mail.user"))
	if addr == "" || user == "" {
		return nil, errors.New("email is not set up; open Connections")
	}
	smtp, _ := m.a.Events.Get(ctx, personal(ctx, "mail.smtp"))
	pw := personal(ctx, "mail.password")
	acct := mail.Account{Addr: addr, Username: user, SMTP: smtp, Insecure: m.a.MailInsecure, Password: func(ctx context.Context) (string, error) { return m.a.secret(ctx, pw) }}
	if auth, _ := m.a.Events.Get(ctx, personal(ctx, "mail.auth")); auth == "oauth" && m.a.Google != nil && people.From(ctx) == people.OwnerID {
		acct.Token = m.a.Google.Token
	}
	return (&mail.Mail{Account: acct}).Call(ctx, c, s, args)
}

func (a *App) judge(ctx context.Context, question string, item any) (judge.Answer, error) {
	if a.DemoJudge != nil {
		return a.DemoJudge.Ask(ctx, question, item)
	}
	set := a.Settings(ctx)
	backends := map[string]judge.Judge{
		"local": judge.Chain{judge.Local{URL: set.LocalJudgeURL}, judge.Ollama{Model: set.OllamaModel}},
		"jev":   judge.Jev{Key: func(ctx context.Context) (string, error) { return a.secret(ctx, "typesafe.key") }},
		"llm":   judge.LLM{Model: a.LLM, Name: set.JudgeModel},
	}
	_, noJev := a.Vault.Get(ctx, "typesafe.key")
	usable := func(n string) bool { return n != "jev" || noJev == nil }
	if set.JudgeBackend == "local" {
		// The small local model answers most questions for free; the ones
		// it is unsure about go to a stronger judge.
		var strong judge.Chain
		for _, n := range []string{"jev", "llm"} {
			if usable(n) {
				strong = append(strong, backends[n])
			}
		}
		return judge.Cascade{First: backends["local"], Then: strong, Band: 0.4}.Ask(ctx, question, item)
	}
	var chain judge.Chain
	seen := map[string]bool{}
	for _, n := range []string{set.JudgeBackend, "jev", "llm", "local"} {
		if backends[n] != nil && usable(n) && !seen[n] {
			seen[n] = true
			chain = append(chain, backends[n])
		}
	}
	return chain.Ask(ctx, question, item)
}

func (a *App) decide(ctx context.Context, act policy.Action) policy.Decision {
	return a.Policy.Decide(ctx, act)
}

func (a *App) generate(ctx context.Context, r llm.Request) (llm.Response, error) {
	if r.Model == "" {
		r.Model = a.Settings(ctx).CompileModel
	}
	return a.LLM.Generate(ctx, r)
}

func (a *App) runAgent(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
	if r.Model == "" {
		r.Model = a.Settings(ctx).ExploreModel
	}
	return a.Agent.Run(ctx, r)
}

// claude is the default model backend: Claude Code with the owner's login.
type claude struct{ a *App }

func (c claude) Generate(ctx context.Context, r llm.Request) (llm.Response, error) {
	return llm.ClaudeCLI{}.Generate(ctx, r)
}
func (c claude) Run(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
	return llm.ClaudeCLI{}.Run(ctx, r)
}

func claudeInstalled() bool { _, err := exec.LookPath("claude"); return err == nil }

type judgeFunc func(context.Context, string, any) (judge.Answer, error)

func (f judgeFunc) Ask(ctx context.Context, q string, item any) (judge.Answer, error) {
	return f(ctx, q, item)
}

type policyFunc func(context.Context, policy.Action) policy.Decision

func (f policyFunc) Decide(ctx context.Context, a policy.Action) policy.Decision { return f(ctx, a) }

type modelFunc func(context.Context, llm.Request) (llm.Response, error)

func (f modelFunc) Generate(ctx context.Context, r llm.Request) (llm.Response, error) {
	return f(ctx, r)
}

type agentFunc func(context.Context, llm.AgentRequest) (llm.Response, error)

func (f agentFunc) Run(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
	return f(ctx, r)
}

// handler answers the owner on Telegram.
type handler struct{ a *App }

func actor(ctx context.Context) string { return "human:" + people.From(ctx) }

func (h handler) Request(ctx context.Context, text string) (string, error) {
	if _, err := h.a.Explore.Start(ctx, text, actor(ctx)); err != nil {
		return "", err
	}
	return "Entendi. Vou fazer agora e te mostro o resultado. 🔎", nil
}

func (h handler) Button(ctx context.Context, action, id string) (string, error) {
	if err := h.allowed(ctx, action, id); err != nil {
		return "", err
	}
	who := actor(ctx)
	switch action {
	case "compile":
		r, err := h.a.Explore.Approve(ctx, id, who)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Rotina \"%s\" criada. Próxima execução: %s.", r.Body.Name, h.a.nextText(ctx, r.ID)), nil
	case "discard":
		return "Descartado.", h.a.Explore.Discard(ctx, id, who)
	case "run":
		h.a.Store.SetRoutineState(ctx, id, store.RoutineActive)
		h.a.Scheduler.Changed(ctx, id)
		if _, err := h.a.Scheduler.RunNow(ctx, id, "owner"); err != nil {
			return "", err
		}
		return "Rodou de novo e deu certo. Rotina reativada.", nil
	case "repair":
		if _, err := h.a.Explore.Repair(ctx, id, "", "human:owner"); err != nil {
			return "", err
		}
		return "Vou refazer com o agente e te mostro.", nil
	case "approve", "always", "deny", "batch":
		ans := map[string]approval.Answer{"approve": approval.Once, "always": approval.Always, "deny": approval.Deny, "batch": approval.Run}[action]
		if ans == approval.Always && people.From(ctx) != people.OwnerID {
			// Only the owner makes lasting rules.
			ans, action = approval.Once, "approve"
		}
		if !h.a.Approvals.Resolve(ctx, id, ans, who) {
			return "", fmt.Errorf("este pedido já não está esperando")
		}
		return map[string]string{"approve": "Permitido.", "batch": "Permitido para o resto desta execução.", "always": "Permitido, e não pergunto mais.", "deny": "Negado."}[action], nil
	}
	return "", fmt.Errorf("unknown action %q", action)
}

// allowed keeps members to their own explorations and to the approvals
// they answer for; routines and everything else stay with the owner.
func (h handler) allowed(ctx context.Context, action, id string) error {
	person := people.From(ctx)
	if person == people.OwnerID {
		return nil
	}
	switch action {
	case "approve", "always", "deny", "batch":
		if h.a.Approvals.MayAnswer(id, person) {
			return nil
		}
	case "compile", "discard":
		if e, err := h.a.Store.Exploration(ctx, id); err == nil && e.Person == person {
			return nil
		}
	}
	return errors.New("só o dono da casa pode fazer isso")
}

func (a *App) nextText(ctx context.Context, id string) string {
	n := a.Scheduler.Next(id)
	if n.IsZero() {
		return "não agendada"
	}
	return n.In(loadZone(a.Settings(ctx).Zone)).Format("02/01 15:04")
}

func budgetCost(usd float64, source string) budget.Cost { return budget.Cost{USD: usd, Source: source} }
