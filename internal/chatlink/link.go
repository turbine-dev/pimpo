// Package chatlink lets the owner talk to Zodim on Discord, Slack and
// Signal, in private messages. Each link keeps a connection open, hands
// over what arrives and sends replies; the app decides who may talk.
package chatlink

import (
	"context"
	"net/http"
	"time"
)

// Inbound is a private message someone sent to Zodim.
type Inbound struct {
	// From identifies the sender in that service: a user id or a number.
	From string
	// Chat is where to answer; empty means answer From directly.
	Chat string
	Text string
}

type Link interface {
	Name() string
	// Run keeps the connection open, calling on for each message, until
	// ctx ends or the connection fails.
	Run(ctx context.Context, on func(Inbound)) error
	// Send writes to a person, by the id Inbound.From gave.
	Send(ctx context.Context, to, text string) error
	// Check verifies the credentials without sending anything.
	Check(ctx context.Context) error
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Keep runs a link again after it fails, waiting longer each time.
func Keep(ctx context.Context, l Link, on func(Inbound), failed func(error)) {
	wait := time.Second
	for ctx.Err() == nil {
		start := time.Now()
		err := l.Run(ctx, on)
		if ctx.Err() != nil {
			return
		}
		if failed != nil && err != nil {
			failed(err)
		}
		if time.Since(start) > time.Minute {
			wait = time.Second
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return
		}
		wait = min(wait*2, 5*time.Minute)
	}
}
