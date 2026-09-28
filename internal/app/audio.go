package app

import (
	"context"
	"crypto/rand"
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

	"github.com/denerFernandes/pimpo/internal/connector"
	"github.com/denerFernandes/pimpo/internal/event"
	"github.com/denerFernandes/pimpo/internal/host"
	"github.com/denerFernandes/pimpo/internal/i18n"
	"github.com/denerFernandes/pimpo/internal/owner"
	"github.com/denerFernandes/pimpo/internal/people"
	"github.com/denerFernandes/pimpo/internal/server"
	"github.com/denerFernandes/pimpo/internal/speech"
	"github.com/denerFernandes/pimpo/internal/telegram"
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
	audio, secs, err := speech.Speak(ctx, spoken(in.Text), in.Language)
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
	return map[string]any{"ok": true, "delivered": delivered, "failed": failed, "seconds": int(secs)}, nil
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
