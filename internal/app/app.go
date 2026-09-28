// Package app wires Pimpo together: storage, connectors, the policy, the
// explorer, the scheduler, the Telegram channel and the web API.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/denerFernandes/pimpo/internal/i18n"
	"github.com/denerFernandes/pimpo/internal/models"
	"github.com/denerFernandes/pimpo/internal/runtime"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/denerFernandes/pimpo/docs"
	"github.com/denerFernandes/pimpo/internal/approval"
	"github.com/denerFernandes/pimpo/internal/budget"
	"github.com/denerFernandes/pimpo/internal/compiler"
	"github.com/denerFernandes/pimpo/internal/connector"
	"github.com/denerFernandes/pimpo/internal/connector/calendar"
	"github.com/denerFernandes/pimpo/internal/connector/external"
	"github.com/denerFernandes/pimpo/internal/connector/mail"
	"github.com/denerFernandes/pimpo/internal/connector/services"
	"github.com/denerFernandes/pimpo/internal/connector/telegramcap"
	"github.com/denerFernandes/pimpo/internal/connector/web"
	"github.com/denerFernandes/pimpo/internal/desktop"
	"github.com/denerFernandes/pimpo/internal/event"
	"github.com/denerFernandes/pimpo/internal/explore"
	"github.com/denerFernandes/pimpo/internal/gallery"
	"github.com/denerFernandes/pimpo/internal/host"
	"github.com/denerFernandes/pimpo/internal/judge"
	"github.com/denerFernandes/pimpo/internal/llm"
	"github.com/denerFernandes/pimpo/internal/local"
	"github.com/denerFernandes/pimpo/internal/memory"
	"github.com/denerFernandes/pimpo/internal/oauth"
	"github.com/denerFernandes/pimpo/internal/ocr"
	"github.com/denerFernandes/pimpo/internal/outbox"
	"github.com/denerFernandes/pimpo/internal/owner"
	"github.com/denerFernandes/pimpo/internal/people"
	"github.com/denerFernandes/pimpo/internal/policy"
	"github.com/denerFernandes/pimpo/internal/protect"
	"github.com/denerFernandes/pimpo/internal/remote"
	"github.com/denerFernandes/pimpo/internal/scheduler"
	"github.com/denerFernandes/pimpo/internal/server"
	"github.com/denerFernandes/pimpo/internal/speech"
	"github.com/denerFernandes/pimpo/internal/store"
	"github.com/denerFernandes/pimpo/internal/telegram"
	"github.com/denerFernandes/pimpo/internal/undo"
	"github.com/denerFernandes/pimpo/internal/vault"
	"github.com/denerFernandes/pimpo/internal/voice"
)

