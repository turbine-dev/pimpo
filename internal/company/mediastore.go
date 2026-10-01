package company

import (
	"context"
	"regexp"
	"time"
)

const mediaSchema = `
CREATE TABLE IF NOT EXISTS company_media (
  id         TEXT PRIMARY KEY,
  company    TEXT NOT NULL,
  data       TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS company_media_company ON company_media (company, created_at);`

// Kinds of media a member makes.
const (
	MediaAudio = "audio"
	MediaImage = "image"
	MediaVideo = "video"
)

// MediaID is what a media file's id looks like, and only that.
var MediaID = regexp.MustCompile(`^m_[0-9a-f]{12}$`)

// A Media is a file a member made: a voice, a screenshot or a video, with
// what its checks found.
type Media struct {
	ID      string    `json:"id"`
	Company string    `json:"company"`
	Member  string    `json:"member"`
	Kind    string    `json:"kind"`
	File    string    `json:"file"`
	Title   string    `json:"title,omitempty"`
	Format  string    `json:"format,omitempty"`
	Seconds float64   `json:"seconds,omitempty"`
	Check   any       `json:"check,omitempty"`
	Work    string    `json:"work,omitempty"`
	Created time.Time `json:"created"`
}

func (s *Store) SaveMedia(ctx context.Context, m Media) error {
	return s.saveRow(ctx, "company_media", m.ID, m.Company, m, m.Created)
}

func (s *Store) MediaList(ctx context.Context, company string) ([]Media, error) {
	return rows[Media](ctx, s, `SELECT data FROM company_media WHERE company = ? ORDER BY created_at DESC LIMIT 300`, company)
}

func (s *Store) MediaItem(ctx context.Context, company, id string) (Media, error) {
	out, err := rows[Media](ctx, s, `SELECT data FROM company_media WHERE id = ? AND company = ?`, id, company)
	if err != nil {
		return Media{}, err
	}
	if len(out) == 0 {
		return Media{}, ErrNotFound
	}
	return out[0], nil
}
