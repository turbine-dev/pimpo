// Package chatlink lets the owner talk to Pimpo on Discord, Slack and
// Signal, in private messages. Each link keeps a connection open, hands
// over what arrives and sends replies; the app decides who may talk.
package chatlink

import (
	"context"
	"net/http"
	"time"
)

// Inbound is a private message someone sent to Pimpo.
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

// Keep runs a link again after it fails, waiting longer each time. status
// hears each failure, and nil once a connection has stayed up for a
// minute, so the caller can tell a link that came back from one still down.
func Keep(ctx context.Context, l Link, on func(Inbound), status func(error)) {
	wait := time.Second
	for ctx.Err() == nil {
		start := time.Now()
		alive := time.AfterFunc(time.Minute, func() {
			if status != nil && ctx.Err() == nil {
				status(nil)
			}
		})
		err := l.Run(ctx, on)
		alive.Stop()
		if ctx.Err() != nil {
			return
		}
		if status != nil && err != nil {
			status(err)
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