type Settings struct {
	Zone         string `json:"zone"`
	Locale       string `json:"locale"`
	JudgeBackend string `json:"judge_backend"` // local, jev, llm
	OllamaModel  string `json:"ollama_model"`
	// LocalJudgeURL is Pimpo's own small judgment model (tools/judge/serve.py).
	LocalJudgeURL string `json:"local_judge_url"`
	ExploreModel  string `json:"explore_model"`
	CompileModel  string `json:"compile_model"`
	JudgeModel    string `json:"judge_model"`
	// GalleryURL is the routine gallery index; a local path works too.
	GalleryURL string `json:"gallery_url"`
	// EmailChannel lets the owner ask by writing to themselves with
	// "Pimpo:" in the subject.
	EmailChannel bool `json:"email_channel"`
	// ProtectionNetwork downloads the shared protection list daily.
	ProtectionNetwork bool   `json:"protection_network"`
	ProtectionURL     string `json:"protection_url"`
	// Mute silences kinds of notices outside the app: task, failure,
	// backup. Approvals are never silenced.
	Mute []string `json:"mute,omitempty"`
	// LabsOff turns off newer features: memory_organize, meaning_search,
	// mcp_registry.
	LabsOff []string `json:"labs_off,omitempty"`
	// Models are API models the owner set up, with their price; the model
	// settings above may name one as provider:model.
	Models    []ModelOption `json:"models,omitempty"`
	OllamaURL string        `json:"ollama_url,omitempty"`
	// LMStudioURL and CustomURL are where LM Studio and an OpenAI-compatible
	// server of the owner's answer (without /v1 for LM Studio).
	LMStudioURL string `json:"lmstudio_url,omitempty"`
	CustomURL   string `json:"custom_url,omitempty"`
	// Fallbacks are tried in order when a job's model fails: explore,
	// compile and judge.
	Fallbacks map[string][]string `json:"fallbacks,omitempty"`
	// Efforts are how hard each job's model thinks by default (explore,
	// compile, judge): low, medium, high or max; missing is the model's own.
	Efforts map[string]string `json:"efforts,omitempty"`
	// Voice is what reads audio aloud: auto (a downloaded voice for the
	// language, else the system's), local, system, openai or elevenlabs;
	// VoiceModel and VoiceName pick the cloud model and voice.
	Voice      string `json:"voice,omitempty"`
	VoiceModel string `json:"voice_model,omitempty"`
	VoiceName  string `json:"voice_name,omitempty"`
	// ChatVoice reads answers aloud in the chat: "" is the same voice as
	// routines, browser is the browser's own (instant), or any engine
	// above with its ChatVoiceModel and ChatVoiceName.
	ChatVoice      string `json:"chat_voice,omitempty"`
	ChatVoiceModel string `json:"chat_voice_model,omitempty"`
	ChatVoiceName  string `json:"chat_voice_name,omitempty"`
	// AutoOff stops the automatic model choice in chats; AutoLight and
	// AutoStrong override the models it sends simple and hard requests to.
	AutoOff    bool   `json:"auto_off,omitempty"`
	AutoLight  string `json:"auto_light,omitempty"`
	AutoStrong string `json:"auto_strong,omitempty"`
}

func (s Settings) fallbacks(job string) []string { return s.Fallbacks[job] }

// ModelOption is an API model with its price in USD per million tokens,
// which the budget needs before the model may run.
type ModelOption struct {
	ID       string  `json:"id"`
	PriceIn  float64 `json:"price_in"`
	PriceOut float64 `json:"price_out"`
}

var (
	mutable = map[string]bool{"task": true, "failure": true, "backup": true}
	labs    = map[string]bool{"memory_organize": true, "meaning_search": true, "mcp_registry": true}
)

// lab reports whether a newer feature is on.
func (a *App) lab(ctx context.Context, name string) bool {
	return !slices.Contains(a.Settings(ctx).LabsOff, name)
}

