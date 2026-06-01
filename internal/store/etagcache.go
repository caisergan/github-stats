package store

import (
	"context"
	"database/sql"
)

// ETagEntry is a cached conditional-GET response keyed by URL.
type ETagEntry struct {
	URL          string
	ETag         string
	Status       int
	Body         []byte
	LastModified string
}

// GetETag returns the cached entry for a URL, or ErrNotFound.
func (s *Store) GetETag(ctx context.Context, url string) (*ETagEntry, error) {
	var e ETagEntry
	err := s.DB.QueryRowContext(ctx,
		`SELECT url, etag, status, body, last_modified FROM etags WHERE url = ?`, url,
	).Scan(&e.URL, &e.ETag, &e.Status, &e.Body, &e.LastModified)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// PutETag inserts or replaces the cached entry for a URL.
func (s *Store) PutETag(ctx context.Context, e *ETagEntry) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO etags (url, etag, status, body, last_modified, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(url) DO UPDATE SET
			etag = excluded.etag,
			status = excluded.status,
			body = excluded.body,
			last_modified = excluded.last_modified,
			updated_at = CURRENT_TIMESTAMP`,
		e.URL, e.ETag, e.Status, e.Body, e.LastModified,
	)
	return err
}
