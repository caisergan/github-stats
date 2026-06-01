package store

import (
	"context"
	"database/sql"
	"time"
)

// SyncJob is one durable unit of sync work (spec §5/§6).
type SyncJob struct {
	ID          int64
	RepoID      int64
	Kind        string // "backfill" | "delta"
	Status      string // "pending" | "running" | "done" | "error"
	CursorState string
	Attempts    int
	NextRunAt   time.Time
	LockedAt    *time.Time
	LastError   string
	CreatedAt   time.Time
}

// EnqueueJob inserts a pending job runnable immediately (next_run_at = now).
func (s *Store) EnqueueJob(ctx context.Context, repoID int64, kind string, now time.Time) (int64, error) {
	return s.EnqueueJobAt(ctx, repoID, kind, now)
}

// EnqueueJobAt inserts a pending job runnable at runAt.
func (s *Store) EnqueueJobAt(ctx context.Context, repoID int64, kind string, runAt time.Time) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO sync_jobs (repo_id, kind, status, next_run_at)
		VALUES (?, ?, 'pending', ?)`,
		repoID, kind, runAt.UTC(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// LeaseNextJob atomically claims the oldest runnable pending job whose
// next_run_at <= now and that is not locked, flipping it to 'running' and
// stamping locked_at, then returns the claimed row in the same statement. It
// returns (nil, nil) when no job is runnable. The claim-and-return is one
// conditional UPDATE ... RETURNING, so concurrent leasers never get the same
// row and there is no separate re-select to race (SQLite 3.35+ RETURNING; the
// pure-Go modernc.org/sqlite driver supports it).
func (s *Store) LeaseNextJob(ctx context.Context, now time.Time) (*SyncJob, error) {
	nowUTC := now.UTC()
	row := s.DB.QueryRowContext(ctx, `
		UPDATE sync_jobs
		SET status = 'running', locked_at = ?
		WHERE id = (
			SELECT id FROM sync_jobs
			WHERE status = 'pending' AND next_run_at <= ?
			ORDER BY next_run_at ASC, id ASC
			LIMIT 1
		)
		RETURNING id, repo_id, kind, status, cursor_state, attempts,
			next_run_at, locked_at, last_error, created_at`,
		nowUTC, nowUTC,
	)
	job, err := s.scanJob(row)
	if err == ErrNotFound { // no runnable job -> UPDATE affected 0 rows -> no RETURNING row
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return job, nil
}

// CompleteJob marks a job done.
func (s *Store) CompleteJob(ctx context.Context, id int64, now time.Time) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE sync_jobs SET status = 'done', locked_at = NULL WHERE id = ?`, id)
	return err
}

// FailJob records an error and increments attempts. If attempts now reaches
// maxAttempts the job becomes terminal ('error'); otherwise it is rescheduled
// to 'pending' at now+backoff with locked_at cleared.
func (s *Store) FailJob(ctx context.Context, id int64, msg string, now time.Time, backoff time.Duration, maxAttempts int) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var attempts int
		if err := tx.QueryRowContext(ctx,
			`SELECT attempts FROM sync_jobs WHERE id = ?`, id).Scan(&attempts); err != nil {
			return err
		}
		attempts++
		if attempts >= maxAttempts {
			_, err := tx.ExecContext(ctx, `
				UPDATE sync_jobs
				SET status = 'error', attempts = ?, last_error = ?, locked_at = NULL
				WHERE id = ?`, attempts, msg, id)
			return err
		}
		_, err := tx.ExecContext(ctx, `
			UPDATE sync_jobs
			SET status = 'pending', attempts = ?, last_error = ?, next_run_at = ?, locked_at = NULL
			WHERE id = ?`, attempts, msg, now.Add(backoff).UTC(), id)
		return err
	})
}

// RescheduleJob returns a job to 'pending' at runAt WITHOUT bumping attempts —
// used when a job yields voluntarily (e.g. rate-limit budget exhausted) rather
// than failing. The cursor is already persisted in sync_state.
func (s *Store) RescheduleJob(ctx context.Context, id int64, runAt time.Time) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE sync_jobs
		SET status = 'pending', next_run_at = ?, locked_at = NULL
		WHERE id = ?`, runAt.UTC(), id)
	return err
}

// ListJobsForRepo returns all jobs for a repo, newest first.
func (s *Store) ListJobsForRepo(ctx context.Context, repoID int64) ([]SyncJob, error) {
	rows, err := s.DB.QueryContext(ctx, jobSelect+` WHERE repo_id = ? ORDER BY id DESC`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []SyncJob
	for rows.Next() {
		j, err := scanJobRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

const jobSelect = `SELECT id, repo_id, kind, status, cursor_state, attempts,
	next_run_at, locked_at, last_error, created_at FROM sync_jobs`

func (s *Store) scanJob(row *sql.Row) (*SyncJob, error) {
	var j SyncJob
	err := row.Scan(&j.ID, &j.RepoID, &j.Kind, &j.Status, &j.CursorState, &j.Attempts,
		&j.NextRunAt, &j.LockedAt, &j.LastError, &j.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func scanJobRows(rows *sql.Rows) (*SyncJob, error) {
	var j SyncJob
	err := rows.Scan(&j.ID, &j.RepoID, &j.Kind, &j.Status, &j.CursorState, &j.Attempts,
		&j.NextRunAt, &j.LockedAt, &j.LastError, &j.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &j, nil
}