func defaultSettings() Settings {
	return Settings{Zone: time.Local.String(), Locale: "pt-BR", JudgeBackend: "local", OllamaModel: "qwen3:1.7b", LocalJudgeURL: "http://127.0.0.1:11500", ExploreModel: "sonnet", CompileModel: "sonnet", JudgeModel: "haiku", GalleryURL: gallery.DefaultIndex, ProtectionNetwork: true, ProtectionURL: protect.DefaultURL}
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
	Protect   *protect.Guard
	Remote    *remote.Remote
	LAN       *remote.LAN
	Google    *oauth.Google
	// LLM and Agent default to Claude Code; tests replace them.
	LLM   llm.Model
	Agent llm.Agent
	// TelegramAPI points at a self-hosted Bot API server; empty means Telegram's.
	TelegramAPI string
	// VoiceAPI replaces a cloud voice provider's address (openai,
	// elevenlabs); tests only.
	VoiceAPI map[string]string
	// WhatsAppAPI replaces the Graph API; tests only.
	WhatsAppAPI string
	// VoiceModel is the whisper.cpp model used for voice notes.
	VoiceModel string
	// GeocodeAPI replaces the place search service; tests only.
	GeocodeAPI string
	// DesktopNotify shows notices as system notifications, for the
	// desktop app.
	DesktopNotify bool
	// Home is the data directory and Version the running version, for
	// backups and connectors.
	Home, Version string
	// MailInsecure uses plain IMAP; tests only.
	MailInsecure bool
	// Router is shared by every run; the demo swaps connectors in it.
	Router *connector.Router
	// localModels downloads and keeps models that run on this computer.
	localModels *local.Manager
	links       map[string]*linkRun
	// Models finds models for the setup screen; nil uses the shared one.
	Models *models.Client
	// TypingEvery renews "typing…" on chat channels; 0 means 4 seconds.
	TypingEvery time.Duration
	health      channelHealth
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
	a.initProtection(ctx)
	a.Policy = a.Rules
	a.LLM = claude{a}
	a.Agent = claude{a}
	set := a.Settings(ctx)
	zone := loadZone(set.Zone)
	a.Budget = &budget.Budget{Events: events, Zone: zone}
	a.Channel = &owner.Channel{Events: events, Bot: a.bot, Handler: handler{a}}
	a.People = &people.Directory{Events: events, OwnerChat: a.Channel.Chat, OwnerWhatsApp: a.ownerWhatsApp}
	a.Channel.People = a.People
	a.Channel.Mirror = func(ctx context.Context, n explore.Notice) {
		a.mirrorWhatsApp(ctx, n)
		a.mirrorLinks(ctx, n)
		go a.mirrorWebhook(context.WithoutCancel(ctx), n)
		if a.DesktopNotify && (n.To == "" || n.To == people.OwnerID) {
			title, body, _ := strings.Cut(n.Text, "\n")
			go desktop.Notify(context.WithoutCancel(ctx), "Pimpo", strings.TrimSpace(title+" "+body))
		}
	}
	a.Channel.Muted = func(ctx context.Context, kind string) bool {
		return mutable[kind] && slices.Contains(a.Settings(ctx).Mute, kind)
	}
	a.Channel.ReadPhoto = ocr.Tesseract{}.Read
	a.Channel.Transcribe = func(ctx context.Context, audio []byte) (string, error) {
		lang := strings.SplitN(a.Settings(ctx).Locale, "-", 2)[0]
		if text, ok, err := a.transcribeLocal(ctx, audio, lang); ok {
			return text, err
		}
		return voice.Whisper{Model: a.VoiceModel, Language: lang}.Transcribe(ctx, audio)
	}
	i18n.Locale = func(ctx context.Context) string { return a.Settings(ctx).Locale }
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
		Approver: approver{a.Approvals}, Remember: a.remember, Write: a.write,
		RoleOf: func(ctx context.Context, person string) string { return string(a.People.Role(ctx, person)) }}
	a.Scheduler = &scheduler.Scheduler{Env: env, Store: st, Notify: a.Channel, Zone: zone}
	a.Explore = &explore.Service{Guide: docs.Guide, Env: env, Store: st, Agent: agentFunc(a.runAgent), Compiler: compiler.Compiler{Model: modelFunc(a.generate), Attempts: 3, Installed: a.installedRoutines,
		Helpers: func(ctx context.Context, id string) (runtime.Helper, error) { return a.Scheduler.Library(ctx, id) }},
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
	a.backupRoutes()
	a.guardRoutes()
	a.channelRoutes()
	a.destinationRoutes()
	a.remoteRoutes()
	a.cloudRoutes()
	a.snapshotRoutes()
	a.repoRoutes()
	a.reminderRoutes()
	a.mediaRoutes()
	a.localRoutes()
	a.speechRoutes()
	a.doctorRoutes()
	a.mcpRoutes()
	a.organizeRoutes()
	a.chatRoutes()
	a.assistantRoutes()
	a.voiceRoutes()
	a.modelRoutes()
	a.systemRoutes()
	return a, nil
}

