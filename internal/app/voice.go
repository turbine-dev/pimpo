package app

import (
	"errors"
	"io"
	"net/http"

	"github.com/denerFernandes/zodim/internal/server"
	"github.com/denerFernandes/zodim/internal/voice"
)

// Dictation for browsers without their own speech recognition: the audio
// is transcribed on this machine, like voice notes on Telegram.
func (a *App) voiceRoutes() {
	a.Server.Handle("POST /api/voice/transcribe", func(w http.ResponseWriter, r *http.Request) {
		audio, err := io.ReadAll(io.LimitReader(r.Body, 20<<20))
		if err != nil || len(audio) == 0 {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "send the recorded audio"})
			return
		}
		text, err := a.Channel.Transcribe(r.Context(), audio)
		switch {
		case errors.Is(err, voice.ErrNotInstalled):
			server.WriteError(w, server.StatusError{Status: 501, Msg: err.Error()})
		case err != nil:
			server.WriteError(w, server.StatusError{Status: 422, Msg: err.Error()})
		default:
			server.WriteJSON(w, 200, map[string]string{"text": text})
		}
	})
}
