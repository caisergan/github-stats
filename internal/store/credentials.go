package store

import (
	"context"
	"database/sql"
)

// Credential is a stored GitHub credential (encrypted token) for a user.
type Credential struct {
	UserID   int64
	Kind     string // "oauth" | "pat"
	EncToken string
	Scopes   string
}

// UpsertCredential inserts or replaces a credential for (user_id, kind).
func (s *Store) UpsertCredential(ctx context.Context, c *Credential) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO credentials (user_id, kind, enc_token, scopes)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id, kind) DO UPDATE SET
			enc_token = excluded.enc_token,
			scopes = excluded.scopes`,
		c.UserID, c.Kind, c.EncToken, c.Scopes,
	)
	return err
}

// GetCredential returns the credential for (user_id, kind), or ErrNotFound.
func (s *Store) GetCredential(ctx context.Context, userID int64, kind string) (*Credential, error) {
	var c Credential
	err := s.DB.QueryRowContext(ctx,
		`SELECT user_id, kind, enc_token, scopes FROM credentials WHERE user_id = ? AND kind = ?`,
		userID, kind,
	).Scan(&c.UserID, &c.Kind, &c.EncToken, &c.Scopes)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}
