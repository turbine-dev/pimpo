package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/browser"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/llm"
)

func TestBrowserOnlyWhenTurnedOnAndOnlyWhereAllowed(t *testing.T) {
	if chromePath() == "" {
		t.Skip("Chrome is not installed")
	}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<title>Pedido</title><p>Pedido 1042: enviado</p><a href="/rastreio">Rastrear</a>`)
	}))
	defer site.Close()
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.Home = t.TempDir()
	defer func() {
		if ta.browser != nil {
			ta.browser.Close()
		}
	}()
	ctx := host.WithSource(context.Background(), "routine:pedidos#1")
	args := map[string]any{"url": site.URL + "/"}
	if _, err := (browserCap{ta.App}).Call(ctx, "browser.open", "127.0.0.1", args); err == nil || !strings.Contains(err.Error(), "Laboratório") {
		t.Fatalf("opened while off: %v", err)
	}
	s := ta.Settings(ctx)
	s.LabsOn = []string{"browser"}
	ta.SaveSettings(ctx, s, "test")
	if _, err := (browserCap{ta.App}).Call(ctx, "browser.open", "example.com", args); err == nil {
		t.Fatal("opened a host other than its scope")
	}
	out, err := (browserCap{ta.App}).Call(ctx, "browser.open", "127.0.0.1", args)
	if errors.Is(err, browser.ErrNoSandbox) || browser.HeadlessHangsHere(err) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	p := out.(browser.Page)
	if !strings.Contains(p.Text, "Pedido 1042: enviado") || len(p.Elements) != 1 || p.Note == "" {
		t.Fatalf("%+v", p)
	}
	// Another run has not opened this host.
	other := host.WithSource(context.Background(), "routine:outra#1")
	if _, err := (browserCap{ta.App}).Call(other, "browser.read", "", map[string]any{}); err == nil {
		t.Fatal("another run read a page it never opened")
	}
	if _, err := os.Stat(ta.Home + "/browser/profile"); err != nil {
		t.Fatal("no profile of its own")
	}
}