// Start runs the scheduler and the Telegram listener until ctx ends.
func (a *App) Start(ctx context.Context) error {
	if err := a.Scheduler.Start(ctx); err != nil {
		return err
	}
	go a.Outbox.Run(ctx, 15*time.Second)
	go a.emailChannel(ctx, time.Minute)
	go a.refreshProtection(ctx)
	a.startRemote(ctx)
	go a.cloudLoop(ctx, 15*time.Minute)
	go a.organizeLoop(ctx, 30*time.Minute)
	a.startLinks(ctx)
	go a.healthLoop(ctx, time.Minute)
	a.announceUpgrade(ctx)
	go a.repoLoop(ctx, 15*time.Minute)
	go a.reminderLoop(ctx, 20*time.Second)
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
	a.health.forget("telegram")
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
	for _, k := range s.Mute {
		if !mutable[k] {
			return server.StatusError{Status: 400, Msg: "only task, failure and backup notices can be silenced"}
		}
	}
	for _, k := range s.LabsOff {
		if !labs[k] {
			return server.StatusError{Status: 400, Msg: "unknown feature " + k}
		}
	}
	known := map[string]bool{}
	for _, m := range s.Models {
		provider, name, _ := strings.Cut(m.ID, ":")
		if llm.IsOpencode(m.ID) {
			// opencode reports each call's cost itself.
			if !strings.Contains(name, "/") {
				return server.StatusError{Status: 400, Msg: "an opencode model is opencode:provider/model"}
			}
			known[m.ID] = true
			continue
		}
		if !slices.Contains(llm.Providers, provider) || strings.TrimSpace(name) == "" {
			return server.StatusError{Status: 400, Msg: "a model is provider:name, with provider anthropic, openai, openrouter or ollama"}
		}
		if m.PriceIn < 0 || m.PriceOut < 0 || m.PriceIn > 1000 || m.PriceOut > 1000 {
			return server.StatusError{Status: 400, Msg: "prices are USD per million tokens, from 0 to 1000"}
		}
		known[m.ID] = true
	}
	for _, v := range []voiceChoice{s.routineVoice(), {s.ChatVoice, s.ChatVoiceModel, s.ChatVoiceName}} {
		switch v.Engine {
		case "", "auto", "local", "system", "openai", "elevenlabs":
		case "browser":
			if v != (voiceChoice{s.ChatVoice, s.ChatVoiceModel, s.ChatVoiceName}) {
				return server.StatusError{Status: 400, Msg: "only the chat can read with the browser's voice"}
			}
		default:
			return server.StatusError{Status: 400, Msg: "voice is auto, local, system, openai or elevenlabs"}
		}
		if v.Engine == "openai" && v.Model != "" && speech.OpenAIPrices[v.Model] == 0 {
			return server.StatusError{Status: 400, Msg: "the OpenAI voice model is tts-1 or tts-1-hd"}
		}
	}
	for job, e := range s.Efforts {
		if job != "explore" && job != "compile" && job != "judge" {
			return server.StatusError{Status: 400, Msg: "efforts are for explore, compile or judge"}
		}
		if !llm.ValidEffort(e) {
			return server.StatusError{Status: 400, Msg: "effort is low, medium, high or max"}
		}
	}
	chosen := []string{s.ExploreModel, s.CompileModel, s.JudgeModel, s.AutoLight, s.AutoStrong}
	for job, list := range s.Fallbacks {
		if job != "explore" && job != "compile" && job != "judge" {
			return server.StatusError{Status: 400, Msg: "fallbacks are for explore, compile or judge"}
		}
		if len(list) > 4 {
			return server.StatusError{Status: 400, Msg: "up to four fallbacks per job"}
		}
		chosen = append(chosen, list...)
	}
	for _, chosen := range chosen {
		if provider, _, ok := strings.Cut(chosen, ":"); ok && slices.Contains(llm.Providers, provider) && !known[chosen] {
			return server.StatusError{Status: 400, Msg: chosen + " is not among your models; add it with its price first"}
		}
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
	return telegram.Bot{Token: tok, BaseURL: a.TelegramAPI, Health: func(err error) { a.health.report("telegram", err) }}
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
		notifyCap{a},
		reminderCap{a},
		audioCap{a},
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
		"llm":   judge.LLM{Model: a.LLM, Name: firstModel(host.ModelOf(ctx), set.JudgeModel)},
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
	s := c.a.Settings(ctx)
	job := "compile"
	if r.Model == "" {
		r.Model = s.CompileModel
	} else if r.Model == s.JudgeModel || r.Model == host.ModelOf(ctx) {
		job = "judge"
	}
	if r.Effort == "" {
		if job == "judge" {
			r.Effort = firstModel(host.EffortOf(ctx), s.Efforts["judge"])
		} else {
			r.Effort = s.Efforts["compile"]
		}
	}
	return c.a.withFallback(ctx, job, r.Model, func(model string) (llm.Response, error) {
		r.Model = model
		if api, ok, err := c.a.apiModel(ctx, model); ok {
			if err != nil {
				return llm.Response{}, err
			}
			return api.Generate(ctx, r)
		}
		if llm.IsOpencode(model) {
			return llm.OpencodeCLI{}.Generate(ctx, r)
		}
		if isCodex(model) {
			return llm.CodexCLI{}.Generate(ctx, r)
		}
		return llm.ClaudeCLI{}.Generate(ctx, r)
	})
}

func firstModel(ms ...string) string {
	for _, m := range ms {
		if m != "" {
			return m
		}
	}
	return ""
}

// isCodex is the Codex CLI with the owner's ChatGPT login: "codex", or
// "codex:<model>" for a model the account offers.
func isCodex(model string) bool { return model == "codex" || strings.HasPrefix(model, "codex:") }

func (c claude) Run(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
	s := c.a.Settings(ctx)
	if r.Model == "" {
		r.Model = s.ExploreModel
	}
	if r.Effort == "" {
		r.Effort = s.Efforts["explore"]
	}
	return c.a.withFallback(ctx, "explore", r.Model, func(model string) (llm.Response, error) {
		r.Model = model
		if api, ok, err := c.a.apiModel(ctx, model); ok {
			if err != nil {
				return llm.Response{}, err
			}
			return api.Run(ctx, r)
		}
		if llm.IsOpencode(model) {
			return llm.OpencodeCLI{}.Run(ctx, r)
		}
		if isCodex(model) {
			return llm.CodexCLI{}.Run(ctx, r)
		}
		return llm.ClaudeCLI{}.Run(ctx, r)
	})
}

// withFallback runs a job on its model and, when the provider fails (a
// refused key, no credits, a rate or usage limit, an outage, a missing
// model or CLI), on the next of the job's fallbacks. The owner hears once
// when a job falls back and once when its model answers again. The
// owner's own spending limit and a cancelled task are never retried.
func (a *App) withFallback(ctx context.Context, job, primary string, call func(model string) (llm.Response, error)) (llm.Response, error) {
	chain := append([]string{primary}, a.Settings(ctx).fallbacks(job)...)
	var firstErr error
	for i, model := range chain {
		if i > 0 && slices.Contains(chain[:i], model) {
			continue
		}
		resp, err := call(model)
		if err == nil {
			a.noteFallback(ctx, job, primary, model, firstErr)
			reported := resp.CostUSD
			resp = a.billed(ctx, job, model, resp)
			// Which model answered, and at what cost, for Custo by model and
			// job; a subscription's calls are there at their API equivalent.
			a.Events.Append(ctx, "model.used", "system", map[string]any{"job": job, "model": model, "usd": reported, "fallback": model != primary, "subscription": resp.CostUSD == 0 && reported > 0})
			return resp, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		if !retryable(ctx, err) {
			return a.billed(ctx, job, model, resp), err
		}
	}
	return llm.Response{}, firstErr
}

func retryable(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, llm.ErrCostLimit) || errors.Is(err, budget.ErrOverBudget) {
		return false
	}
	return models.Problem(err) != "other"
}

// noteFallback tells the owner when a job starts or stops using a fallback.
func (a *App) noteFallback(ctx context.Context, job, primary, used string, why error) {
	key := "model.fallback." + job
	was, _ := a.Events.Get(ctx, key)
	switch {
	case used != primary && was != used:
		a.Events.Put(ctx, key, used)
		a.Events.Append(ctx, "model.fallback", "system", map[string]string{"job": job, "model": primary, "fallback": used, "error": errText(why)})
		a.Channel.Notify(ctx, explore.Notice{Kind: "failure", Text: i18n.T(ctx, "msg.model.fallback", "model", primary, "backup", used, "why", i18n.T(ctx, "msg.problem."+models.Problem(why)))})
	case used == primary && was != "":
		a.Events.Put(ctx, key, "")
		a.Events.Append(ctx, "model.recovered", "system", map[string]string{"job": job, "model": primary})
		a.Channel.Notify(ctx, explore.Notice{Kind: "failure", Text: i18n.T(ctx, "msg.model.back", "model", primary)})
	}
}

// apiModel resolves a provider:model setting to an API backend; ok is
// false for Claude Code models (sonnet, opus, haiku).
func (a *App) apiModel(ctx context.Context, model string) (llm.API, bool, error) {
	provider, name, found := strings.Cut(model, ":")
	if !found || !slices.Contains(llm.Providers, provider) {
		return llm.API{}, false, nil
	}
	for _, m := range a.Settings(ctx).Models {
		if m.ID == model {
			return a.apiFor(ctx, provider, name, m.PriceIn, m.PriceOut)
		}
	}
	return llm.API{}, true, fmt.Errorf("%s has no price yet; add it in Settings › Models", model)
}

// apiFor reaches one provider's model with its price, key and address.
func (a *App) apiFor(ctx context.Context, provider, name string, in, out float64) (llm.API, bool, error) {
	api := llm.API{Provider: provider, Model: name, PriceIn: in, PriceOut: out, Base: modelBase[provider]}
	base, err := a.modelEndpoint(ctx, provider)
	if err != nil {
		return api, true, err
	}
	if api.Base == "" {
		api.Base = base.Base
	}
	api.Key = base.Key
	return api, true, nil
}

// modelEndpoint is where a provider answers and the key it needs.
func (a *App) modelEndpoint(ctx context.Context, provider string) (models.Endpoint, error) {
	s := a.Settings(ctx)
	e := models.Endpoint{Provider: provider, Base: llm.Bases[provider]}
	switch provider {
	case "ollama":
		if s.OllamaURL != "" {
			e.Base = strings.TrimRight(s.OllamaURL, "/") + "/v1"
		}
	case "lmstudio":
		if s.LMStudioURL != "" {
			e.Base = strings.TrimRight(s.LMStudioURL, "/") + "/v1"
		}
	case "custom":
		if s.CustomURL == "" {
			return e, errors.New("give the address of your OpenAI-compatible server in Settings › Models")
		}
		e.Base = strings.TrimRight(s.CustomURL, "/")
	}
	if b := modelBase[provider]; b != "" {
		e.Base = b
	}
	p, _ := models.Get(provider)
	key, _ := a.secret(ctx, "model."+provider+".key")
	if p.NeedsKey && key == "" {
		return e, fmt.Errorf("add your %s API key in Settings › Models", p.Name)
	}
	e.Key = key
	return e, nil
}

// modelBase lets tests point providers at fakes.
var modelBase = map[string]string{}

// installedRoutines are what a new routine may build on: the active ones.
func (a *App) installedRoutines(ctx context.Context) []compiler.Installed {
	list, _ := a.Store.Routines(ctx)
	var out []compiler.Installed
	for _, r := range list {
		if r.State != store.RoutineActive {
			continue
		}
		out = append(out, compiler.Installed{ID: r.ID, Name: r.Body.Name, Description: r.Body.Description, Capabilities: r.Body.Manifest.Capabilities, Params: r.Body.Manifest.Params})
	}
	return out
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
	via := owner.ChannelOf(ctx)
	if via == "" {
		if _, err := h.a.Explore.Start(ctx, text, actor(ctx)); err != nil {
			return "", err
		}
		return i18n.T(ctx, "msg.request.started"), nil
	}
	return h.a.converse(ctx, via, text)
}

// convQuiet is how long a chat-channel conversation waits for the next
// message before a new one starts.
const convQuiet = 3 * time.Hour

var newConversation = regexp.MustCompile(`(?i)^\s*/?(new|novo|nova|nuevo|nueva|nouveau|neu|nuovo|reset)(\s+(conversa|conversation|chat))?\s*[.!]?\s*$`)

// converse continues the owner's conversation on a chat channel: messages
// there are turns of one of the app's chats (listed in the app too), so a
// follow-up like "and tomorrow?" knows what came before. /new starts over;
// so do three quiet hours.
func (a *App) converse(ctx context.Context, via, text string) (string, error) {
	person := people.From(ctx)
	key := "conv." + via + "." + person
	if newConversation.MatchString(text) {
		a.Events.Put(ctx, key, "")
		return i18n.T(ctx, "msg.conv.new"), nil
	}
	if m := modelCommand.FindStringSubmatch(text); m != nil {
		return a.modelCommand(ctx, key, strings.TrimSpace(m[1])), nil
	}
	if m := effortCommand.FindStringSubmatch(text); m != nil {
		return a.effortCommand(ctx, key, strings.TrimSpace(m[1])), nil
	}
	var ids []string
	conv := ""
	if raw, _ := a.Events.Get(ctx, key); raw != "" {
		id, at, _ := strings.Cut(raw, "|")
		if sec, err := strconv.ParseInt(at, 10, 64); err == nil && time.Since(time.Unix(sec, 0)) < convQuiet {
			if _, turns, err := a.Store.Chat(ctx, id); err == nil {
				conv, ids = id, turns
			}
		}
	}
	history := a.history(ctx, ids)
	convEffort, _ := a.Events.Get(ctx, key+".effort")
	pick := a.routeModel(ctx, text, history, a.convModel(ctx, key), convEffort)
	o := explore.Options{Context: history, Model: pick.Model, Effort: pick.Effort}
	exp, err := a.Explore.StartWith(ctx, text, actor(ctx), o)
	if err != nil {
		return "", err
	}
	if conv == "" {
		title := channelTitle(via) + " · " + text
		if r := []rune(title); len(r) > 60 {
			title = string(r[:59]) + "…"
		}
		conv = chatID()
		if err := a.Store.CreateChat(ctx, store.Chat{ID: conv, Title: title, Person: chatPerson(person)}); err != nil {
			return "", err
		}
	}
	if show := owner.TypingOf(ctx); show != nil {
		go a.keepTyping(context.WithoutCancel(ctx), exp, show)
	}
	a.noteRouted(ctx, exp, pick)
	a.Store.AddTurn(ctx, conv, exp)
	a.Events.Put(ctx, key, conv+"|"+strconv.FormatInt(time.Now().Unix(), 10))
	if len(ids) > 0 {
		return i18n.T(ctx, "msg.conv.continue"), nil
	}
	return i18n.T(ctx, "msg.request.started"), nil
}

// keepTyping shows "typing…" on the channel while the task runs, renewing
// it before it fades, for at most ten minutes.
func (a *App) keepTyping(ctx context.Context, exploration string, show func(context.Context) error) {
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		if e, err := a.Store.Exploration(ctx, exploration); err != nil || e.State != store.ExplorationRunning {
			return
		}
		tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := show(tctx)
		cancel()
		if err != nil {
			return
		}
		select {
		case <-time.After(a.typingEvery()):
		case <-ctx.Done():
			return
		}
	}
}

