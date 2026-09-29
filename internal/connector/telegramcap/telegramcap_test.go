package telegramcap

import (
	"context"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/telegram"
)

type recorder struct{ sent []string }

func (r *recorder) Send(_ context.Context, chat int64, text string, _ ...[]telegram.Button) (telegram.Message, error) {
	r.sent = append(r.sent, text)
	return telegram.Message{}, nil
}

func TestSendsOnlyToTheOwner(t *testing.T) {
	rec := &recorder{}
	o := &Owner{Bot: rec, Chat: func(context.Context) (int64, error) { return 42, nil }}
	if _, err := o.Call(context.Background(), "telegram.send", "", map[string]any{"text": "oi", "chat_id": 999}); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("linha\n", 1500)
	o.Call(context.Background(), "telegram.send", "", map[string]any{"text": long})
	if len(rec.sent) != 4 || strings.Count(strings.Join(rec.sent[1:], "\n"), "linha") != 1500 {
		t.Fatalf("long message split into %d parts, or lost lines", len(rec.sent)-1)
	}
	for _, p := range rec.sent {
		if len([]rune(p)) > 4000 {
			t.Fatalf("part too long: %d", len(p))
		}
	}
	unpaired := &Owner{Bot: rec, Chat: func(context.Context) (int64, error) { return 0, nil }}
	if _, err := unpaired.Call(context.Background(), "", "", map[string]any{"text": "x"}); err == nil || !strings.Contains(err.Error(), "paired") {
		t.Fatalf("unpaired: %v", err)
	}
	if _, err := o.Call(context.Background(), "", "", map[string]any{"text": "  "}); err == nil {
		t.Fatal("sent an empty message")
	}
}
