package speech

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// Cloud voices: the text goes to a provider, which reads it back as audio.
// Both are asked for raw 24 kHz PCM, a piece at a time, so long texts
// join into one file.

// OpenAIPrices are USD per million characters read, for the models priced
// by character; the cost of a reading is exact.
var OpenAIPrices = map[string]float64{"tts-1": 15, "tts-1-hd": 30}

// OpenAIVoices are the voices OpenAI offers for tts-1 and tts-1-hd; they
// read any language.
var OpenAIVoices = []string{"alloy", "ash", "coral", "echo", "fable", "nova", "onyx", "sage", "shimmer"}

// Cloud reads text through a provider.
type Cloud struct {
	// Provider is openai or elevenlabs.
	Provider string
	Key      string
	// Model is tts-1 or tts-1-hd for OpenAI, a model id for ElevenLabs.
	Model string
	// Voice is an OpenAI voice name or an ElevenLabs voice id.
	Voice string
	// Base replaces the provider's address; tests only.
	Base string
	HTTP *http.Client
}

// Cost is what reading text costs, in USD; ElevenLabs spends the plan's
// credits instead, so 0.
func (c Cloud) Cost(text string) float64 {
	if c.Provider != "openai" {
		return 0
	}
	return float64(len([]rune(text))) * OpenAIPrices[c.model()] / 1e6
}

func (c Cloud) model() string {
	if c.Model != "" {
		return c.Model
	}
	if c.Provider == "openai" {
		return "tts-1"
	}
	return "eleven_multilingual_v2"
}

// Speak reads text and returns an M4A file and its length in seconds.
func (c Cloud) Speak(ctx context.Context, text string) ([]byte, float64, error) {
	if c.Key == "" {
		return nil, 0, fmt.Errorf("%s needs its API key", c.Provider)
	}
	limit := 4000
	if c.Provider == "elevenlabs" {
		limit = 4800
	}
	var pcm bytes.Buffer
	for _, piece := range Chunks(text, limit) {
		b, err := c.read(ctx, piece)
		if err != nil {
			return nil, 0, err
		}
		pcm.Write(b)
	}
	dir, err := os.MkdirTemp("", "pimpo-cloud-voice-")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(dir)
	wav := filepath.Join(dir, "speech.wav")
	if err := os.WriteFile(wav, WAV(pcm.Bytes(), 24000), 0o600); err != nil {
		return nil, 0, err
	}
	return Encode(ctx, wav)
}

func (c Cloud) read(ctx context.Context, text string) ([]byte, error) {
	var req *http.Request
	var err error
	switch c.Provider {
	case "openai":
		base := firstOf(c.Base, "https://api.openai.com/v1")
		voice := firstOf(c.Voice, "nova")
		body, _ := json.Marshal(map[string]string{"model": c.model(), "voice": voice, "input": text, "response_format": "pcm"})
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, base+"/audio/speech", bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+c.Key)
		}
	case "elevenlabs":
		base := firstOf(c.Base, "https://api.elevenlabs.io/v1")
		if c.Voice == "" {
			return nil, errors.New("choose an ElevenLabs voice first")
		}
		body, _ := json.Marshal(map[string]string{"text": text, "model_id": c.model()})
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, base+"/text-to-speech/"+url.PathEscape(c.Voice)+"?output_format=pcm_24000", bytes.NewReader(body))
		if err == nil {
			req.Header.Set("xi-api-key", c.Key)
		}
	default:
		return nil, fmt.Errorf("unknown voice provider %q", c.Provider)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s is unreachable: %w", c.Provider, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 200<<20))
	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return nil, fmt.Errorf("%s refused the API key", c.Provider)
	case resp.StatusCode == 429:
		return nil, fmt.Errorf("%s: usage or rate limit reached", c.Provider)
	case resp.StatusCode/100 != 2:
		msg := strings.TrimSpace(string(b))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, fmt.Errorf("%s answered %d: %s", c.Provider, resp.StatusCode, msg)
	}
	return b, nil
}

// Chunks splits text into pieces of at most limit characters, at sentence
// ends when it can, then at spaces.
func Chunks(text string, limit int) []string {
	var out []string
	rest := []rune(strings.TrimSpace(text))
	for len(rest) > limit {
		cut := -1
		for i := limit - 1; i > limit/2; i-- {
			if (rest[i] == '.' || rest[i] == '!' || rest[i] == '?' || rest[i] == '\n') && (i+1 == len(rest) || unicode.IsSpace(rest[i+1])) {
				cut = i + 1
				break
			}
		}
		if cut < 0 {
			for i := limit - 1; i > 0; i-- {
				if unicode.IsSpace(rest[i]) {
					cut = i
					break
				}
			}
		}
		if cut <= 0 {
			cut = limit
		}
		out = append(out, strings.TrimSpace(string(rest[:cut])))
		rest = []rune(strings.TrimSpace(string(rest[cut:])))
	}
	if len(rest) > 0 {
		out = append(out, string(rest))
	}
	return out
}

// WAV wraps 16-bit mono PCM in a WAV header.
func WAV(pcm []byte, rate int) []byte {
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+len(pcm)))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))
	binary.Write(&b, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(&b, binary.LittleEndian, uint16(1)) // mono
	binary.Write(&b, binary.LittleEndian, uint32(rate))
	binary.Write(&b, binary.LittleEndian, uint32(rate*2))
	binary.Write(&b, binary.LittleEndian, uint16(2))
	binary.Write(&b, binary.LittleEndian, uint16(16))
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(len(pcm)))
	b.Write(pcm)
	return b.Bytes()
}

// ElevenVoice is one voice of the owner's ElevenLabs account.
type ElevenVoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ElevenVoices lists the voices the owner's ElevenLabs account can use.
func (c Cloud) ElevenVoices(ctx context.Context) ([]ElevenVoice, error) {
	base := firstOf(c.Base, "https://api.elevenlabs.io/v1")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/voices", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", c.Key)
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs is unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, errors.New("elevenlabs refused the API key")
	}
	var r struct {
		Voices []struct {
			ID   string `json:"voice_id"`
			Name string `json:"name"`
		} `json:"voices"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("elevenlabs: unreadable voice list")
	}
	out := []ElevenVoice{}
	for _, v := range r.Voices {
		out = append(out, ElevenVoice{v.ID, v.Name})
	}
	return out, nil
}

func firstOf(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