// typingEvery is how often "typing…" is renewed; tests make it quick.
func (a *App) typingEvery() time.Duration {
	if a.TypingEvery > 0 {
		return a.TypingEvery
	}
	return 4 * time.Second
}

var modelCommand = regexp.MustCompile(`(?i)^\s*/(?:model|modelo|modele|modèle|modell|modello|模型)(?:\s+(.*))?\s*$`)

// convModel is the model the owner fixed for a channel's conversations.
func (a *App) convModel(ctx context.Context, key string) string {
	m, _ := a.Events.Get(ctx, key+".model")
	return m
}

// modelCommand answers /modelo: with nothing, which model answers and the
// choices; with a name (or "auto"), it fixes that model on this channel.
func (a *App) modelCommand(ctx context.Context, key, arg string) string {
	arg = strings.ToLower(arg)
	s := a.Settings(ctx)
	options := []string{Auto}
	if claudeInstalled() {
		options = append(options, "sonnet", "opus", "haiku")
	}
	if llm.CodexBinary() != "" {
		options = append(options, "codex")
	}
	for _, m := range s.Models {
		options = append(options, m.ID)
	}
	if arg == "" {
		current := a.convModel(ctx, key)
		if current == "" {
			current = Auto
		}
		return i18n.T(ctx, "msg.model.current", "model", current, "options", strings.Join(options, ", "))
	}
	if arg == "automatico" || arg == "automático" || arg == "automatic" {
		arg = Auto
	}
	if !slices.Contains(options, arg) {
		return i18n.T(ctx, "msg.model.unknown", "model", arg, "options", strings.Join(options, ", "))
	}
	if arg == Auto {
		a.Events.Put(ctx, key+".model", "")
	} else {
		a.Events.Put(ctx, key+".model", arg)
	}
	return i18n.T(ctx, "msg.model.set", "model", arg)
}

