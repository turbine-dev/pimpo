// Package outbox holds emails the policy made reversible: they wait a few
// minutes before leaving, and until then the owner can cancel them.
package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
)

const (
	Queued    = "queued"
	Sent      = "sent"
	Cancelled = "cancelled"
	Failed    = "failed"
)

var ErrTooLate = errors.New("this email has already been sent")

// Sender delivers a queued message; the app routes it to gmail.send.
type Sender func(ctx context.Context, args any) (any, error)

type Outbox struct {
	DB     *sql.DB
	Events *event.Store
	Send   Sender
	Delay  time.Duration
	Now    func() time.Time
}

type Item struct {
	ID     int64     `json:"id"`
	Args   any       `json:"args"`
	SendAt time.Time `json:"send_at"`
	State  string    `json:"state"`
	Error  string    `json:"error,omitempty"`
}

const schema = `CREATE TABLE IF NOT EXISTS outbox (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  args    TEXT NOT NULL,
  send_at TEXT NOT NULL,
  state   TEXT NOT NULL,
  error   TEXT
)`

func (o *Outbox) Init() error { _, err := o.DB.Exec(schema); return err }

func (o *Outbox) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o *Outbox) Capabilities() []string { return []string{"outbox.send_later"} }

// Call queues an email for sending after the delay.
func (o *Outbox) Call(ctx context.Context, _, _ string, args any) (any, error) {
	delay := o.Delay
	if delay == 0 {
		delay = 10 * time.Minute
	}
	b, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	at := o.now().Add(delay).UTC()
	res, err := o.DB.ExecContext(ctx, `INSERT INTO outbox (args, send_at, state) VALUES (?, ?, ?)`, string(b), at.Format(time.RFC3339Nano), Queued)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return map[string]any{"ok": true, "queued": true, "outbox_id": id, "send_at": at.Format(time.RFC3339)}, nil
}

// Cancel stops a queued email.
func (o *Outbox) Cancel(ctx context.Context, id int64) error {
	res, err := o.DB.ExecContext(ctx, `UPDATE outbox SET state = ? WHERE id = ? AND state = ?`, Cancelled, id, Queued)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTooLate
	}
	return nil
}

func (o *Outbox) Get(ctx context.Context, id int64) (Item, error) {
	var it Item
	var args, at string
	var errText sql.NullString
	err := o.DB.QueryRowContext(ctx, `SELECT id, args, send_at, state, error FROM outbox WHERE id = ?`, id).Scan(&it.ID, &args, &at, &it.State, &errText)
	if err != nil {
		return it, err
	}
	json.Unmarshal([]byte(args), &it.Args)
	it.SendAt, _ = time.Parse(time.RFC3339Nano, at)
	it.Error = errText.String
	return it, nil
}

// Flush sends every email whose time has come. It returns how many left.
func (o *Outbox) Flush(ctx context.Context) (int, error) {
	rows, err := o.DB.QueryContext(ctx, `SELECT id FROM outbox WHERE state = ? AND send_at <= ? ORDER BY id`, Queued, o.now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	sent := 0
	for _, id := range ids {
		// Claim it first, so a cancel racing the send loses cleanly.
		res, err := o.DB.ExecContext(ctx, `UPDATE outbox SET state = 'sending' WHERE id = ? AND state = ?`, id, Queued)
		if err != nil {
			return sent, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}
		it, _ := o.Get(ctx, id)
		_, err = o.Send(ctx, it.Args)
		if err != nil {
			o.DB.ExecContext(ctx, `UPDATE outbox SET state = ?, error = ? WHERE id = ?`, Failed, err.Error(), id)
			o.Events.Append(ctx, "outbox.failed", "system", map[string]any{"outbox_id": id, "error": err.Error()})
			continue
		}
		o.DB.ExecContext(ctx, `UPDATE outbox SET state = ? WHERE id = ?`, Sent, id)
		o.Events.Append(ctx, "outbox.sent", "system", map[string]any{"outbox_id": id, "args": it.Args})
		sent++
	}
	return sent, nil
}

// Run flushes every interval until ctx ends.
func (o *Outbox) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if _, err := o.Flush(ctx); err != nil {
				o.Events.Append(ctx, "outbox.failed", "system", map[string]string{"error": fmt.Sprint(err)})
			}
		case <-ctx.Done():
			return
		}
	}
}
