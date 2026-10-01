// Package app wires Pimpo together: storage, connectors, the policy, the
// explorer, the scheduler, the Telegram channel and the web API.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/models"
	"github.com/turbine-dev/pimpo/internal/runtime"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/docs"
	"github.com/turbine-dev/pimpo/internal/approval"
	"github.com/turbine-dev/pimpo/internal/browser"
	"github.com/turbine-dev/pimpo/internal/budget"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/compiler"
	"github.com/turbine-dev/pimpo/internal/connector"
	"github.com/turbine-dev/pimpo/internal/connector/calendar"
	"github.com/turbine-dev/pimpo/internal/connector/external"
	"github.com/turbine-dev/pimpo/internal/connector/mail"
	"github.com/turbine-dev/pimpo/internal/connector/services"
	"github.com/turbine-dev/pimpo/internal/connector/sheets"
	"github.com/turbine-dev/pimpo/internal/connector/spotify"
	"github.com/turbine-dev/pimpo/internal/connector/telegramcap"
	"github.com/turbine-dev/pimpo/internal/connector/web"
	"github.com/turbine-dev/pimpo/internal/desktop"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/gallery"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/judge"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/local"
	"github.com/turbine-dev/pimpo/internal/memory"
	"github.com/turbine-dev/pimpo/internal/oauth"
	"github.com/turbine-dev/pimpo/internal/ocr"
	"github.com/turbine-dev/pimpo/internal/outbox"
	"github.com/turbine-dev/pimpo/internal/owner"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/policy"
	"github.com/turbine-dev/pimpo/internal/protect"
	"github.com/turbine-dev/pimpo/internal/push"
	"github.com/turbine-dev/pimpo/internal/remote"
	"github.com/turbine-dev/pimpo/internal/scheduler"
	"github.com/turbine-dev/pimpo/internal/secretscan"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/speech"
	"github.com/turbine-dev/pimpo/internal/store"
	"github.com/turbine-dev/pimpo/internal/telegram"
	"github.com/turbine-dev/pimpo/internal/undo"
	"github.com/turbine-dev/pimpo/internal/vault"
	"github.com/turbine-dev/pimpo/internal/voice"
	"github.com/turbine-dev/pimpo/internal/workspace"
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
	// LabsOn turns on features that stay off until the owner chooses them:
	// code_sandbox, browser, whatsapp_personal, companies.
	LabsOn []string `json:"labs_on,omitempty"`
	// Models are API models the owner set up, with their price; the model
	// settings above may name one as provider:model.
	Models    []ModelOption `json:"models,omitempty"`
	OllamaURL string        `json:"ollama_url,omitempty"`
	// SuggestOff stops the daily routine suggestions.
	SuggestOff bool `json:"suggest_off,omitempty"`
	// LearnOff stops learning preferences from the owner's own requests.
	LearnOff bool `json:"learn_off,omitempty"`
	// LessonDigestOff stops the weekly notice of lessons waiting.
	LessonDigestOff bool `json:"lesson_digest_off,omitempty"`
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
	// CompactOff stops Anthropic's server-side compaction of long
	// conversations; CompactAt is the size in tokens where it starts
	// (0 is 150000; the API's minimum is 50000).
	CompactOff bool `json:"compact_off,omitempty"`
	CompactAt  int  `json:"compact_at,omitempty"`
}

// compactAt is when Anthropic models summarize a long conversation, 0 off.
func (s Settings) compactAt() int {
	switch {
	case s.CompactOff:
		return 0
	case s.CompactAt == 0:
		return 150000
	}
	return s.CompactAt
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
	optIn   = map[string]bool{"code_sandbox": true, "browser": true, "whatsapp_personal": true, companiesLab: true}
)

// lab reports whether a newer feature is on.
func (a *App) lab(ctx context.Context, name string) bool {
	return !slices.Contains(a.Settings(ctx).LabsOff, name)
}

// chose says whether the owner turned on a feature that starts off.
func (a *App) chose(ctx context.Context, name string) bool {
	return slices.Contains(a.Settings(ctx).LabsOn, name)
}

