package owner

import (
	"time"

	"github.com/turbine-dev/pimpo/internal/people"
)

// The owner's pairing code is what makes a chat the owner's, on every
// channel, so it is long, short-lived and used once. Wrong codes count:
// a sender who keeps guessing is ignored for a while, and many wrong
// codes from anyone change the code.
const (
	codeLife    = 15 * time.Minute
	senderTries = 3
	senderPause = time.Hour
	globalTries = 10
	maxSenders  = 1024
)

type tries struct {
	wrong int
	last  time.Time
}

// PairingCode returns the code the owner sends as "/start <code>". It
// lasts 15 minutes and changes after every pairing.
func (c *Channel) PairingCode() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentCode()
}

func (c *Channel) currentCode() string {
	if c.code == "" || time.Since(c.made) > codeLife {
		c.code, c.made, c.wrong = people.NewCode(), time.Now(), 0
	}
	return c.code
}

// ClaimOwner reports whether code is the owner's pairing code; a match
// uses the code up, so it pairs one chat on one channel.
func (c *Channel) ClaimOwner(code string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !people.SameCode(code, c.currentCode()) {
		return false
	}
	c.code = ""
	return true
}

// Blocked says whether a sender ("telegram:42", "signal:+55…") tried too
// many wrong codes lately; their codes are not even checked.
func (c *Channel) Blocked(sender string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.senders[sender]
	return t != nil && t.wrong >= senderTries && time.Since(t.last) < senderPause
}

// Wrong counts a wrong code from sender.
func (c *Channel) Wrong(sender string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.senders == nil {
		c.senders = map[string]*tries{}
	}
	if len(c.senders) >= maxSenders {
		for k, t := range c.senders {
			if time.Since(t.last) >= senderPause {
				delete(c.senders, k)
			}
		}
	}
	t := c.senders[sender]
	if t == nil || time.Since(t.last) >= senderPause {
		t = &tries{}
		if len(c.senders) < maxSenders {
			c.senders[sender] = t
		}
	}
	t.wrong++
	t.last = time.Now()
	c.wrong++
	if c.wrong >= globalTries {
		// Someone is guessing: the code they guess at goes away.
		c.code = ""
	}
}
