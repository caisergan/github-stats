package store

import (
	"context"
	"database/sql"
	"time"
)

// Repo is a tracked GitHub repository.
type Repo struct {
	ID              int64
	GitHubID        int64
	FullName        string // "owner/name"
	IsPrivate       bool
	DefaultBranch   string
	Description     string
	Stargazers      int64
	Forks           int64
	PrimaryLanguage string // e.g. "Go" ("" when GitHub reports none)
	LanguageColor   string // e.g. "#00ADD8"
	Languages       string // JSON array [{name,color,size}], desc by size; default "[]"
	CommitCount     int64  // GitHub's total commits on the default branch; 0 until first sync
	CreatedAt       time.Time
}

// UpsertRepo inserts or updates a repo by github_id and returns the local id.
func (s *Store) UpsertRepo(ctx context.Context, r *Repo) (int64, error) {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO repos (github_id, full_name, is_private, default_branch, description, stargazers, forks, primary_language, language_color, languages, commit_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(github_id) DO UPDATE SET
			full_name = excluded.full_name,
			is_private = excluded.is_private,
			default_branch = excluded.default_branch,
			description = excluded.description,
			stargazers = excluded.stargazers,
			forks = excluded.forks,
			primary_language = excluded.primary_language,
			language_color = excluded.language_color,
			languages = excluded.languages,
			-- never let a metadata blip (commit_count = 0) clobber a known total
			commit_count = MAX(excluded.commit_count, repos.commit_count)`,
		r.GitHubID, r.FullName, boolToInt(r.IsPrivate), r.DefaultBranch,
		r.Description, r.Stargazers, r.Forks, r.PrimaryLanguage, r.LanguageColor, languagesOrEmpty(r.Languages), r.CommitCount,
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

// PurgeRepo hard-deletes a repository and ALL of its stored data. Every
// repo-scoped child table (commits, pull_requests, issues, releases,
// daily_repo_stats, daily_contributor_stats, sync_state, sync_jobs,
// repo_tracking, collection_repos) declares ON DELETE CASCADE, so deleting the
// repos row removes them. The etags HTTP cache is keyed by URL (no repo_id), so
// its rows are purged by URL first — otherwise a re-track could serve stale,
// pre-deletion bodies via conditional requests. Idempotent: a missing repo is a
// no-op.
func (s *Store) PurgeRepo(ctx context.Context, repoID int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var fullName string
		switch err := tx.QueryRowContext(ctx,
			`SELECT full_name FROM repos WHERE id = ?`, repoID).Scan(&fullName); err {
		case sql.ErrNoRows:
			return nil
		case nil:
			// continue
		default:
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM etags WHERE url LIKE '%/repos/' || ? || '/%'`, fullName); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM repos WHERE id = ?`, repoID)
		return err
	})
}

// languagesOrEmpty guards the NOT NULL languages column against a zero-value
// Repo (e.g. metadata built before the languages query existed).
func languagesOrEmpty(s string) string {
	if s == "" {
		return "[]"
	}
	return s
}

const repoSelect = `SELECT id, github_id, full_name, is_private, default_branch,
	description, stargazers, forks, primary_language, language_color, languages, commit_count, created_at FROM repos`

func (s *Store) scanRepo(row *sql.Row) (*Repo, error) {
	var r Repo
	var priv int
	err := row.Scan(&r.ID, &r.GitHubID, &r.FullName, &priv, &r.DefaultBranch,
		&r.Description, &r.Stargazers, &r.Forks, &r.PrimaryLanguage, &r.LanguageColor, &r.Languages, &r.CommitCount, &r.CreatedAt)
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