func defaultSettings() Settings {
	return Settings{Zone: time.Local.String(), Locale: "pt-BR", JudgeBackend: "local", OllamaModel: "qwen3:1.7b", LocalJudgeURL: "http://127.0.0.1:11500", ExploreModel: "sonnet", CompileModel: "sonnet", JudgeModel: "haiku", GalleryURL: gallery.DefaultIndex, ProtectionNetwork: true, ProtectionURL: protect.DefaultURL}
}

type App struct {
	bg        sync.WaitGroup
	Events    *event.Store
	Vault     *vault.Vault
	Store     *store.Store
	Companies *company.Store
	work      companyWork
	browsers  map[string]*browser.Browser
	Budget    *budget.Budget
	Channel   *owner.Channel
	Explore   *explore.Service
	Scheduler *scheduler.Scheduler
	Server    *server.Server
	Policy    policy.Policy
	Rules     *policy.Engine
	Approvals *approval.Manager
	Grants    *approval.Grants
	Outbox    *outbox.Outbox
	Undo      *undo.Undo
	Memory    *memory.Memory
	People    *people.Directory
	Protect   *protect.Guard
	Remote    *remote.Remote
	LAN       *remote.LAN
	browser   *browser.Browser
	// startedAt tells parts of jobs a restart interrupted from running ones.
	startedAt  time.Time
	jobRunning sync.Mutex
	jobRuns    map[string]bool
	jobPoll    time.Duration
	progress   progressState
	// ProgressEvery is how often a followed job's message is edited;
	// 0 means every 10 seconds. ProgressSinks replaces the channels it
	// goes to; tests only.
	ProgressEvery time.Duration
	ProgressSinks func(ctx context.Context, person string) []progressSink
	// ListenAddr is where the server listens (--addr); the home network uses its port.
	ListenAddr string
	Google     *oauth.Google
	// LLM and Agent default to Claude Code; tests replace them.
	LLM   llm.Model
	Agent llm.Agent
	// Coder runs company members' coding CLIs, and Workspaces keeps their
	// worktrees; tests replace both.
	Coder      llm.Coder
	Workspaces workspace.Spaces
	// TelegramAPI points at a self-hosted Bot API server; empty means Telegram's.
	TelegramAPI string
	// VoiceAPI replaces a cloud voice provider's address (openai,
	// elevenlabs); tests only.
	VoiceAPI map[string]string
	// SheetsAPI replaces Google Sheets' address; tests only.
	SheetsAPI string
	// GmailAPI and GoogleCerts replace Gmail's and Google's key addresses,
	// and GmailToken each person's Gmail token, for push; tests only.
	GmailAPI    string
	GoogleCerts string
	GmailToken  func(ctx context.Context) (string, error)
	verifier    *push.Verifier
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
	// Spotify controls the owner's Spotify.
	Spotify *spotify.Spotify
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
	// damaged is why the database last failed its check while running.
	damaged string
}

