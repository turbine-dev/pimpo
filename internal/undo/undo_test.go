package undo

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/denerFernandes/pimpo/internal/event"
	"github.com/denerFernandes/pimpo/internal/host"
)

type fakeMail struct{ restored, removed []string }

func (f *fakeMail) Restore(_ context.Context, from, id string) error {
	f.restored = append(f.restored, from+" "+id)
	return nil
}
func (f *fakeMail) Remove(_ context.Context, from, id string) error {
	f.removed = append(f.removed, from+" "+id)
	return nil
}

type fakeOutbox struct{ cancelled []int64 }

func (f *fakeOutbox) Cancel(_ context.Context, id int64) error {
	f.cancelled = append(f.cancelled, id)
	return nil
}

func TestUndoEachKind(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	defer ev.Close()
	mail, box := &fakeMail{}, &fakeOutbox{}
	u := &Undo{Events: ev, Outbox: box, Mail: func(context.Context) (Mail, error) { return mail, nil }}
	ctx := context.Background()
	add := func(rec host.ActionRecord, result string) int64 {
		rec.Result = []byte(result)
		e, _ := ev.Append(ctx, host.ActionEvent, rec.Source, rec)
		return e.ID
	}
	trash := add(host.ActionRecord{Source: "routine:x#1", Capability: "gmail.delete", Done: "gmail.trash"}, `{"ok":true,"moved_to":"Trash","message_id":"<m1@x>"}`)
	send := add(host.ActionRecord{Source: "routine:x#1", Capability: "gmail.send", Done: "outbox.send_later"}, `{"ok":true,"outbox_id":7,"send_at":"2026-09-24T09:10:00Z"}`)
	label := add(host.ActionRecord{Source: "routine:x#1", Capability: "gmail.label"}, `{"ok":true,"label":"Contratos","message_id":"<m2@x>"}`)
	read := add(host.ActionRecord{Source: "routine:x#1", Capability: "gmail.search"}, `[]`)
	dry := add(host.ActionRecord{Source: "exploration:e", Capability: "gmail.archive", DryRun: true}, `{"ok":true,"dry_run":true}`)

	for _, id := range []int64{trash, send, label} {
		if err := u.Undo(ctx, id, "human:owner"); err != nil {
			t.Fatalf("undo %d: %v", id, err)
		}
	}
	if mail.restored[0] != "Trash <m1@x>" || box.cancelled[0] != 7 || mail.removed[0] != "Contratos <m2@x>" {
		t.Fatalf("restored %v cancelled %v removed %v", mail.restored, box.cancelled, mail.removed)
	}
	if err := u.Undo(ctx, trash, "human:owner"); !errors.Is(err, ErrAlreadyUndone) {
		t.Fatalf("second undo: %v", err)
	}
	for _, id := range []int64{read, dry} {
		if err := u.Undo(ctx, id, "human:owner"); !errors.Is(err, ErrNotUndoable) {
			t.Fatalf("undo %d: %v", id, err)
		}
	}
}

// A connector's change that says how to undo itself is undone by calling
// that step.
func TestUndoStep(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	defer ev.Close()
	var called []string
	u := &Undo{Events: ev, Call: func(_ context.Context, name string, args any) (any, error) {
		called = append(called, name+" "+args.(map[string]any)["id"].(string))
		return nil, nil
	}}
	ctx := context.Background()
	rec := host.ActionRecord{Source: "routine:x#1", Capability: "apple.reminders.add", Result: []byte(`{"id":"r1","undo":{"capability":"apple.reminders.delete","args":{"id":"r1"}}}`)}
	e, _ := ev.Append(ctx, host.ActionEvent, rec.Source, rec)
	if err := u.Undo(ctx, e.ID, "human:owner"); err != nil || len(called) != 1 || called[0] != "apple.reminders.delete r1" {
		t.Fatalf("%v %v", err, called)
	}
	if err := u.Undo(ctx, e.ID, "human:owner"); !errors.Is(err, ErrAlreadyUndone) {
		t.Fatalf("second undo: %v", err)
	}
}
