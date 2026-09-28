package speech

import (
	"context"
	"os/exec"
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