// New assembles an App on top of an open event store.
func New(ctx context.Context, events *event.Store, v *vault.Vault, token, baseURL string) (*App, error) {
	st, err := store.Open(events.DB())
	if err != nil {
		return nil, err
	}
	companies, err := company.Open(events.DB())
	if err != nil {
		return nil, err
	}
	a := &App{Events: events, Vault: v, Store: st, Companies: companies, startedAt: time.Now().UTC(), jobPoll: 2 * time.Second}
	a.Rules = &policy.Engine{Events: events}
	a.initProtection(ctx)
	a.Policy = a.Rules
	a.LLM = claude{a}
	a.Agent = claude{a}
	a.Coder = llm.CodeCLI{}
	set := a.Settings(ctx)
	zone := loadZone(set.Zone)
	a.Budget = &budget.Budget{Events: events, Zone: zone}
	a.Channel = &owner.Channel{Events: events, Bot: a.bot, Handler: handler{a}}
	a.People = &people.Directory{Events: events, OwnerChat: a.Channel.Chat, OwnerWhatsApp: a.ownerWhatsApp}
	a.Budget.PersonLimit = a.personLimit
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
	i18n.SetLocale(func(ctx context.Context) string { return a.Settings(ctx).Locale })
	a.Approvals = &approval.Manager{Events: events, Notify: a.Channel, Describe: describeAction, Responsible: func(ctx context.Context, person string) (string, string) {
		asker, _ := a.People.Get(ctx, person)
		return a.People.Responsible(ctx, person).ID, asker.Name
	}}
	a.Grants = &approval.Grants{Events: events}
	router := a.router()
	a.Router = router
	a.Outbox = &outbox.Outbox{DB: events.DB(), Events: events, Send: func(ctx context.Context, args any) (any, error) { return router.Call(ctx, "gmail.send", "", args) }}
	if err := a.Outbox.Init(); err != nil {
		return nil, err
	}
	router.Add(a.Outbox)
	a.Undo = &undo.Undo{Events: events, Outbox: a.Outbox, Mail: a.mailOps, Call: func(ctx context.Context, name string, args any) (any, error) {
		return a.Router.Call(ctx, name, "", args)
	}}
	env := host.Env{Router: router, Judge: judgeFunc(a.judge), Budget: a.Budget, Events: events, Policy: policyFunc(a.decide),
		Approver: approver{a.Approvals, a.deliveryAnswered}, Remember: a.remember, Write: a.write,
		RoleOf:  func(ctx context.Context, person string) string { return string(a.People.Role(ctx, person)) },
		Missing: a.missingCredential}
	a.Scheduler = &scheduler.Scheduler{Env: env, Store: st, Notify: a.Channel, Zone: zone, Progress: a.runProgress, Pushed: a.pushLive, Hold: a.holdRoutine}
	a.Explore = &explore.Service{Guide: docs.Guide, Skills: a.exploreSkills, Env: env, Store: st, Agent: agentFunc(a.runAgent), Compiler: compiler.Compiler{Model: modelFunc(a.generate), Attempts: 3, Installed: a.installedRoutines,
		Helpers: func(ctx context.Context, id string) (runtime.Helper, error) { return a.Scheduler.Library(ctx, id) }},
		Notify: a.Channel, Routines: a.Scheduler, BaseURL: baseURL, Zone: zone}
	a.Server = server.New(events, token)
	a.Server.Allow, a.Server.Visible = a.allow, a.eventVisible
	a.googleRoutes()
	a.routes()
	a.safetyRoutes()
	a.grantRoutes()
	a.memoryRoutes()
	a.migrateRoutes()
	a.pairingRoutes()
	a.miniAppRoutes()
	a.peopleRoutes()
	a.limitRoutes()
	a.whatsappRoutes()
	a.galleryRoutes()
	a.catalogRoutes()
	a.historyRoutes()
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
	a.webhookRoutes()
	a.pushRoutes()
	a.questionRoutes()
	a.spotifyRoutes()
	a.doctorRoutes()
	a.mcpRoutes()
	a.openapiRoutes()
	a.quickSetupRoutes()
	a.suggestionRoutes()
	a.skillRoutes()
	a.browserRoutes()
	a.learnRoutes()
	a.lessonRoutes()
	a.phoneRoutes()
	a.jobRoutes()
	a.companyRoutes()
	a.companyWorkRoutes()
	a.companyTeamRoutes()
	a.companyMemoryRoutes()
	a.companyDecideRoutes()
	a.companyLevelRoutes()
	a.companyCostRoutes()
	a.companyAccountRoutes()
	a.companyMeetRoutes()
	a.companyCodeRoutes()
	a.companyProductRoutes()
	a.companyEarnRoutes()
	a.progressRoutes()
	a.needRoutes()
	a.passkeyRoutes()
	a.accountRoutes()
	a.widgetRoutes()
	a.widgetFeedRoutes()
	a.organizeRoutes()
	a.chatRoutes()
	a.credentialRoutes()
	a.assistantRoutes()
	a.voiceRoutes()
	a.modelRoutes()
	a.systemRoutes()
	a.passwordManagerRoutes()
	return a, nil
}

// background runs one of Start's loops, counted so Wait can tell when
// they have all returned.
func (a *App) background(f func()) {
	a.bg.Add(1)
	go func() {
		defer a.bg.Done()
		f()
	}()
}

// Wait blocks until the loops Start began have returned after its context
// ended, or until d passes.
func (a *App) Wait(d time.Duration) {
	done := make(chan struct{})
	go func() { a.bg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
	}
}

