package store

import (
	"context"
	"database/sql"
	"time"
)

// SyncState holds per-repo backfill/delta cursors and status.
type SyncState struct {
	RepoID            int64
	LastCommitAt      *time.Time
	LastCommitCursor  string
	LastPRCursor      string
	LastIssueCursor   string
	LastReleaseCursor string
	LastBackfillAt    *time.Time
	LastDeltaAt       *time.Time
	Status            string // "" | "backfilling" | "complete"
}

// GetSyncState returns the sync state for a repo. When no row exists it returns
// a zero-value state (RepoID set) and a nil error — callers treat absence as "fresh".
func (s *Store) GetSyncState(ctx context.Context, repoID int64) (*SyncState, error) {
	st := &SyncState{RepoID: repoID}
	err := s.DB.QueryRowContext(ctx, `
		SELECT last_commit_at, last_commit_cursor, last_pr_cursor, last_issue_cursor,
			last_release_cursor, last_backfill_at, last_delta_at, status
		FROM sync_state WHERE repo_id = ?`, repoID,
	).Scan(&st.LastCommitAt, &st.LastCommitCursor, &st.LastPRCursor, &st.LastIssueCursor,
		&st.LastReleaseCursor, &st.LastBackfillAt, &st.LastDeltaAt, &st.Status)
	if err == sql.ErrNoRows {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	return st, nil
}

// UpsertSyncState inserts or updates the sync state for a repo.
func (s *Store) UpsertSyncState(ctx context.Context, st *SyncState) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO sync_state (repo_id, last_commit_at, last_commit_cursor, last_pr_cursor,
			last_issue_cursor, last_release_cursor, last_backfill_at, last_delta_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repo_id) DO UPDATE SET
			last_commit_at = excluded.last_commit_at,
			last_commit_cursor = excluded.last_commit_cursor,
			last_pr_cursor = excluded.last_pr_cursor,
			last_issue_cursor = excluded.last_issue_cursor,
			last_release_cursor = excluded.last_release_cursor,
			last_backfill_at = excluded.last_backfill_at,
			last_delta_at = excluded.last_delta_at,
			status = excluded.status`,
		st.RepoID, st.LastCommitAt, st.LastCommitCursor, st.LastPRCursor,
		st.LastIssueCursor, st.LastReleaseCursor, st.LastBackfillAt, st.LastDeltaAt, st.Status,
	)
	return err
}
