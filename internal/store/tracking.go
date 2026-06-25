package store

import (
	"context"
	"database/sql"
)

// TrackRepo records that user tracks repo (idempotent).
func (s *Store) TrackRepo(ctx context.Context, userID, repoID int64) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO repo_tracking (user_id, repo_id) VALUES (?, ?)
		ON CONFLICT(user_id, repo_id) DO NOTHING`,
		userID, repoID,
	)
	return err
}

// UntrackRepo removes a user's tracking of a repo (no error if absent).
func (s *Store) UntrackRepo(ctx context.Context, userID, repoID int64) error {
	_, err := s.DB.ExecContext(ctx,
		`DELETE FROM repo_tracking WHERE user_id = ? AND repo_id = ?`, userID, repoID)
	return err
}

// IsTracked reports whether user tracks repo.
func (s *Store) IsTracked(ctx context.Context, userID, repoID int64) (bool, error) {
	var one int
	err := s.DB.QueryRowContext(ctx,
		`SELECT 1 FROM repo_tracking WHERE user_id = ? AND repo_id = ?`, userID, repoID,
	).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ListTrackedRepos returns the full Repo rows a user tracks, newest tracking first.
func (s *Store) ListTrackedRepos(ctx context.Context, userID int64) ([]Repo, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT r.id, r.github_id, r.full_name, r.is_private, r.default_branch,
			r.description, r.stargazers, r.forks, r.primary_language, r.language_color, r.languages, r.created_at
		FROM repo_tracking t
		JOIN repos r ON r.id = t.repo_id
		WHERE t.user_id = ?
		ORDER BY t.created_at DESC, r.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var repos []Repo
	for rows.Next() {
		var r Repo
		var priv int
		if err := rows.Scan(&r.ID, &r.GitHubID, &r.FullName, &priv, &r.DefaultBranch,
			&r.Description, &r.Stargazers, &r.Forks, &r.PrimaryLanguage, &r.LanguageColor, &r.Languages, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.IsPrivate = priv != 0
		repos = append(repos, r)
	}
	return repos, rows.Err()
}
