package store

import (
	"context"
	"database/sql"
	"time"
)

// Commit is a single commit event (source of truth).
type Commit struct {
	SHA          string
	AuthorLogin  string
	CommittedAt  time.Time
	Additions    int64
	Deletions    int64
	IsBot        bool
	MsgFirstLine string
}

// PullRequest is a single pull-request event.
type PullRequest struct {
	Number        int64
	AuthorLogin   string
	State         string // "OPEN" | "CLOSED" | "MERGED"
	CreatedAt     time.Time
	MergedAt      *time.Time
	ClosedAt      *time.Time
	Additions     int64
	Deletions     int64
	ChangedFiles  int64
	CommentsCount int64
	FirstReviewAt *time.Time
	IsBot         bool
	Title         string
}

// Issue is a single issue event.
type Issue struct {
	Number        int64
	AuthorLogin   string
	State         string // "OPEN" | "CLOSED"
	CreatedAt     time.Time
	ClosedAt      *time.Time
	CommentsCount int64
	IsBot         bool
	Title         string
}

// Release is a single release event.
type Release struct {
	Tag         string
	Name        string
	PublishedAt *time.Time
	AuthorLogin string
}

// inTx runs fn inside a transaction, committing on success and rolling back on error.
func (s *Store) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// UpsertCommits batch-upserts a page of commits in one transaction.
func (s *Store) UpsertCommits(ctx context.Context, repoID int64, commits []Commit) error {
	if len(commits) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO commits (repo_id, sha, author_login, committed_at, additions, deletions, is_bot, msg_first_line)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(repo_id, sha) DO UPDATE SET
				author_login = excluded.author_login,
				committed_at = excluded.committed_at,
				additions = excluded.additions,
				deletions = excluded.deletions,
				is_bot = excluded.is_bot,
				msg_first_line = excluded.msg_first_line`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, c := range commits {
			if _, err := stmt.ExecContext(ctx,
				repoID, c.SHA, c.AuthorLogin, c.CommittedAt, c.Additions, c.Deletions,
				boolToInt(c.IsBot), c.MsgFirstLine,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

// UpsertPullRequests batch-upserts a page of pull requests in one transaction.
func (s *Store) UpsertPullRequests(ctx context.Context, repoID int64, prs []PullRequest) error {
	if len(prs) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO pull_requests (repo_id, number, author_login, state, created_at, merged_at, closed_at,
				additions, deletions, changed_files, comments_count, first_review_at, is_bot, title)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(repo_id, number) DO UPDATE SET
				author_login = excluded.author_login,
				state = excluded.state,
				created_at = excluded.created_at,
				merged_at = excluded.merged_at,
				closed_at = excluded.closed_at,
				additions = excluded.additions,
				deletions = excluded.deletions,
				changed_files = excluded.changed_files,
				comments_count = excluded.comments_count,
				first_review_at = excluded.first_review_at,
				is_bot = excluded.is_bot,
				title = excluded.title`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, p := range prs {
			if _, err := stmt.ExecContext(ctx,
				repoID, p.Number, p.AuthorLogin, p.State, p.CreatedAt, p.MergedAt, p.ClosedAt,
				p.Additions, p.Deletions, p.ChangedFiles, p.CommentsCount, p.FirstReviewAt,
				boolToInt(p.IsBot), p.Title,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

// UpsertIssues batch-upserts a page of issues in one transaction.
func (s *Store) UpsertIssues(ctx context.Context, repoID int64, issues []Issue) error {
	if len(issues) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO issues (repo_id, number, author_login, state, created_at, closed_at, comments_count, is_bot, title)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(repo_id, number) DO UPDATE SET
				author_login = excluded.author_login,
				state = excluded.state,
				created_at = excluded.created_at,
				closed_at = excluded.closed_at,
				comments_count = excluded.comments_count,
				is_bot = excluded.is_bot,
				title = excluded.title`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, is := range issues {
			if _, err := stmt.ExecContext(ctx,
				repoID, is.Number, is.AuthorLogin, is.State, is.CreatedAt, is.ClosedAt,
				is.CommentsCount, boolToInt(is.IsBot), is.Title,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

// UpsertReleases batch-upserts a page of releases in one transaction.
func (s *Store) UpsertReleases(ctx context.Context, repoID int64, releases []Release) error {
	if len(releases) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO releases (repo_id, tag, name, published_at, author_login)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(repo_id, tag) DO UPDATE SET
				name = excluded.name,
				published_at = excluded.published_at,
				author_login = excluded.author_login`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, rel := range releases {
			if _, err := stmt.ExecContext(ctx,
				repoID, rel.Tag, rel.Name, rel.PublishedAt, rel.AuthorLogin,
			); err != nil {
				return err
			}
		}
		return nil
	})
}
