package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/budget"
	"github.com/turbine-dev/pimpo/internal/connector"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/owner"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/speech"
	"github.com/turbine-dev/pimpo/internal/telegram"
)

// audio.send reads a text aloud on this machine and sends the recording to
// the person the run works for: an audio message with a player on
// Telegram, a note pointing to Pimpo elsewhere, and always a copy in the
// inbox. Like notify.send it only reaches that person.

const mediaKeep = 60

var mediaID = regexp.MustCompile(`^aud-[0-9a-f]{12}$`)

type audioCap struct{ a *App }

func (audioCap) Capabilities() []string { return []string{"audio.send"} }

func (c audioCap) Call(ctx context.Context, _, _ string, args any) (any, error) {
	var in struct {
		Title    string `json:"title"`
		Text     string `json:"text"`
		Language string `json:"language"`
	}
	if err := connector.Args(args, &in); err != nil {
		return nil, err
	}
	in.Title, in.Text = strings.TrimSpace(in.Title), strings.TrimSpace(in.Text)
	if in.Text == "" {
		return nil, errors.New("text is empty")
	}
	if in.Title == "" {
		in.Title = "Pimpo"
	}
	if in.Language == "" {
		in.Language = "pt-BR"
	}
	audio, secs, voice, err := c.a.speak(ctx, spoken(in.Text), in.Language)
	if err != nil {
		return nil, err
	}
	id, err := c.a.saveMedia(audio)
	if err != nil {
		return nil, err
	}
	targets := host.DestinationsFrom(ctx)
	if len(targets) == 0 {
		for _, d := range c.a.destinations(ctx) {
			if d.Ready && (d.ID == "telegram" || d.ID == "whatsapp") {
				targets = []string{d.ID}
				break
			}
		}
	}
	file := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, in.Title) + ".m4a"
	note := i18n.T(ctx, "msg.audio.ready", "title", in.Title, "minutes", fmt.Sprintf("%.0f", max(1, secs/60)))
	var delivered, failed []string
	for _, t := range targets {
		var err error
		switch {
		case t == "telegram":
			err = c.a.sendAudio(ctx, "", file, audio, in.Title, secs)
		case strings.HasPrefix(t, "bot:"):
			err = c.a.sendAudio(ctx, strings.TrimPrefix(t, "bot:"), file, audio, in.Title, secs)
		default:
			// Channels without files get a note; the recording is in Pimpo.
			err = c.a.deliver(ctx, t, note)
		}
		if err != nil {
			failed = append(failed, t+": "+err.Error())
			continue
		}
		delivered = append(delivered, t)
	}
	// The inbox keeps every recording, with a player.
	c.a.Events.Append(ctx, owner.EventNotice, "system", map[string]any{"text": "🎧 " + in.Title, "audio": id, "to": people.Norm(people.From(ctx)), "kind": ""})
	delivered = append(delivered, "inbox")
	return map[string]any{"ok": true, "delivered": delivered, "failed": failed, "seconds": int(secs), "voice": voice}, nil
}

// speak reads text with the voice the owner chose: a downloaded voice, the
// system's, or a cloud provider's, whose cost counts toward the daily
// limit. It says which voice read.
func (a *App) speak(ctx context.Context, text, language string) ([]byte, float64, string, error) {
	return a.speakWith(ctx, a.Settings(ctx).routineVoice(), text, language)
}

// voiceChoice is an engine with its cloud model and voice.
type voiceChoice struct{ Engine, Model, Name string }

func (s Settings) routineVoice() voiceChoice { return voiceChoice{s.Voice, s.VoiceModel, s.VoiceName} }

// chatVoice is the chat's own choice, or the routines' when it has none.
func (s Settings) chatVoice() voiceChoice {
	if s.ChatVoice == "" {
		return s.routineVoice()
	}
	return voiceChoice{s.ChatVoice, s.ChatVoiceModel, s.ChatVoiceName}
}

