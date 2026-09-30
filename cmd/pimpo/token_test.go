package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/event"
)

// The login link is shown when made and when asked for; rotating it makes
// the old one useless.
func TestTokenIsPrintedAndRotated(t *testing.T) {
	t.Setenv("PIMPO_TOKEN", "")
	home := t.TempDir()
	s, err := event.Open(filepath.Join(home, "pimpo.db"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, created, _ := sessionToken(ctx, s)
	if !created {
		t.Fatal("a new token was not marked as made")
	}
	if again, created, _ := sessionToken(ctx, s); created || again != first {
		t.Fatal("the kept token was made again")
	}
	s.Put(ctx, listenKey, "0.0.0.0:7788")
	s.Close()

	var out bytes.Buffer
	if err := tokenCmd([]string{"--data", home}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "http://127.0.0.1:7788/auth?token="+first) {
		t.Fatalf("pimpo token: %s", out.String())
	}
	out.Reset()
	if err := tokenCmd([]string{"rotate", "--data", home}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), first) || !strings.Contains(out.String(), "/auth?token=") {
		t.Fatalf("pimpo token rotate: %s", out.String())
	}
	s, _ = event.Open(filepath.Join(home, "pimpo.db"))
	defer s.Close()
	if now, _ := s.Get(ctx, tokenKey); now == first || now == "" {
		t.Fatal("the token was not replaced")
	}
}
