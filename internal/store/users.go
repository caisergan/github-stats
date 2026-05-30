package store

import (
	"context"
	"database/sql"
	"time"
)

// User is an authenticated GitHub user.
type User struct {
	ID        int64
	GitHubID  int64
	Login     string
	AvatarURL string
	CreatedAt time.Time
}

// UpsertUser inserts or updates a user by github_id and returns the local id.
func (s *Store) UpsertUser(ctx context.Context, u *User) (int64, error) {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO users (github_id, login, avatar_url)
		VALUES (?, ?, ?)
		ON CONFLICT(github_id) DO UPDATE SET
			login = excluded.login,
			avatar_url = excluded.avatar_url`,
		u.GitHubID, u.Login, u.AvatarURL,
	)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := s.DB.QueryRowContext(ctx,
		`SELECT id FROM users WHERE github_id = ?`, u.GitHubID,
	).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// GetUserByID returns the user with the given local id, or ErrNotFound.
func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	var u User
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, github_id, login, avatar_url, created_at FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.GitHubID, &u.Login, &u.AvatarURL, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
