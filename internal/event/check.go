package event

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// ErrDamaged marks a database that failed its integrity check.
var ErrDamaged = errors.New("the database is damaged")

// tailEvents is how many of the newest events the start-up check walks:
// enough to catch a torn or edited tail, cheap on any history size.
const tailEvents = 500

// Check runs SQLite's quick_check and walks the newest events of the hash
// chain. It only reads. A damaged database is reported as ErrDamaged.
func Check(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA quick_check`)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDamaged, err)
	}
	var problems []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			rows.Close()
			return fmt.Errorf("%w: %v", ErrDamaged, err)
		}
		if line != "ok" {
			problems = append(problems, line)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("%w: %v", ErrDamaged, err)
	}
	rows.Close()
	if len(problems) > 0 {
		if len(problems) > 3 {
			problems = problems[:3]
		}
		return fmt.Errorf("%w: %s", ErrDamaged, strings.Join(problems, "; "))
	}
	bad, err := verifyTail(ctx, db, tailEvents)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDamaged, err)
	}
	if bad != 0 {
		return fmt.Errorf("%w: the history breaks at event %d", ErrDamaged, bad)
	}
	return nil
}

// CheckFile checks the database at path without writing to it: it is
// opened read-only, so not even a pending journal is folded in. A file
// that does not exist yet is fine.
func CheckFile(path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDamaged, err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return Check(ctx, db)
}

// verifyTail checks the newest n events: each one's hash, and that each
// links to the one before. It returns the first bad event, or 0.
func verifyTail(ctx context.Context, db *sql.DB, n int) (int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, ts, type, actor, data, prev, hash FROM events ORDER BY id DESC LIMIT ?`, n+1)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()
	var newest []Event
	for rows.Next() {
		var e Event
		var ts, data string
		if err := rows.Scan(&e.ID, &ts, &e.Type, &e.Actor, &data, &e.Prev, &e.Hash); err != nil {
			return 0, err
		}
		e.Time, _ = time.Parse(time.RFC3339Nano, ts)
		e.Data = []byte(data)
		newest = append(newest, e)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for i := len(newest) - 1; i >= 0; i-- {
		e := newest[i]
		if digest(e) != e.Hash {
			return e.ID, nil
		}
		// The oldest one read anchors the walk; it links to the start only
		// when the whole history fits.
		switch {
		case i < len(newest)-1 && e.Prev != newest[i+1].Hash:
			return e.ID, nil
		case i == len(newest)-1 && len(newest) <= n && e.Prev != "":
			return e.ID, nil
		}
	}
	return 0, nil
}
