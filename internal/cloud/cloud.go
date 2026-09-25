// Package cloud keeps backup files in storage the owner already has: an
// S3 bucket (Amazon or any compatible service) or their Google Drive.
// Files arrive already encrypted; nothing here sees their contents.
package cloud

import (
	"context"
	"io"
	"net/http"
	"time"
)

type Object struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

type Store interface {
	Put(ctx context.Context, name string, data []byte) error
	List(ctx context.Context) ([]Object, error)
	Get(ctx context.Context, name string) ([]byte, error)
	Delete(ctx context.Context, name string) error
}

func client(c *http.Client) *http.Client {
	if c == nil {
		return &http.Client{Timeout: 10 * time.Minute}
	}
	return c
}

// body reads a response up to limit, so an error page cannot fill memory.
func body(r io.Reader, limit int64) []byte {
	b, _ := io.ReadAll(io.LimitReader(r, limit))
	return b
}