func (a *App) speakWith(ctx context.Context, v voiceChoice, text, language string) ([]byte, float64, string, error) {
	if r := []rune(text); len(r) > speech.MaxText {
		text = string(r[:speech.MaxText])
	}
	s := Settings{Voice: v.Engine, VoiceModel: v.Model, VoiceName: v.Name}
	switch s.Voice {
	case "openai", "elevenlabs":
		c, err := a.cloudVoice(ctx, s)
		if err != nil {
			return nil, 0, "", err
		}
		cost := c.Cost(text)
		if err := a.Budget.CheckFor(ctx, cost); err != nil {
			return nil, 0, "", err
		}
		b, secs, err := c.Speak(ctx, text)
		if err != nil {
			return nil, 0, "", err
		}
		a.Budget.Record(ctx, budget.Cost{USD: cost, Source: "audio", Ref: "voice:" + s.Voice})
		name := "OpenAI " + firstModel(s.VoiceName, "nova")
		if s.Voice == "elevenlabs" {
			name = "ElevenLabs"
			a.Events.Append(ctx, "voice.used", "system", map[string]any{"provider": "elevenlabs", "chars": len([]rune(text))})
		}
		return b, secs, name, nil
	}
	if a.Home != "" && s.Voice != "system" {
		m := a.local()
		v, ok := m.VoiceFor(language)
		if !ok && s.Voice == "local" {
			return nil, 0, "", fmt.Errorf("no downloaded voice reads %s; get one in Settings › Models › Download models", language)
		}
		if ok {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			dir, err := os.MkdirTemp("", "pimpo-voice-")
			if err != nil {
				return nil, 0, "", err
			}
			defer os.RemoveAll(dir)
			wav := filepath.Join(dir, "speech.wav")
			if err := m.Synthesize(ctx, v, language, text, wav); err != nil {
				return nil, 0, "", err
			}
			b, secs, err := speech.Encode(ctx, wav)
			return b, secs, v.Name, err
		}
	}
	b, secs, err := speech.Speak(ctx, text, language)
	return b, secs, "system", err
}

// cloudVoice is the cloud provider the owner chose, with its key.
func (a *App) cloudVoice(ctx context.Context, s Settings) (speech.Cloud, error) {
	c := speech.Cloud{Provider: s.Voice, Model: s.VoiceModel, Voice: s.VoiceName, Base: a.VoiceAPI[s.Voice]}
	name := "model.openai.key"
	if s.Voice == "elevenlabs" {
		name = "voice.elevenlabs.key"
	}
	key, err := a.Vault.Get(ctx, name)
	if err != nil || key == "" {
		if s.Voice == "openai" {
			return c, errors.New("the OpenAI voice uses your OpenAI key; add it in Settings › Models › Providers")
		}
		return c, errors.New("add your ElevenLabs key in Settings › Models › Voice")
	}
	c.Key = key
	return c, nil
}

var markdown = strings.NewReplacer("**", "", "__", "", "##", "", "#", "", "`", "", "* ", "", "- ", "")
var urls = regexp.MustCompile(`https?://\S+`)

// spoken strips what reads badly aloud: markdown marks and web addresses.
func spoken(text string) string {
	return strings.TrimSpace(urls.ReplaceAllString(markdown.Replace(text), ""))
}

// sendAudio sends to the person's chat with the main bot ("") or to an
// extra bot's chat.
func (a *App) sendAudio(ctx context.Context, botID, file string, audio []byte, title string, secs float64) error {
	if botID == "" {
		chat, err := a.personChat(ctx)
		if err != nil || chat == 0 {
			return errors.New("Telegram is not paired")
		}
		tok, err := a.Vault.Get(ctx, "telegram.token")
		if err != nil || tok == "" {
			return errors.New("Telegram is not set up; open Connections")
		}
		bot := telegram.Bot{Token: tok, BaseURL: a.TelegramAPI}
		_, err = bot.SendAudio(ctx, chat, file, audio, title, "", int(secs))
		return err
	}
	for _, b := range a.bots(ctx) {
		if b.ID == botID {
			if b.Chat == 0 {
				return errors.New("the bot has no chat yet; open Connections")
			}
			c, err := a.botClient(ctx, botID)
			if err != nil {
				return err
			}
			_, err = c.SendAudio(ctx, b.Chat, file, audio, title, "", int(secs))
			return err
		}
	}
	return errors.New("that bot was removed")
}

func (a *App) mediaDir() string { return filepath.Join(a.Home, "media") }

// saveMedia keeps a recording, and only the latest ones.
func (a *App) saveMedia(audio []byte) (string, error) {
	if a.Home == "" {
		return "", errors.New("no data folder for recordings")
	}
	dir := a.mediaDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	b := make([]byte, 6)
	rand.Read(b)
	id := "aud-" + hex.EncodeToString(b)
	if err := os.WriteFile(filepath.Join(dir, id+".m4a"), audio, 0o600); err != nil {
		return "", err
	}
	if files, _ := filepath.Glob(filepath.Join(dir, "aud-*.m4a")); len(files) > mediaKeep {
		sort.Slice(files, func(i, j int) bool { return modTime(files[i]).Before(modTime(files[j])) })
		for _, f := range files[:len(files)-mediaKeep] {
			os.Remove(f)
		}
	}
	return id, nil
}

