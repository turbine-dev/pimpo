package app

import (
	"context"
	"github.com/denerFernandes/pimpo/internal/i18n"
	"sync"
	"time"

	"github.com/denerFernandes/pimpo/internal/explore"
)

// Channel health: each channel reports how its last attempts went. One
// that keeps failing for a few minutes is announced on the others, once,
// and again when it comes back.

const downAfter = 3 * time.Minute

type channelState struct {
	failingSince time.Time
	lastErr      string
	failures     int
	announced    bool
}

type channelHealth struct {
	mu    sync.Mutex
	state map[string]*channelState
	now   func() time.Time
}

func (h *channelHealth) clock() time.Time {
	if h.now != nil {
		return h.now()
	}
	return time.Now()
}

func (h *channelHealth) report(channel string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state == nil {
		h.state = map[string]*channelState{}
	}
	s := h.state[channel]
	if s == nil {
		s = &channelState{}
		h.state[channel] = s
	}
	if err == nil {
		s.failingSince, s.lastErr, s.failures = time.Time{}, "", 0
		return
	}
	s.failures++
	if s.failingSince.IsZero() {
		s.failingSince = h.clock()
	}
	s.lastErr = err.Error()
}

// down reports whether a channel has been failing long enough to count.
func (h *channelHealth) down(channel string) (bool, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.state[channel]
	if s == nil || s.failingSince.IsZero() {
		return false, ""
	}
	return s.isDown(h.clock()), s.lastErr
}

// isDown needs more than one failure, so a single lost message is not an
// outage, and minutes of them, so a blip is not either.
func (s *channelState) isDown(now time.Time) bool {
	return s.failures >= 2 && !s.failingSince.IsZero() && now.Sub(s.failingSince) >= downAfter
}

// forget drops what is known of a channel that was turned off or set up again.
func (h *channelHealth) forget(channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.state, channel)
}

// changes returns the channels that just went down or came back.
func (h *channelHealth) changes() (wentDown, cameBack map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	wentDown, cameBack = map[string]string{}, map[string]string{}
	now := h.clock()
	for name, s := range h.state {
		isDown := s.isDown(now)
		switch {
		case isDown && !s.announced:
			s.announced = true
			wentDown[name] = s.lastErr
		case !isDown && s.announced && s.failingSince.IsZero():
			s.announced = false
			cameBack[name] = ""
		}
	}
	return wentDown, cameBack
}

var channelNames = map[string]string{"telegram": "Telegram", "discordchat": "Discord", "slackchat": "Slack", "signal": "Signal"}

func (a *App) healthLoop(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
		a.announceHealth(ctx)
	}
}

func (a *App) announceHealth(ctx context.Context) {
	down, back := a.health.changes()
	for ch, why := range down {
		a.Events.Append(ctx, "channel.down", "system", map[string]string{"channel": ch, "error": why})
		a.Channel.Notify(ctx, explore.Notice{Kind: "failure",
			Text: i18n.T(ctx, "msg.health.down", "channel", channelNames[ch], "error", why)})
	}
	for ch := range back {
		a.Events.Append(ctx, "channel.back", "system", map[string]string{"channel": ch})
		a.Channel.Notify(ctx, explore.Notice{Kind: "failure", Text: i18n.T(ctx, "msg.health.back", "channel", channelNames[ch])})
	}
}
