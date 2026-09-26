package scheduler

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/denerFernandes/pimpo/internal/capability"
	"github.com/denerFernandes/pimpo/internal/event"
	"github.com/denerFernandes/pimpo/internal/host"
	"github.com/denerFernandes/pimpo/internal/routine"
	"github.com/denerFernandes/pimpo/internal/runtime"
)

// inbox serves test.inbox: the mail it has now, and the queries it got.
type inbox struct {
	mu      sync.Mutex
	mail    []map[string]any
	queries []string
}

func (i *inbox) Capabilities() []string { return []string{"test.inbox"} }
func (i *inbox) Call(_ context.Context, _, _ string, args any) (any, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.queries = append(i.queries, args.(map[string]any)["query"].(string))
	return append([]map[string]any{}, i.mail...), nil
}

func TestWatchRunsOnlyForNewItems(t *testing.T) {
	capability.Register(capability.Spec{Name: "test.inbox", Risk: capability.Read, Signature: "test.inbox({query})", Returns: "[{id, from, subject}]"})
	defer capability.Unregister("test.inbox")
	s, bot, _ := setup(t, "")
	ctx := context.Background()
	box := &inbox{mail: []map[string]any{{"id": "1", "from": "chefe@acme.com", "subject": "antigo"}}}
	s.Env.Router.Add(box)
	code := `async function run() { for (const m of event.items) await telegram.send({text: "Novo e-mail de " + m.from + ": " + m.subject}); }`
	s.Store.SaveRoutine(ctx, "chefe", routine.Routine{Name: "E-mail do chefe", Code: code, Manifest: runtime.Manifest{
		Capabilities: []string{"test.inbox", "telegram.send"},
		Params:       []runtime.Param{{Name: "remetente", Label: "Remetente", Type: "email", Default: "chefe@acme.com"}},
		Watch:        &runtime.Watch{Capability: "test.inbox", Args: map[string]any{"query": "from:{{remetente}}"}, Key: "id", Every: "10m"},
	}}, "test", "owner")

	if n, err := s.Poll(ctx, "chefe"); err != nil || n != 0 || len(bot.sent) != 0 {
		t.Fatalf("the first poll must only learn what is there: %d %v %v", n, err, bot.sent)
	}
	box.mu.Lock()
	box.mail = append(box.mail, map[string]any{"id": "2", "from": "chefe@acme.com", "subject": "reunião amanhã"}, map[string]any{"id": "3", "from": "chefe@acme.com", "subject": "orçamento"})
	box.mu.Unlock()
	if n, err := s.Poll(ctx, "chefe"); err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	if strings.Join(bot.sent, "|") != "Novo e-mail de chefe@acme.com: reunião amanhã|Novo e-mail de chefe@acme.com: orçamento" {
		t.Fatalf("%v", bot.sent)
	}
	if n, _ := s.Poll(ctx, "chefe"); n != 0 || len(bot.sent) != 2 {
		t.Fatal("ran again for items it had seen")
	}
	if box.queries[0] != "from:chefe@acme.com" {
		t.Fatalf("the query ignored the parameter: %v", box.queries)
	}
	quiet, _ := s.Env.Events.List(ctx, event.Query{Types: []string{host.ActionEvent}})
	for _, e := range quiet {
		if strings.Contains(string(e.Data), "test.inbox") {
			t.Fatal("polling filled the receipts")
		}
	}
	if s.Next("chefe").IsZero() == false {
		t.Fatal("a watch-only routine got a schedule")
	}
}

func TestWatchValidation(t *testing.T) {
	capability.Register(capability.Spec{Name: "test.inbox", Risk: capability.Read})
	defer capability.Unregister("test.inbox")
	bad := []runtime.Manifest{
		{Capabilities: []string{"telegram.send"}, Watch: &runtime.Watch{Capability: "telegram.send", Key: "id"}},
		{Capabilities: []string{"telegram.send"}, Watch: &runtime.Watch{Capability: "test.inbox", Key: "id"}},
		{Capabilities: []string{"test.inbox"}, Watch: &runtime.Watch{Capability: "test.inbox"}},
		{Capabilities: []string{"test.inbox"}, Watch: &runtime.Watch{Capability: "test.inbox", Key: "id", Every: "soon"}},
	}
	for i, m := range bad {
		if m.Validate() == nil {
			t.Errorf("manifest %d accepted", i)
		}
	}
	if (runtime.Manifest{Capabilities: []string{"telegram.send"}}).Starts() == nil {
		t.Error("a routine with neither schedule nor watch can be saved")
	}
	if got := (runtime.Watch{Every: "1m"}).Interval().Minutes(); got != 5 {
		t.Errorf("interval floor %v", got)
	}
}