func modTime(p string) time.Time {
	st, err := os.Stat(p)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}

func (a *App) mediaRoutes() {
	// speak reads a chat answer aloud with the chat's voice and sends the
	// audio back; nothing is kept.
	a.Server.Handle("POST /api/speak", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text     string `json:"text"`
			Language string `json:"language"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		v := a.Settings(r.Context()).chatVoice()
		if v.Engine == "browser" {
			server.WriteError(w, server.StatusError{Status: 409, Msg: "the chat reads with the browser's voice"})
			return
		}
		text, lang := spoken(req.Text), firstModel(req.Language, "pt-BR")
		if strings.TrimSpace(text) == "" {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "nothing to read"})
			return
		}
		key := speechKey(v, lang, text)
		if audio, ok := a.cachedSpeech(key); ok {
			w.Header().Set("Content-Type", "audio/mp4")
			w.Header().Set("X-Pimpo-Cache", "hit")
			w.Write(audio)
			return
		}
		audio, secs, voice, err := a.speakWith(r.Context(), v, text, lang)
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 422, Msg: err.Error()})
			return
		}
		a.cacheSpeech(key, audio)
		w.Header().Set("Content-Type", "audio/mp4")
		w.Header().Set("X-Pimpo-Voice", voice)
		w.Header().Set("X-Pimpo-Seconds", fmt.Sprintf("%.1f", secs))
		w.Write(audio)
	})
	// media lists the latest recordings still kept, newest first.
	a.Server.Handle("GET /api/media", func(w http.ResponseWriter, r *http.Request) {
		evs, _ := a.Events.List(r.Context(), event.Query{Types: []string{owner.EventNotice}, Newest: true, Limit: 300})
		out := []map[string]any{}
		me := people.Norm(people.From(r.Context()))
		for _, e := range evs {
			var n struct {
				Text  string `json:"text"`
				Audio string `json:"audio"`
				To    string `json:"to"`
			}
			if e.Decode(&n) != nil || n.Audio == "" || (n.To != "" && n.To != me) {
				continue
			}
			if _, err := os.Stat(filepath.Join(a.mediaDir(), n.Audio+".m4a")); err != nil {
				continue
			}
			out = append(out, map[string]any{"id": n.Audio, "title": strings.TrimPrefix(n.Text, "🎧 "), "at": e.Time})
			if len(out) == 10 {
				break
			}
		}
		server.WriteJSON(w, 200, out)
	})
	a.Server.Handle("GET /api/media/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !mediaID.MatchString(id) {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such recording"})
			return
		}
		f, err := os.Open(filepath.Join(a.mediaDir(), id+".m4a"))
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "that recording is no longer kept"})
			return
		}
		defer f.Close()
		st, _ := f.Stat()
		w.Header().Set("Content-Type", "audio/mp4")
		http.ServeContent(w, r, id+".m4a", st.ModTime(), f)
	})
}

// Readings for the chat are kept, so listening again, or to a sentence
// already read, starts at once and costs nothing. The key is the voice,
// the language and the text; the oldest go past speechKeep files.
const speechKeep = 400

func speechKey(v voiceChoice, lang, text string) string {
	h := sha256.Sum256([]byte(v.Engine + "\x00" + v.Model + "\x00" + v.Name + "\x00" + lang + "\x00" + text))
	return hex.EncodeToString(h[:16])
}

func (a *App) speechCache() string { return filepath.Join(a.Home, "cache", "speech") }

func (a *App) cachedSpeech(key string) ([]byte, bool) {
	if a.Home == "" {
		return nil, false
	}
	p := filepath.Join(a.speechCache(), key+".m4a")
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	now := time.Now()
	os.Chtimes(p, now, now) // recently heard stays longest
	return b, true
}

func (a *App) cacheSpeech(key string, audio []byte) {
	if a.Home == "" {
		return
	}
	dir := a.speechCache()
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	os.WriteFile(filepath.Join(dir, key+".m4a"), audio, 0o600)
	if files, _ := filepath.Glob(filepath.Join(dir, "*.m4a")); len(files) > speechKeep {
		sort.Slice(files, func(i, j int) bool { return modTime(files[i]).Before(modTime(files[j])) })
		for _, f := range files[:len(files)-speechKeep] {
			os.Remove(f)
		}
	}
}
