// Package undo reverses recorded actions: a restored email, a removed
// label or draft, a cancelled send. Every undo is itself recorded.
package undo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/denerFernandes/pimpo/internal/event"
	"github.com/denerFernandes/pimpo/internal/host"
)

const EventUndone = "action.undone"

// Mail is the part of the mail connector undo needs.
type Mail interface {
	Restore(ctx context.Context, from, msgID string) error
	Remove(ctx context.Context, from, msgID string) error
}

type Outbox interface {
	Cancel(ctx context.Context, id int64) error
}

type Undo struct {
	Events *event.Store
	Mail   func(ctx context.Context) (Mail, error)
	Outbox Outbox
	// Call runs the undo step a connector gave with its result.
	Call func(ctx context.Context, capability string, args any) (any, error)
}

// step is how a connector says a change is undone: call this capability
// with these arguments.
func step(result map[string]any) (string, any, bool) {
	u, _ := result["undo"].(map[string]any)
	c, _ := u["capability"].(string)
	return c, u["args"], c != ""
}

var ErrNotUndoable = errors.New("this action cannot be undone")
var ErrAlreadyUndone = errors.New("this action was already undone")

// Plan says whether an action can be undone and until when (zero: no limit).
func Plan(rec host.ActionRecord, result map[string]any) (bool, time.Time) {
	if rec.DryRun || rec.Error != "" {
		return false, time.Time{}
	}
	if _, _, ok := step(result); ok {
		return true, time.Time{}
	}
	switch done(rec) {
	case "gmail.archive", "gmail.trash":
		return result["moved_to"] != nil && result["message_id"] != "", time.Time{}
	case "gmail.label":
		return result["message_id"] != "", time.Time{}
	case "gmail.draft":
		return result["saved_in"] != nil, time.Time{}
	case "outbox.send_later":
		at, _ := time.Parse(time.RFC3339, fmt.Sprint(result["send_at"]))
		return true, at
	}
	return false, time.Time{}
}

func done(rec host.ActionRecord) string {
	if rec.Done != "" {
		return rec.Done
	}
	return rec.Capability
}

func (u *Undo) Undo(ctx context.Context, id int64, actor string) error {
	ev, err := u.Events.ByID(ctx, id)
	if err != nil {
		return err
	}
	if ev.Type != host.ActionEvent {
		return ErrNotUndoable
	}
	prior, _ := u.Events.List(ctx, event.Query{Types: []string{EventUndone}, Search: fmt.Sprintf(`"action":%d,`, id)})
	if len(prior) > 0 {
		return ErrAlreadyUndone
	}
	var rec host.ActionRecord
	if err := ev.Decode(&rec); err != nil {
		return err
	}
	var result map[string]any
	if len(rec.Result) > 0 {
		jsonUnmarshal(rec.Result, &result)
	}
	ok, _ := Plan(rec, result)
	if !ok {
		return ErrNotUndoable
	}
	str := func(k string) string { s, _ := result[k].(string); return s }
	if c, args, ok := step(result); ok {
		if u.Call == nil {
			return ErrNotUndoable
		}
		if _, err := u.Call(ctx, c, args); err != nil {
			return err
		}
		_, err = u.Events.Append(ctx, EventUndone, actor, map[string]any{"action": id, "capability": rec.Capability, "source": rec.Source})
		return err
	}
	switch done(rec) {
	case "gmail.archive", "gmail.trash":
		m, err := u.Mail(ctx)
		if err == nil {
			err = m.Restore(ctx, str("moved_to"), str("message_id"))
		}
		if err != nil {
			return err
		}
	case "gmail.label":
		m, err := u.Mail(ctx)
		if err == nil {
			err = m.Remove(ctx, str("label"), str("message_id"))
		}
		if err != nil {
			return err
		}
	case "gmail.draft":
		m, err := u.Mail(ctx)
		if err == nil {
			err = m.Remove(ctx, str("saved_in"), str("message_id"))
		}
		if err != nil {
			return err
		}
	case "outbox.send_later":
		n, _ := result["outbox_id"].(float64)
		if err := u.Outbox.Cancel(ctx, int64(n)); err != nil {
			return err
		}
	}
	_, err = u.Events.Append(ctx, EventUndone, actor, map[string]any{"action": id, "capability": rec.Capability, "source": rec.Source})
	return err
}