var effortCommand = regexp.MustCompile(`(?i)^\s*/(?:think|pensar|pensa|effort|esforço|esforco|reasoning|raciocinio|raciocínio|réflexion|reflexion|denken|pensare|思考|생각|думать)(?:\s+(.*))?\s*$`)

// effortWords are the levels as people may type them, in the app's
// languages.
var effortWords = map[string]string{
	"low": "low", "baixo": "low", "bajo": "low", "bas": "low", "niedrig": "low", "basso": "low", "低": "low", "낮음": "low", "низкий": "low",
	"medium": "medium", "médio": "medium", "medio": "medium", "moyen": "medium", "mittel": "medium", "中": "medium", "보통": "medium", "средний": "medium",
	"high": "high", "alto": "high", "élevé": "high", "eleve": "high", "hoch": "high", "高": "high", "높음": "high", "высокий": "high",
	"max": "max", "máximo": "max", "maximo": "max", "maximum": "max", "massimo": "max", "maximal": "max", "最大": "max", "최대": "max", "максимум": "max",
	"auto": Auto, "automático": Auto, "automatico": Auto, "automatic": Auto, "automatique": Auto, "automatisch": Auto,
}

// effortCommand answers /pensar: with nothing, the level in use; with a
// level, that level for this conversation; auto goes back to Pimpo's
// choice.
func (a *App) effortCommand(ctx context.Context, key, arg string) string {
	name := func(level string) string { return i18n.T(ctx, "effort."+level) }
	var names []string
	for _, e := range append([]string{Auto}, llm.Efforts...) {
		names = append(names, name(e))
	}
	options := strings.Join(names, ", ")
	if arg == "" {
		current, _ := a.Events.Get(ctx, key+".effort")
		return i18n.T(ctx, "msg.effort.current", "effort", name(firstModel(current, Auto)), "options", options)
	}
	level, ok := effortWords[strings.ToLower(arg)]
	if !ok {
		return i18n.T(ctx, "msg.effort.unknown", "effort", arg, "options", options)
	}
	if level == Auto {
		a.Events.Put(ctx, key+".effort", "")
	} else {
		a.Events.Put(ctx, key+".effort", level)
	}
	return i18n.T(ctx, "msg.effort.set", "effort", name(level))
}

