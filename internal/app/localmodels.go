package app

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/local"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/speech"
	"github.com/turbine-dev/pimpo/internal/voice"
)

// Models to download and run on this computer: voices for reading aloud,
// and language models through Ollama. Settings › Models shows each
// download as it happens.

var localMu sync.Mutex

func (a *App) local() *local.Manager {
	localMu.Lock()
	defer localMu.Unlock()
	dir := filepath.Join(a.Home, "local")
	if a.localModels == nil || a.localModels.Dir != dir {
		// The owner may keep a catalog of their own next to the models.
		if err := local.UseFile(filepath.Join(dir, "catalog.json")); err != nil {
			a.Events.Append(context.Background(), "local.catalog.invalid", "system", map[string]string{"error": err.Error()})
		}
		a.localModels = &local.Manager{Dir: dir, OllamaURL: func() string { return a.ollamaBase(context.Background()) }}
	}
	return a.localModels
}

func (a *App) ollamaBase(ctx context.Context) string {
	if u := a.Settings(ctx).OllamaURL; u != "" {
		return strings.TrimRight(u, "/")
	}
	return strings.TrimSuffix(llm.Bases["ollama"], "/v1")
}

func memoryBytes() int64 {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err == nil {
			n, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
			return n
		}
	}
	return 0
}

type localItem struct {
	local.Item
	Installed bool `json:"installed"`
}