// Start runs the scheduler and the Telegram listener until ctx ends.
func (a *App) Start(ctx context.Context) error {
	if err := a.Scheduler.Start(ctx); err != nil {
		return err
	}
	a.background(func() { a.Outbox.Run(ctx, 15*time.Second) })
	a.background(func() { a.emailChannel(ctx, time.Minute) })
	a.background(func() { a.refreshProtection(ctx) })
	a.startRemote(ctx)
	a.background(func() { a.cloudLoop(ctx, 15*time.Minute) })
	a.background(func() { a.organizeLoop(ctx, 30*time.Minute) })
	a.startLinks(ctx)
	a.background(func() { a.healthLoop(ctx, time.Minute) })
	a.announceUpgrade(ctx)
	a.background(func() { a.repoLoop(ctx, 15*time.Minute) })
	a.background(func() { a.reminderLoop(ctx, 20*time.Second) })
	a.background(func() { a.aliveLoop(ctx, time.Minute) })
	a.background(func() { a.suggestLoop(ctx, 30*time.Minute) })
	a.background(func() { a.learnLoop(ctx, time.Hour) })
	a.settleProgress(ctx)
	a.background(func() { a.pushLoop(ctx, time.Hour) })
	a.background(func() { a.lessonDigestLoop(ctx, time.Hour) })
	a.background(func() { a.catalogLoop(ctx, time.Hour) })
	a.resumeJobs(ctx)
	a.resumeWork(ctx)
	a.background(func() { a.workLoop(ctx, workPumpEvery) })
	a.background(func() { a.digestLoop(ctx, 30*time.Minute) })
	go func() {
		<-ctx.Done()
		a.mu.Lock()
		b := a.browser
		members := a.browsers
		a.mu.Unlock()
		if b != nil {
			b.Close()
		}
		for _, mb := range members {
			mb.Close()
		}
	}()
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
	go a.syncMiniApp(lctx)
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

// SaveSettings checks and keeps the settings, and records in the history
// what changed, the models apart from the rest.
func (a *App) SaveSettings(ctx context.Context, s Settings, actor string) error {
	general := a.track(ctx, histEntry{Area: "settings", Store: histSettings, Person: people.OwnerID})
	models := a.track(ctx, histEntry{Area: "models", Store: histSettings, Person: people.OwnerID})
	if err := a.saveSettings(ctx, s, actor); err != nil {
		return err
	}
	general()
	models()
	return nil
}

func (a *App) saveSettings(ctx context.Context, s Settings, actor string) error {
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
	for _, k := range s.LabsOn {
		if !optIn[k] {
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
			return server.StatusError{Status: 400, Msg: "a model is provider:name, with provider one of " + strings.Join(llm.Providers, ", ") + " (or opencode:provider/model)"}
		}
		if m.PriceIn < 0 || m.PriceOut < 0 || m.PriceIn > 1000 || m.PriceOut > 1000 {
			return server.StatusError{Status: 400, Msg: "prices are USD per million tokens, from 0 to 1000"}
		}
		known[m.ID] = true
	}
	if s.CompactAt != 0 && (s.CompactAt < 50000 || s.CompactAt > 1000000) {
		return server.StatusError{Status: 400, Msg: "compaction starts between 50000 and 1000000 tokens"}
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
		codeCap{a},
		browserCap{a},
		phoneCap{a},
		slackCap{a},
		widgetCap{a},
		audioCap{a},
		askCap{a},
		wakeCap{a},
		codeWorkspace{a},
		productCap{a},
		teamCap{a},
		memoryCap{a},
		a.spotify(),
		&sheets.Sheets{Token: func(ctx context.Context) (string, error) { return a.Google.Token(ctx) }, Granted: func(ctx context.Context) bool { return a.Google.Granted(ctx, oauth.SheetsScope) }, API: a.SheetsAPI},
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

// personal names a setting or secret of whoever ctx acts for. A company
// member has its own, never its person's; the owner's keep the names they
// had before people existed.
func personal(ctx context.Context, name string) string {
	if m := host.MemberOf(ctx); m != "" {
		return memberKey(m, name)
	}
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
	if src, _ := z.a.Events.Get(ctx, "calendar.source"); src == "google" && z.a.Google != nil && people.From(ctx) == people.OwnerID && host.MemberOf(ctx) == "" {
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
	acct := mail.Account{Addr: addr, Username: user, SMTP: smtp, Insecure: m.a.MailInsecure, Password: func(ctx context.Context) (string, error) {
		v, err := m.a.Vault.Get(ctx, pw)
		if errors.Is(err, vault.ErrNotFound) {
			return "", &connector.MissingCredential{Connector: "mail", Field: "password", Err: errors.New("email is not set up; open Connections")}
		}
		return v, err
	}}
	if auth, _ := m.a.Events.Get(ctx, personal(ctx, "mail.auth")); auth == "oauth" && m.a.Google != nil && people.From(ctx) == people.OwnerID && host.MemberOf(ctx) == "" {
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
		"local": judge.Chain{judge.Local{URL: set.LocalJudgeURL}, judge.Ollama{BaseURL: a.ollamaBase(ctx), Model: set.OllamaModel}},
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
	d := a.granted(ctx, act, a.Policy.Decide(ctx, act))
	if act.Member != "" {
		d = a.companyDecides(ctx, act, d)
	}
	return d
}

func (a *App) generate(ctx context.Context, r llm.Request) (llm.Response, error) {
	if r.Model == "" {
		r.Model = a.Settings(ctx).CompileModel
	}
	var ok bool
	if r.Model, ok = a.fitModel(ctx, r.Model); !ok {
		return llm.Response{}, a.errNoModel(ctx)
	}
	return a.LLM.Generate(ctx, r)
}

func (a *App) runAgent(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
	if r.Model == "" {
		r.Model = a.Settings(ctx).ExploreModel
	}
	var ok bool
	if r.Model, ok = a.fitModel(ctx, r.Model); !ok {
		return llm.Response{}, a.errNoModel(ctx)
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
		if err := claudeCodeHere(); err != nil {
			return llm.Response{}, err
		}
		return llm.ClaudeCLI{}.Generate(ctx, r)
	})
}

// claudeCodeHere explains, when a job falls to Claude Code on a computer
// without it, how to choose another model instead of naming a missing file.
func claudeCodeHere() error {
	if _, err := exec.LookPath("claude"); err != nil {
		return errors.New("no model is set up yet: choose one in Ajustes › Modelos (an API key, Ollama, Codex, opencode or Claude Code). Claude Code is not installed")
	}
	return nil
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
		if err := claudeCodeHere(); err != nil {
			return llm.Response{}, err
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
	// Only what the person and the assistant may use is tried.
	chain := a.allowedChain(ctx, append([]string{primary}, a.Settings(ctx).fallbacks(job)...))
	if len(chain) == 0 {
		return llm.Response{}, a.errNoModel(ctx)
	}
	primary = chain[0]
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
	if provider == "anthropic" {
		api.CompactAt = a.Settings(ctx).compactAt()
	}
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
	// A pasted key goes no further than here: not to the model, the
	// conversation or the log. The person is told, and nothing is kept.
	text, warning := h.a.guardPasted(ctx, text)
	if warning == "" {
		return h.request(ctx, text)
	}
	if secretscan.Only(text) {
		return warning, nil
	}
	reply, err := h.request(ctx, text)
	return warning + "\n\n" + reply, err
}

func (h handler) request(ctx context.Context, text string) (string, error) {
	via := owner.ChannelOf(ctx)
	if via == "" {
		if _, err := h.a.Explore.Start(ctx, text, actor(ctx)); err != nil {
			return "", err
		}
		return i18n.T(ctx, "msg.request.started"), nil
	}
	if out, ok := h.a.bareAnswer(ctx, via, text); ok {
		return out, nil
	}
	return h.a.converse(ctx, via, text)
}

// Reply takes a typed reply to a notice with choices on a chat channel:
// a question's options are matched; other notices are not answered in
// words.
func (h handler) Reply(ctx context.Context, choices []explore.Action, text string) (string, bool) {
	return h.a.replyAnswer(ctx, choices, text)
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
	var chat store.Chat
	if raw, _ := a.Events.Get(ctx, key); raw != "" {
		id, at, _ := strings.Cut(raw, "|")
		if sec, err := strconv.ParseInt(at, 10, 64); err == nil && time.Since(time.Unix(sec, 0)) < convQuiet {
			if c, turns, err := a.Store.Chat(ctx, id); err == nil {
				chat, ids = c, turns
			}
		}
	}
	fresh := chat.ID == ""
	if fresh {
		title := channelTitle(via) + " · " + text
		if r := []rune(title); len(r) > 60 {
			title = string(r[:59]) + "…"
		}
		chat = store.Chat{ID: chatID(), Title: title, Person: chatPerson(person)}
	}
	history := a.history(ctx, ids)
	convEffort, _ := a.Events.Get(ctx, key+".effort")
	pick := a.routeModel(ctx, text, history, a.convModel(ctx, key), convEffort)
	o := explore.Options{Context: history, Model: pick.Model, Effort: pick.Effort, Origin: chatOrigin(chat)}
	exp, err := a.Explore.StartWith(ctx, text, actor(ctx), o)
	if err != nil {
		return "", err
	}
	conv := chat.ID
	if fresh {
		if err := a.Store.CreateChat(ctx, chat); err != nil {
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
	options := []string{Auto}
	for _, m := range a.houseModels(ctx) {
		if a.modelAllowed(ctx, m) {
			options = append(options, m)
		}
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
		h.a.unstop(ctx, id)
		if _, err := h.a.Scheduler.RunNow(ctx, id, "owner"); err != nil {
			return "", err
		}
		return i18n.T(ctx, "msg.routine.reran"), nil
	case "repair":
		if _, err := h.a.Explore.Repair(ctx, id, "", "human:owner"); err != nil {
			return "", err
		}
		return i18n.T(ctx, "msg.routine.redo"), nil
	case "suggest":
		if _, err := h.a.acceptSuggestion(ctx, id); err != nil {
			return "", err
		}
		return i18n.T(ctx, "msg.suggestion.started"), nil
	case "nosuggest":
		if err := h.a.dismissSuggestion(ctx, id); err != nil {
			return "", err
		}
		return i18n.T(ctx, "msg.suggestion.declined"), nil
	case "answer":
		qid, i, err := parseAnswer(id)
		if err != nil {
			return "", err
		}
		return h.a.answer(ctx, qid, i)
	case "coq":
		return h.a.answerCompanyButton(ctx, id)
	case "approve", "always", "deny", "batch", "grant":
		ans := map[string]approval.Answer{"approve": approval.Once, "always": approval.Always, "deny": approval.Deny, "batch": approval.Run, "grant": approval.Routine}[action]
		if ans == approval.Always && people.From(ctx) != people.OwnerID {
			// Only the owner makes lasting rules.
			ans, action = approval.Once, "approve"
		}
		if err := h.a.resolveApproval(ctx, id, ans, nil); err != nil {
			if errors.Is(err, errApprovalGone) {
				return "", errors.New(i18n.T(ctx, "msg.approval.gone"))
			}
			return "", errors.New(i18n.T(ctx, "msg.approval.noGrant"))
		}
		return i18n.T(ctx, "msg.approval."+action), nil
	}
	return "", fmt.Errorf("unknown action %q", action)
}

// allowed keeps everyone, the owner included, to their own explorations
// and to the approvals they answer for; routines and everything else stay
// with the owner.
func (h handler) allowed(ctx context.Context, action, id string) error {
	person := people.From(ctx)
	switch action {
	case "approve", "always", "deny", "batch", "grant":
		if h.a.Approvals.MayAnswer(id, person) {
			return nil
		}
		return errors.New(i18n.T(ctx, "msg.approval.gone"))
	case "compile", "discard":
		if e, err := h.a.Store.Exploration(ctx, id); err == nil && mine(ctx, e.Person) {
			return nil
		}
		if person == people.OwnerID {
			return store.ErrNotFound
		}
	case "run", "repair":
		if _, err := h.a.myRoutine(ctx, id); err != nil && person == people.OwnerID {
			return err
		}
	}
	if person == people.OwnerID {
		return nil
	}
	switch action {
	case "answer", "coq":
		// Both check that the question is theirs to answer.
		return nil
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
