package speech

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func TestPick(t *testing.T) {
	vs := []Voice{{"Eddy (Português (Brasil))", "pt-BR"}, {"Luciana", "pt-BR"}, {"Joana", "pt-PT"}, {"Samantha", "en-US"}, {"Ava (Premium)", "en-US"}}
	for lang, want := range map[string]string{"pt-BR": "Luciana", "pt_BR": "Luciana", "pt-PT": "Joana", "pt": "Luciana", "en-US": "Ava (Premium)", "en-GB": "Ava (Premium)"} {
		if v, ok := Pick(vs, lang); !ok || v.Name != want {
			t.Errorf("%s: %v, want %s", lang, v, want)
		}
	}
	if _, ok := Pick(vs, "ja-JP"); ok {
		t.Error("picked a voice for a language with none")
	}
}

// On a Mac, a sentence becomes an AAC file of about the right length.
func TestSpeak(t *testing.T) {
	if _, err := exec.LookPath("say"); err != nil {
		t.Skip("no macOS voices")
	}
	b, secs, err := Speak(context.Background(), "Olá! Este é o resumo de hoje do Pimpo.", "pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 2000 || string(b[4:8]) != "ftyp" || secs < 1 || secs > 10 {
		t.Fatalf("%d bytes, %.1fs, header %q", len(b), secs, b[4:8])
	}
}

func TestChunks(t *testing.T) {
	text := strings.Repeat("Uma frase curta. ", 600) // 10200 chars
	parts := Chunks(text, 4000)
	if len(parts) != 3 {
		t.Fatalf("%d parts", len(parts))
	}
	joined := 0
	for _, p := range parts {
		if len([]rune(p)) > 4000 || !strings.HasSuffix(p, ".") {
			t.Fatalf("part of %d chars ending %q", len(p), p[len(p)-5:])
		}
		joined += len(p)
	}
	if one := Chunks("oi", 4000); len(one) != 1 || one[0] != "oi" {
		t.Fatal(one)
	}
}

// OpenAI is asked a piece at a time, the pieces join into one file, and
// the cost is exact per character.
func TestCloudOpenAI(t *testing.T) {
	if _, err := exec.LookPath("afconvert"); err != nil {
		t.Skip("no afconvert")
	}
	var inputs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if r.Header.Get("Authorization") != "Bearer k" || r.URL.Path != "/audio/speech" || body["response_format"] != "pcm" || body["voice"] != "nova" {
			w.WriteHeader(401)
			return
		}
		inputs = append(inputs, body["input"])
		w.Write(make([]byte, 24000*2)) // one second of silence
	}))
	defer srv.Close()
	c := Cloud{Provider: "openai", Key: "k", Base: srv.URL, Voice: "nova"}
	text := strings.Repeat("Bom dia. ", 600)
	b, secs, err := c.Speak(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 || secs < 1.9 || secs > 2.1 || len(b) < 100 {
		t.Fatalf("%d requests, %.2fs, %d bytes", len(inputs), secs, len(b))
	}
	if got := c.Cost(text); got < 0.0809 || got > 0.0811 {
		t.Fatalf("cost %v", got)
	}
	if _, _, err := (Cloud{Provider: "openai", Key: "bad", Base: srv.URL}).Speak(context.Background(), "oi"); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("bad key: %v", err)
	}
}