func (a *App) localRoutes() {
	a.Server.Handle("GET /api/local", func(w http.ResponseWriter, r *http.Request) {
		m := a.local()
		out := map[string]any{"jobs": m.Jobs(), "free": local.FreeBytes(m.Dir), "memory": memoryBytes(), "suggestions": local.Suggestions}
		if e, ok := local.Engine(); ok {
			out["engine"] = localItem{e, m.Installed(e)}
		}
		voices := []localItem{}
		for _, v := range local.Voices {
			voices = append(voices, localItem{v, m.Installed(v)})
		}
		out["voices"] = voices
		trs := []localItem{}
		for _, t := range local.Transcribers {
			trs = append(trs, localItem{t, m.Installed(t)})
		}
		out["transcribers"] = trs
		ollama := map[string]any{"url": a.ollamaBase(r.Context())}
		if found := a.modelClient().Detect(r.Context(), a.Settings(r.Context()).OllamaURL, "x"); found.OllamaUp {
			ollama["up"] = true
			ollama["models"] = found.Ollama
		}
		out["ollama"] = ollama
		server.WriteJSON(w, 200, out)
	})
	a.Server.Handle("POST /api/local/install/{id}", func(w http.ResponseWriter, r *http.Request) {
		job, err := a.local().Install(r.PathValue("id"))
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
			return
		}
		a.Events.Append(r.Context(), "local.download", actor(r.Context()), map[string]string{"item": job.Item})
		server.WriteJSON(w, 202, job)
	})
	a.Server.Handle("DELETE /api/local/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.local().Remove(r.PathValue("id")); err != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
			return
		}
		a.Events.Append(r.Context(), "local.removed", actor(r.Context()), map[string]string{"item": r.PathValue("id")})
		server.WriteJSON(w, 200, map[string]bool{"ok": true})
	})
	a.Server.Handle("POST /api/local/jobs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		server.WriteJSON(w, 200, map[string]bool{"ok": a.local().Cancel(r.PathValue("id"))})
	})
	a.Server.Handle("POST /api/local/ollama/pull", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		job, err := a.local().Pull(req.Model)
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
			return
		}
		a.Events.Append(r.Context(), "local.download", actor(r.Context()), map[string]string{"item": job.Item})
		server.WriteJSON(w, 202, job)
	})
	a.Server.Handle("DELETE /api/local/ollama/{model...}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.local().DeleteOllama(r.Context(), r.PathValue("model")); err != nil {
			server.WriteError(w, server.StatusError{Status: 502, Msg: err.Error()})
			return
		}
		server.WriteJSON(w, 200, map[string]bool{"ok": true})
	})
	// sample reads a sentence with a voice, to hear it before choosing.
	a.Server.Handle("POST /api/local/sample", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Language string `json:"language"`
			Text     string `json:"text"`
			// For is chat or routines: whose voice to hear.
			For string `json:"for"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		if req.Text == "" || len(req.Text) > 400 {
			req.Text = sampleText(req.Language)
		}
		v := a.Settings(r.Context()).routineVoice()
		if req.For == "chat" {
			v = a.Settings(r.Context()).chatVoice()
		}
		if v.Engine == "browser" {
			server.WriteError(w, server.StatusError{Status: 409, Msg: "the chat reads with the browser's voice"})
			return
		}
		audio, secs, voice, err := a.speakWith(r.Context(), v, req.Text, req.Language)
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
			return
		}
		id, err := a.saveMedia(audio)
		if err != nil {
			server.WriteError(w, err)
			return
		}
		server.WriteJSON(w, 200, map[string]any{"id": id, "seconds": secs, "voice": voice})
	})
}

func sampleText(lang string) string {
	switch {
	case strings.HasPrefix(lang, "en"):
		return "Good morning! Here are today's top stories on Hacker News."
	case strings.HasPrefix(lang, "es"):
		return "¡Buenos días! Estas son las historias más votadas hoy en Hacker News."
	case strings.HasPrefix(lang, "fr"):
		return "Bonjour ! Voici les histoires les plus populaires aujourd'hui sur Hacker News."
	case strings.HasPrefix(lang, "de"):
		return "Guten Morgen! Hier sind die wichtigsten Geschichten von heute auf Hacker News."
	case strings.HasPrefix(lang, "it"):
		return "Buongiorno! Ecco le storie più votate di oggi su Hacker News."
	}
	return "Bom dia! Estas são as histórias com mais pontos hoje no Hacker News."
}

func (a *App) speechRoutes() {
	// voice says what can read aloud here, for Settings.
	a.Server.Handle("GET /api/voice", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		_, oerr := a.Vault.Get(ctx, "model.openai.key")
		_, eerr := a.Vault.Get(ctx, "voice.elevenlabs.key")
		out := map[string]any{"openai_key": oerr == nil, "elevenlabs_key": eerr == nil, "openai_voices": speech.OpenAIVoices, "openai_prices": speech.OpenAIPrices}
		if eerr == nil && a.Settings(ctx).Voice == "elevenlabs" {
			if c, err := a.cloudVoice(ctx, Settings{Voice: "elevenlabs"}); err == nil {
				if vs, err := c.ElevenVoices(ctx); err == nil {
					out["elevenlabs_voices"] = vs
				} else {
					out["elevenlabs_error"] = err.Error()
				}
			}
		}
		server.WriteJSON(w, 200, out)
	})
	a.Server.Handle("PUT /api/voice/elevenlabs-key", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Key string `json:"key"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		var err error
		done := a.track(r.Context(), kvEntry("models", "elevenlabs", people.OwnerID, nil, "voice.elevenlabs.key"))
		if key := strings.TrimSpace(req.Key); key == "" {
			err = a.Vault.Delete(r.Context(), "voice.elevenlabs.key")
		} else {
			if _, verr := (speech.Cloud{Provider: "elevenlabs", Key: key, Base: a.VoiceAPI["elevenlabs"]}).ElevenVoices(r.Context()); verr != nil {
				server.WriteError(w, server.StatusError{Status: 400, Msg: verr.Error()})
				return
			}
			err = a.Vault.Set(r.Context(), "voice.elevenlabs.key", key)
		}
		if err != nil {
			server.WriteError(w, err)
			return
		}
		done()
		a.Events.Append(r.Context(), "voice.key", actor(r.Context()), map[string]string{"provider": "elevenlabs"})
		server.WriteJSON(w, 200, map[string]bool{"ok": true})
	})
}

// transcribeLocal turns speech into text with a downloaded Whisper model;
// ok is false when none is downloaded.
func (a *App) transcribeLocal(ctx context.Context, audio []byte, lang string) (string, bool, error) {
	if a.Home == "" {
		return "", false, nil
	}
	m := a.local()
	t, ok := m.Transcriber()
	if !ok {
		return "", false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	dir, err := os.MkdirTemp("", "pimpo-listen-")
	if err != nil {
		return "", true, err
	}
	defer os.RemoveAll(dir)
	in, wav := filepath.Join(dir, "in.audio"), filepath.Join(dir, "in.wav")
	if err := os.WriteFile(in, audio, 0o600); err != nil {
		return "", true, err
	}
	if err := voice.ToWAV16(ctx, in, wav); err != nil {
		return "", true, err
	}
	text, err := m.Transcribe(ctx, t, wav, lang)
	return text, true, err
}
