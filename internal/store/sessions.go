package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"time"
)

// Session is a server-side login session referenced by an httpOnly cookie.
type Session struct {
	ID        string
	UserID    int64
	ExpiresAt time.Time
}

func newSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateSession creates a session valid for ttl.
func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (*Session, error) {
	id, err := newSessionID()
	if err != nil {
		return nil, err
	}
	exp := time.Now().Add(ttl).UTC()
	if _, err := s.DB.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`,
		id, userID, exp,
	); err != nil {
		return nil, err
	}
	return &Session{ID: id, UserID: userID, ExpiresAt: exp}, nil
}

// GetSession returns a non-expired session, or ErrNotFound.
func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	var sess Session
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, user_id, expires_at FROM sessions WHERE id = ?`, id,
	).Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if time.Now().After(sess.ExpiresAt) {
		_ = s.DeleteSession(ctx, id)
		return nil, ErrNotFound
	}
	return &sess, nil
}

// DeleteSession removes a session (logout).
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}
