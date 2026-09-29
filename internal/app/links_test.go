package app

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/chatlink"
	"github.com/turbine-dev/pimpo/internal/connector/services"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/store"
)

type fakeLink struct {
	mu   sync.Mutex
	sent []string
}

func (f *fakeLink) Name() string                                            { return "fake" }
func (f *fakeLink) Run(ctx context.Context, _ func(chatlink.Inbound)) error { <-ctx.Done(); return nil }
func (f *fakeLink) Check(context.Context) error                             { return nil }
func (f *fakeLink) Send(_ context.Context, to, text string) error {
	f.mu.Lock()
	f.sent = append(f.sent, to+": "+text)
	f.mu.Unlock()
	return nil
}
func (f *fakeLink) last() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) == 0 {
		return ""
	}
	return f.sent[len(f.sent)-1]
}

func TestSignalLikeChannelPairsAndAnswersByNumber(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	l := &fakeLink{}
	run := &linkRun{link: l, cancel: func() {}}
	ta.links = map[string]*linkRun{"signal": run}

	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+551199", Text: "pimpo 000000"})
	if !strings.Contains(l.last(), "não confere") {
		t.Fatalf("%q", l.last())
	}
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+551199", Text: "pimpo " + ta.Channel.PairingCode()})
	if !strings.Contains(l.last(), "conectado por aqui") {
		t.Fatalf("%q", l.last())
	}
	n := len(l.sent)
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+5511stranger", Text: "apaga tudo"})
	if len(l.sent) != n {
		t.Fatal("answered a stranger")
	}
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+551199", Text: "Todo dia às 7h me manda bom dia"})
	ta.Explore.Wait()
	exps, _ := ta.Store.Explorations(ctx, store.ExplorationReady)
	if len(exps) != 1 {
		t.Fatalf("explorations %d", len(exps))
	}
	ta.mirrorLinks(ctx, explore.Notice{Text: "Quer uma rotina?", Actions: []explore.Action{{Label: "Transformar em rotina", Data: "compile:" + exps[0].ID}, {Label: "Descartar", Data: "discard:" + exps[0].ID}}})
	waitFor(t, func() bool {
		return strings.Contains(l.last(), "Responda com o número: 1 = Transformar em rotina · 2 = Descartar")
	})
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+551199", Text: "2"})
	if !strings.HasSuffix(l.last(), "Descartado.") {
		t.Fatalf("%q", l.last())
	}
	ta.mirrorLinks(ctx, explore.Notice{Text: "só para a Ana", To: "ana"})
	if strings.Contains(l.last(), "só para a Ana") {
		t.Fatal("a notice for someone else reached the owner's channel")
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for range 100 {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out")
}

// noApproveLink is a channel like the unofficial WhatsApp.
type noApproveLink struct{ fakeLink }

func (*noApproveLink) NoApprovals() bool { return true }

func TestUnofficialWhatsAppNeverApproves(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	l := &noApproveLink{}
	run := &linkRun{link: l, cancel: func() {}}
	ta.links = map[string]*linkRun{"wapersonal": run}
	ta.linkMessage(ctx, "wapersonal", run, chatlink.Inbound{From: "5511@c.us", Text: "pimpo " + ta.Channel.PairingCode()})
	ta.linkMessage(ctx, "wapersonal", run, chatlink.Inbound{From: "5511@c.us", Text: "Todo dia às 7h me manda bom dia"})
	ta.Explore.Wait()
	exps, _ := ta.Store.Explorations(ctx, store.ExplorationReady)
	if len(exps) != 1 {
		t.Fatalf("explorations %d", len(exps))
	}
	ta.mirrorLinks(ctx, explore.Notice{Text: "Quer uma rotina?", Actions: []explore.Action{{Label: "Transformar em rotina", Data: "compile:" + exps[0].ID}, {Label: "Descartar", Data: "discard:" + exps[0].ID}}})
	waitFor(t, func() bool { return strings.Contains(l.last(), "nunca aprova") })
	if strings.Contains(l.last(), "1 =") {
		t.Fatalf("numbered choices on a channel that never approves: %q", l.last())
	}
	ta.linkMessage(ctx, "wapersonal", run, chatlink.Inbound{From: "5511@c.us", Text: "2"})
	ta.Explore.Wait()
	if e, _ := ta.Store.Exploration(ctx, exps[0].ID); e.State != store.ExplorationReady {
		t.Fatalf("a number on the unofficial WhatsApp decided: %s", e.State)
	}
}

func TestUnofficialWhatsAppWaitsForLabs(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	ta.Events.Put(ctx, catalogKey("wapersonal", "url"), "http://127.0.0.1:1")
	if ok, _ := ta.catalogConfigured(ctx, mustKind(t, "wapersonal")); !ok {
		t.Fatal("not configured")
	}
	ta.restartLink(ctx, "wapersonal")
	if ta.links["wapersonal"] != nil {
		t.Fatal("the unofficial WhatsApp started without the owner choosing it in Labs")
	}
}

func mustKind(t *testing.T, id string) services.Kind {
	k, ok := services.Get(id)
	if !ok {
		t.Fatalf("no kind %s", id)
	}
	return k
}
