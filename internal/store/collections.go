package store

import (
	"context"
	"database/sql"
	"time"
)

// Collection is a named group of repos owned by a user.
type Collection struct {
	ID        int64
	UserID    int64
	Name      string
	CreatedAt time.Time
}

// CreateCollection creates a collection for the user and returns its id.
func (s *Store) CreateCollection(ctx context.Context, userID int64, name string) (int64, error) {
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO collections (user_id, name) VALUES (?, ?)`, userID, name,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListCollections returns the user's collections, newest first.
func (s *Store) ListCollections(ctx context.Context, userID int64) ([]Collection, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, user_id, name, created_at FROM collections
		 WHERE user_id = ? ORDER BY created_at DESC, id DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Collection, 0)
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListUserCollectionRepoIDs returns, for every collection owned by userID, the
// ordered list of member repo ids — in a single join query. This avoids the
// 1+N pattern of calling ListCollectionRepos per collection when listing.
// Collections with no repos are absent from the map (callers default to []).
func (s *Store) ListUserCollectionRepoIDs(ctx context.Context, userID int64) (map[int64][]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT cr.collection_id, cr.repo_id
		FROM collection_repos cr
		JOIN collections c ON c.id = cr.collection_id
		WHERE c.user_id = ?
		ORDER BY cr.collection_id, cr.created_at, cr.repo_id`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]int64)
	for rows.Next() {
		var collectionID, repoID int64
		if err := rows.Scan(&collectionID, &repoID); err != nil {
			return nil, err
		}
		out[collectionID] = append(out[collectionID], repoID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ownsCollection returns ErrNotFound if the collection is missing or not owned by userID.
func (s *Store) ownsCollection(ctx context.Context, userID, collectionID int64) error {
	var n int
	err := s.DB.QueryRowContext(ctx,
		`SELECT 1 FROM collections WHERE id = ? AND user_id = ?`, collectionID, userID,
	).Scan(&n)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	return err
}

// RenameCollection renames an owned collection. ErrNotFound if not owned.
func (s *Store) RenameCollection(ctx context.Context, userID, collectionID int64, name string) error {
	res, err := s.DB.ExecContext(ctx,
		`UPDATE collections SET name = ? WHERE id = ? AND user_id = ?`,
		name, collectionID, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteCollection deletes an owned collection (cascades collection_repos). ErrNotFound if not owned.
func (s *Store) DeleteCollection(ctx context.Context, userID, collectionID int64) error {
	res, err := s.DB.ExecContext(ctx,
		`DELETE FROM collections WHERE id = ? AND user_id = ?`, collectionID, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AddRepoToCollection adds a repo to an owned collection (idempotent). ErrNotFound if not owned.
func (s *Store) AddRepoToCollection(ctx context.Context, userID, collectionID, repoID int64) error {
	if err := s.ownsCollection(ctx, userID, collectionID); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO collection_repos (collection_id, repo_id) VALUES (?, ?)
		 ON CONFLICT(collection_id, repo_id) DO NOTHING`,
		collectionID, repoID,
	)
	return err
}

// RemoveRepoFromCollection removes a repo from an owned collection. ErrNotFound if not owned.
func (s *Store) RemoveRepoFromCollection(ctx context.Context, userID, collectionID, repoID int64) error {
	if err := s.ownsCollection(ctx, userID, collectionID); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx,
		`DELETE FROM collection_repos WHERE collection_id = ? AND repo_id = ?`,
		collectionID, repoID,
	)
	return err
}

// ListCollectionRepos returns the repos in an owned collection. ErrNotFound if not owned.
func (s *Store) ListCollectionRepos(ctx context.Context, userID, collectionID int64) ([]Repo, error) {
	if err := s.ownsCollection(ctx, userID, collectionID); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT r.id, r.github_id, r.full_name, r.is_private, r.default_branch,
		       r.description, r.stargazers, r.forks, r.primary_language, r.language_color, r.languages, r.created_at
		FROM collection_repos cr
		JOIN repos r ON r.id = cr.repo_id
		WHERE cr.collection_id = ?
		ORDER BY cr.created_at ASC, r.id ASC`, collectionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Repo, 0)
	for rows.Next() {
		var r Repo
		var priv int
		if err := rows.Scan(&r.ID, &r.GitHubID, &r.FullName, &priv,
			&r.DefaultBranch, &r.Description, &r.Stargazers, &r.Forks, &r.PrimaryLanguage, &r.LanguageColor, &r.Languages, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.IsPrivate = priv != 0
		out = append(out, r)
	}
	return out, rows.Err()
}
