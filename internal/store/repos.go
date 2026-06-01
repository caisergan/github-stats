package store

import (
	"context"
	"database/sql"
	"time"
)

// Repo is a tracked GitHub repository.
type Repo struct {
	ID            int64
	GitHubID      int64
	FullName      string // "owner/name"
	IsPrivate     bool
	DefaultBranch string
	Description   string
	Stargazers    int64
	Forks         int64
	CreatedAt     time.Time
}

// UpsertRepo inserts or updates a repo by github_id and returns the local id.
func (s *Store) UpsertRepo(ctx context.Context, r *Repo) (int64, error) {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO repos (github_id, full_name, is_private, default_branch, description, stargazers, forks)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(github_id) DO UPDATE SET
			full_name = excluded.full_name,
			is_private = excluded.is_private,
			default_branch = excluded.default_branch,
			description = excluded.description,
			stargazers = excluded.stargazers,
			forks = excluded.forks`,
		r.GitHubID, r.FullName, boolToInt(r.IsPrivate), r.DefaultBranch,
		r.Description, r.Stargazers, r.Forks,
	)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := s.DB.QueryRowContext(ctx,
		`SELECT id FROM repos WHERE github_id = ?`, r.GitHubID,
	).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// GetRepo returns the repo with the given local id, or ErrNotFound.
func (s *Store) GetRepo(ctx context.Context, id int64) (*Repo, error) {
	return s.scanRepo(s.DB.QueryRowContext(ctx, repoSelect+` WHERE id = ?`, id))
}

// GetRepoByFullName returns the repo with the given "owner/name", or ErrNotFound.
func (s *Store) GetRepoByFullName(ctx context.Context, fullName string) (*Repo, error) {
	return s.scanRepo(s.DB.QueryRowContext(ctx, repoSelect+` WHERE full_name = ?`, fullName))
}

const repoSelect = `SELECT id, github_id, full_name, is_private, default_branch,
	description, stargazers, forks, created_at FROM repos`

func (s *Store) scanRepo(row *sql.Row) (*Repo, error) {
	var r Repo
	var priv int
	err := row.Scan(&r.ID, &r.GitHubID, &r.FullName, &priv, &r.DefaultBranch,
		&r.Description, &r.Stargazers, &r.Forks, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.IsPrivate = priv != 0
	return &r, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
