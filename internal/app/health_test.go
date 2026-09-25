package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/denerFernandes/zodim/internal/event"
	"github.com/denerFernandes/zodim/internal/owner"
)

func TestChannelHealth(t *testing.T) {
	now := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	h := channelHealth{now: func() time.Time { return now }}
	fail := errors.New("dial tcp: no route to host")

	h.report("telegram", fail)
	now = now.Add(5 * time.Minute)
	if down, _ := h.down("telegram"); down {
		t.Fatal("one failure counted as an outage")
	}
	h.report("telegram", nil)
	h.report("telegram", fail)
	now = now.Add(time.Minute)
	h.report("telegram", fail)
	if down, _ := h.down("telegram"); down {
		t.Fatal("down after one minute")
	}
	now = now.Add(2 * time.Minute)
	h.report("telegram", fail)
	down, why := h.down("telegram")
	if !down || why != fail.Error() {
		t.Fatalf("not down after 3 minutes: %v %q", down, why)
	}
	d, b := h.changes()
	if len(d) != 1 || len(b) != 0 {
		t.Fatalf("changes %v %v", d, b)
	}
	if d, b = h.changes(); len(d)+len(b) != 0 {
		t.Fatalf("announced twice: %v %v", d, b)
	}
	h.report("telegram", nil)
	if d, b = h.changes(); len(b) != 1 || len(d) != 0 {
		t.Fatalf("recovery %v %v", d, b)
	}
	if d, b = h.changes(); len(d)+len(b) != 0 {
		t.Fatalf("recovery announced twice: %v %v", d, b)
	}

	h.report("slackchat", fail)
	h.report("slackchat", fail)
	h.forget("slackchat")
	now = now.Add(10 * time.Minute)
	if d, _ = h.changes(); len(d) != 0 {
		t.Fatalf("a channel turned off was announced: %v", d)
	}
}

func TestChannelDownIsAnnounced(t *testing.T) {
	a := newApp(t, nil, nil)
	ctx := context.Background()
	now := time.Now()
	a.health.mu.Lock()
	a.health.now = func() time.Time { return now }
	a.health.mu.Unlock()
	a.health.report("discordchat", errors.New("gateway closed"))
	a.health.report("discordchat", errors.New("gateway closed"))
	now = now.Add(4 * time.Minute)
	a.announceHealth(ctx)
	a.announceHealth(ctx)
	a.health.report("discordchat", nil)
	a.announceHealth(ctx)

	evs, _ := a.Events.List(ctx, event.Query{Types: []string{owner.EventNotice}})
	var texts []string
	for _, e := range evs {
		var n struct{ Text string }
		e.Decode(&n)
		texts = append(texts, n.Text)
	}
	if len(texts) != 2 || !strings.Contains(texts[0], "Discord parou") || !strings.Contains(texts[0], "gateway closed") || !strings.Contains(texts[1], "Discord voltou") {
		t.Fatalf("notices %q", texts)
	}
}