func channelTitle(via string) string {
	if name, ok := channelNames[via]; ok {
		return name
	}
	return map[string]string{"whatsapp": "WhatsApp"}[via]
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
		return i18n.T(ctx, "msg.routine.created", "name", r.Body.Name, "next", h.a.nextText(ctx, r.ID)), nil
	case "discard":
		return i18n.T(ctx, "msg.discarded"), h.a.Explore.Discard(ctx, id, who)
	case "run":
		h.a.Store.SetRoutineState(ctx, id, store.RoutineActive)
		h.a.Scheduler.Changed(ctx, id)
		if _, err := h.a.Scheduler.RunNow(ctx, id, "owner"); err != nil {
			return "", err
		}
		return i18n.T(ctx, "msg.routine.reran"), nil
	case "repair":
		if _, err := h.a.Explore.Repair(ctx, id, "", "human:owner"); err != nil {
			return "", err
		}
		return i18n.T(ctx, "msg.routine.redo"), nil
	case "approve", "always", "deny", "batch":
		ans := map[string]approval.Answer{"approve": approval.Once, "always": approval.Always, "deny": approval.Deny, "batch": approval.Run}[action]
		if ans == approval.Always && people.From(ctx) != people.OwnerID {
			// Only the owner makes lasting rules.
			ans, action = approval.Once, "approve"
		}
		if !h.a.Approvals.Resolve(ctx, id, ans, who) {
			return "", errors.New(i18n.T(ctx, "msg.approval.gone"))
		}
		return i18n.T(ctx, "msg.approval."+action), nil
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
	return errors.New(i18n.T(ctx, "msg.ownerOnly"))
}

func (a *App) nextText(ctx context.Context, id string) string {
	n := a.Scheduler.Next(id)
	if n.IsZero() {
		return i18n.T(ctx, "msg.routine.notScheduled")
	}
	return n.In(loadZone(a.Settings(ctx).Zone)).Format("02/01 15:04")
}

func budgetCost(usd float64, source string) budget.Cost { return budget.Cost{USD: usd, Source: source} }
