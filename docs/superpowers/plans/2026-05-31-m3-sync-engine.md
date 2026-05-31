# M3 — Sync Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the M2 one-shot `backfill.Run` into a continuously-running **sync engine** (spec §6) and expose it over HTTP (spec §10). This delivers: a `0003` migration adding a durable **job queue** (`sync_jobs`) and **per-user repo tracking** (`repo_tracking`) with their DAOs; **delta fetchers** in `githubapi` (`FetchCommitsSince`, `FetchPullRequestsUpdated`, `FetchIssuesUpdated`); a new `internal/sync` package with a **delta sync** routine, a **bounded worker pool**, a **scheduler** that enqueues periodic delta jobs, a per-repo **progress broadcaster** for SSE, and an `Engine` that owns all three with `Start(ctx)`/`Stop()`; and the auth-gated HTTP surface `POST/GET /api/repos`, `DELETE /api/repos/{id}`, `POST /api/repos/{id}/refresh`, `GET /api/repos/{id}/sync/stream`. M3 also extends `api.NewServer` to receive the `Engine` and rewires `cmd/server/main.go`.

**Architecture:** Builds directly on M1 (`docs/superpowers/plans/2026-05-30-m1-skeleton-and-auth.md`) and M2 (`docs/superpowers/plans/2026-05-31-m2-collector-and-backfill.md`). The engine keeps spec §4 boundaries: `sync` **orchestrates** (queue, pool, scheduler, delta), delegating fetching to `githubapi` and persistence to `store`; `backfill.Run` is reused unchanged for `kind="backfill"` jobs. **Jobs are durable** rows in `sync_jobs` so an in-flight sync survives a restart; `LeaseNextJob` claims the oldest runnable job **atomically** (a single conditional `UPDATE` guarded by `locked_at`/`next_run_at`) so two workers never run the same job. Workers share **one** `githubapi.Budget`; on budget exhaustion the cursor is already persisted by `backfill`/delta, so the job is simply rescheduled at the bucket reset (`next_run_at`). **Determinism**: the clock is injected as `now func() time.Time`; the engine exposes a synchronous `processNextJob` that tests drive directly, plus a separate `Start/Stop` goroutine-lifecycle test — no wall-clock sleeps in tests. The HTTP layer mints a **per-user** `githubapi.Client` from the caller's decrypted `oauth` credential (M1 `GetCredential` + `crypto.Cipher`) to add and refresh repos.

**Tech Stack:** Go 1.25+, `github.com/go-chi/chi/v5` (already a dep, used for `chi.URLParam`), `modernc.org/sqlite` (driver `"sqlite"`, WAL, `db.SetMaxOpenConns(1)`), Go stdlib `net/http`/`encoding/json`/`context`/`sync`/`time`, `net/http/httptest` for fake GitHub REST + GraphQL servers in tests. No new third-party dependencies are required.

---

## File Structure

```
github-stats/
├── internal/
│   ├── store/
│   │   ├── migrations/0003_sync.sql          # sync_jobs + repo_tracking
│   │   ├── jobs.go                            # SyncJob + EnqueueJob/LeaseNextJob/CompleteJob/FailJob/ListJobsForRepo
│   │   ├── jobs_test.go
│   │   ├── tracking.go                        # TrackRepo/UntrackRepo/ListTrackedRepos/IsTracked
│   │   └── tracking_test.go
│   ├── githubapi/
│   │   ├── delta.go                           # FetchCommitsSince/FetchPullRequestsUpdated/FetchIssuesUpdated
│   │   └── delta_test.go
│   ├── sync/
│   │   ├── delta.go                           # RunDelta(ctx, store, client, repoID, now)
│   │   ├── delta_test.go
│   │   ├── broadcaster.go                     # Broadcaster: Subscribe/publish (SSE pub-sub)
│   │   ├── broadcaster_test.go
│   │   ├── engine.go                          # Engine: pool + scheduler + broadcaster; Start/Stop/Trigger*; processNextJob
│   │   └── engine_test.go
│   └── api/
│       ├── server.go                          # MODIFIED: NewServer gains (engine, cipher) params + repo routes
│       ├── server_test.go                     # MODIFIED: existing M1 test helper passes new args
│       ├── me_test.go                         # MODIFIED: testServer helper passes new args
│       ├── repos.go                           # POST/GET/DELETE /api/repos, refresh, per-user client builder
│       ├── repos_test.go
│       ├── stream.go                          # GET /api/repos/{id}/sync/stream (SSE)
│       └── stream_test.go
└── cmd/server/main.go                         # MODIFIED: construct + Start + inject + Stop the Engine
```

> All files under `internal/store/` join the **existing** `package store`; `internal/githubapi/delta.go` joins `package githubapi`; `internal/sync/` is `package sync` (a *local* package — code under it imports the stdlib as `"sync"` only where needed; to avoid the name clash, engine code that needs the stdlib mutex aliases it `stdsync "sync"`); `internal/api/` joins `package api`. The M1 `openTemp(t)` helper in `internal/store/store_test.go` is reused by every new store test.

---

## Task 1: 0003 migration — sync_jobs + repo_tracking

**Files:**
- Create: `internal/store/migrations/0003_sync.sql`

This migration is picked up automatically by the M1 migration runner (`//go:embed migrations/*.sql`, applied in sorted filename order, tracked in `schema_migrations`). Column names are stable — M4/M5 read `repo_tracking` and `sync_jobs`.

- [ ] **Step 1: Write the migration SQL**

`internal/store/migrations/0003_sync.sql`:
```sql
-- Durable job queue for the sync engine (spec §5/§6).
CREATE TABLE sync_jobs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id      INTEGER NOT NULL,
    kind         TEXT    NOT NULL,              -- 'backfill' | 'delta'
    status       TEXT    NOT NULL DEFAULT 'pending', -- 'pending' | 'running' | 'done' | 'error'
    cursor_state TEXT    NOT NULL DEFAULT '',   -- opaque JSON scratch (cursors live in sync_state)
    attempts     INTEGER NOT NULL DEFAULT 0,
    next_run_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    locked_at    TIMESTAMP,                     -- NULL when not leased
    last_error   TEXT    NOT NULL DEFAULT '',
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);
CREATE INDEX idx_sync_jobs_runnable ON sync_jobs(status, next_run_at);
CREATE INDEX idx_sync_jobs_repo ON sync_jobs(repo_id);

-- Per-user repo tracking (spec §5). A repo row is shared; tracking is per-user.
CREATE TABLE repo_tracking (
    user_id    INTEGER NOT NULL,
    repo_id    INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, repo_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);
CREATE INDEX idx_repo_tracking_user ON repo_tracking(user_id);
```

- [ ] **Step 2: Verify the migration applies (reuse M1/M2 store tests)**

Run: `go test ./internal/store/ -run TestOpenAppliesMigrations -v`
Expected: PASS — `Open()` now applies `0001`, `0002`, then `0003`; existing assertions still pass. (A dedicated table-existence assertion is added in Task 2's test.)

- [ ] **Step 3: Commit**

```bash
git add internal/store/migrations/0003_sync.sql
git commit -m "feat: 0003 migration for sync_jobs and repo_tracking"
```

---

## Task 2: Job-queue DAO

**Files:**
- Create: `internal/store/jobs.go`, `internal/store/jobs_test.go`

`LeaseNextJob` is the heart of the queue: it claims the oldest runnable `pending` job whose `next_run_at <= now` and that is not currently locked, flipping it to `running` and stamping `locked_at` — **in a single conditional `UPDATE`** so two concurrent leasers can never claim the same row. `FailJob` increments `attempts`, records the error, and either reschedules with a backoff (`next_run_at = now + backoff`, back to `pending`, clears `locked_at`) or, once `attempts >= maxAttempts`, marks the job `error`. The DAO takes `now`/`backoff`/`maxAttempts` as explicit arguments so tests are deterministic (no internal `time.Now()`).

- [ ] **Step 1: Write the failing test**

`internal/store/jobs_test.go`:
```go
package store

import (
	"context"
	"testing"
	"time"
)

func TestNewSyncTablesExist(t *testing.T) {
	s := openTemp(t)
	for _, table := range []string{"sync_jobs", "repo_tracking"} {
		var name string
		err := s.DB.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestEnqueueAndLeaseJob(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)
	now := ts("2026-05-01T12:00:00Z")

	id, err := s.EnqueueJob(ctx, repoID, "backfill", now)
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero job id")
	}

	job, err := s.LeaseNextJob(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if job == nil {
		t.Fatal("expected a leased job")
	}
	if job.ID != id || job.RepoID != repoID || job.Kind != "backfill" || job.Status != "running" {
		t.Fatalf("leased job = %+v", job)
	}
	if job.LockedAt == nil {
		t.Fatal("expected locked_at to be set on lease")
	}
}

func TestLeaseSkipsFutureAndLocked(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)
	now := ts("2026-05-01T12:00:00Z")

	// A job scheduled in the future is not runnable yet.
	future := now.Add(time.Hour)
	if _, err := s.EnqueueJobAt(ctx, repoID, "delta", future); err != nil {
		t.Fatal(err)
	}
	if job, err := s.LeaseNextJob(ctx, now); err != nil {
		t.Fatal(err)
	} else if job != nil {
		t.Fatalf("future job should not be leased, got %+v", job)
	}

	// Once now advances past next_run_at it becomes leasable.
	job, err := s.LeaseNextJob(ctx, future.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if job == nil {
		t.Fatal("expected job to be leasable after next_run_at")
	}

	// It is now running/locked: a second lease at the same instant returns nothing.
	if again, err := s.LeaseNextJob(ctx, future.Add(time.Second)); err != nil {
		t.Fatal(err)
	} else if again != nil {
		t.Fatalf("locked job leased twice: %+v", again)
	}
}

func TestLeaseIsAtomicAcrossLeasers(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)
	now := ts("2026-05-01T12:00:00Z")

	// Enqueue exactly one runnable job; two leases must split into one hit, one miss.
	if _, err := s.EnqueueJob(ctx, repoID, "delta", now); err != nil {
		t.Fatal(err)
	}
	a, err := s.LeaseNextJob(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.LeaseNextJob(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	got := 0
	if a != nil {
		got++
	}
	if b != nil {
		got++
	}
	if got != 1 {
		t.Fatalf("expected exactly one successful lease, got %d (a=%v b=%v)", got, a, b)
	}
}

func TestCompleteJob(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)
	now := ts("2026-05-01T12:00:00Z")

	id, _ := s.EnqueueJob(ctx, repoID, "delta", now)
	if _, err := s.LeaseNextJob(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJob(ctx, id, now); err != nil {
		t.Fatal(err)
	}

	var status string
	s.DB.QueryRowContext(ctx, `SELECT status FROM sync_jobs WHERE id=?`, id).Scan(&status)
	if status != "done" {
		t.Fatalf("status = %q, want done", status)
	}
	// A done job is not leasable.
	if job, err := s.LeaseNextJob(ctx, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	} else if job != nil {
		t.Fatalf("done job should not be leased: %+v", job)
	}
}

func TestFailJobReschedulesThenErrors(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)
	now := ts("2026-05-01T12:00:00Z")

	id, _ := s.EnqueueJob(ctx, repoID, "delta", now)
	if _, err := s.LeaseNextJob(ctx, now); err != nil {
		t.Fatal(err)
	}

	// First failure (attempts -> 1) with maxAttempts 2: reschedules to pending in the future.
	if err := s.FailJob(ctx, id, "boom", now, 5*time.Minute, 2); err != nil {
		t.Fatal(err)
	}
	var status string
	var attempts int
	var lastErr string
	s.DB.QueryRowContext(ctx, `SELECT status, attempts, last_error FROM sync_jobs WHERE id=?`, id).
		Scan(&status, &attempts, &lastErr)
	if status != "pending" || attempts != 1 || lastErr != "boom" {
		t.Fatalf("after fail 1: status=%q attempts=%d err=%q", status, attempts, lastErr)
	}
	// Not leasable before the backoff elapses, leasable after.
	if job, _ := s.LeaseNextJob(ctx, now.Add(time.Minute)); job != nil {
		t.Fatalf("job leasable before backoff elapsed: %+v", job)
	}
	job, err := s.LeaseNextJob(ctx, now.Add(6*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if job == nil {
		t.Fatal("expected job leasable after backoff")
	}

	// Second failure hits maxAttempts -> terminal 'error'.
	if err := s.FailJob(ctx, id, "again", now.Add(6*time.Minute), 5*time.Minute, 2); err != nil {
		t.Fatal(err)
	}
	s.DB.QueryRowContext(ctx, `SELECT status, attempts FROM sync_jobs WHERE id=?`, id).
		Scan(&status, &attempts)
	if status != "error" || attempts != 2 {
		t.Fatalf("after fail 2: status=%q attempts=%d, want error/2", status, attempts)
	}
}

func TestRescheduleJob(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)
	now := ts("2026-05-01T12:00:00Z")

	id, _ := s.EnqueueJob(ctx, repoID, "delta", now)
	if _, err := s.LeaseNextJob(ctx, now); err != nil {
		t.Fatal(err)
	}
	// Budget exhausted: reschedule (not a failure) at the bucket reset.
	reset := now.Add(30 * time.Minute)
	if err := s.RescheduleJob(ctx, id, reset); err != nil {
		t.Fatal(err)
	}
	var status string
	var attempts int
	s.DB.QueryRowContext(ctx, `SELECT status, attempts FROM sync_jobs WHERE id=?`, id).
		Scan(&status, &attempts)
	if status != "pending" || attempts != 0 {
		t.Fatalf("reschedule should not bump attempts: status=%q attempts=%d", status, attempts)
	}
	if job, _ := s.LeaseNextJob(ctx, now.Add(time.Minute)); job != nil {
		t.Fatalf("rescheduled job leasable before reset: %+v", job)
	}
	if job, _ := s.LeaseNextJob(ctx, reset.Add(time.Second)); job == nil {
		t.Fatal("expected leasable after reset")
	}
}

func TestListJobsForRepo(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)
	now := ts("2026-05-01T12:00:00Z")

	if _, err := s.EnqueueJob(ctx, repoID, "backfill", now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnqueueJob(ctx, repoID, "delta", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	jobs, err := s.ListJobsForRepo(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("ListJobsForRepo len = %d, want 2", len(jobs))
	}
	// Newest first.
	if jobs[0].Kind != "delta" || jobs[1].Kind != "backfill" {
		t.Fatalf("order = %q,%q", jobs[0].Kind, jobs[1].Kind)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestNewSyncTablesExist|TestEnqueue|TestLease|TestComplete|TestFailJob|TestReschedule|TestListJobs' -v`
Expected: FAIL — `undefined: (*Store).EnqueueJob`.

- [ ] **Step 3: Write minimal implementation**

`internal/store/jobs.go`:
```go
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
// stamping locked_at, then RETURNS the claimed row in the same statement. It
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
	if err == ErrNotFound { // no runnable job → UPDATE affected 0 rows → no RETURNING row
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run 'TestNewSyncTablesExist|TestEnqueue|TestLease|TestComplete|TestFailJob|TestReschedule|TestListJobs' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/jobs.go internal/store/jobs_test.go
git commit -m "feat: durable job-queue DAO with atomic lease"
```

---

## Task 3: Repo-tracking DAO

**Files:**
- Create: `internal/store/tracking.go`, `internal/store/tracking_test.go`

`repo_tracking` is per-user; `ListTrackedRepos(userID)` joins to `repos` and returns the full `Repo` rows so the API can render repo cards without a second query.

- [ ] **Step 1: Write the failing test**

`internal/store/tracking_test.go`:
```go
package store

import (
	"context"
	"testing"
)

func TestTrackUntrackRepo(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	uid := seedUser(t, s)
	repoID := seedRepo(t, s)

	if tracked, err := s.IsTracked(ctx, uid, repoID); err != nil || tracked {
		t.Fatalf("IsTracked before = %v err=%v, want false", tracked, err)
	}

	if err := s.TrackRepo(ctx, uid, repoID); err != nil {
		t.Fatal(err)
	}
	// Tracking twice is idempotent.
	if err := s.TrackRepo(ctx, uid, repoID); err != nil {
		t.Fatal(err)
	}

	if tracked, err := s.IsTracked(ctx, uid, repoID); err != nil || !tracked {
		t.Fatalf("IsTracked after track = %v err=%v, want true", tracked, err)
	}

	repos, err := s.ListTrackedRepos(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].ID != repoID {
		t.Fatalf("ListTrackedRepos = %+v", repos)
	}

	if err := s.UntrackRepo(ctx, uid, repoID); err != nil {
		t.Fatal(err)
	}
	if tracked, _ := s.IsTracked(ctx, uid, repoID); tracked {
		t.Fatal("expected untracked")
	}
	repos, _ = s.ListTrackedRepos(ctx, uid)
	if len(repos) != 0 {
		t.Fatalf("ListTrackedRepos after untrack = %+v", repos)
	}
}

func TestListTrackedReposIsPerUser(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	u1 := seedUser(t, s)
	u2, err := s.UpsertUser(ctx, &User{GitHubID: 2, Login: "second"})
	if err != nil {
		t.Fatal(err)
	}
	repoID := seedRepo(t, s)

	if err := s.TrackRepo(ctx, u1, repoID); err != nil {
		t.Fatal(err)
	}
	r1, _ := s.ListTrackedRepos(ctx, u1)
	r2, _ := s.ListTrackedRepos(ctx, u2)
	if len(r1) != 1 || len(r2) != 0 {
		t.Fatalf("per-user isolation broken: u1=%d u2=%d", len(r1), len(r2))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestTrackUntrack|TestListTrackedRepos' -v`
Expected: FAIL — `undefined: (*Store).TrackRepo`.

- [ ] **Step 3: Write minimal implementation**

`internal/store/tracking.go`:
```go
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
			r.description, r.stargazers, r.forks, r.created_at
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
			&r.Description, &r.Stargazers, &r.Forks, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.IsPrivate = priv != 0
		repos = append(repos, r)
	}
	return repos, rows.Err()
}
```

> Note: `IsTracked` uses `QueryRowContext(...).Scan`, which returns `sql.ErrNoRows` (the raw driver sentinel, not the store's wrapped `ErrNotFound`) on no match — that is the path we normalize to `(false, nil)`, matching how M1/M2 DAOs treat absent single-row lookups.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run 'TestTrackUntrack|TestListTrackedRepos' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/tracking.go internal/store/tracking_test.go
git commit -m "feat: per-user repo_tracking DAO"
```

---

## Task 4: GraphQL delta fetchers

**Files:**
- Create: `internal/githubapi/delta.go`, `internal/githubapi/delta_test.go`

These extend `package githubapi` and **reuse the existing page types** (`CommitPage`, `PRPage`, `IssuePage`) and decode/convert shapes from M2's `fetch.go`. `FetchCommitsSince` adds `since:$since` to `history(...)` (GitHub returns only commits at-or-after `since`). `FetchPullRequestsUpdated`/`FetchIssuesUpdated` order by `UPDATED_AT DESC` and also select `updatedAt`; the **caller** stops paging once it sees an item older than `last_synced_at` minus an overlap (the engine's delta routine in Task 6 does this). The helpers `parseTime`/`parseTimePtr`/`IsBot` and the `pageInfo` type from M2 are reused.

- [ ] **Step 1: Write the failing test**

`internal/githubapi/delta_test.go`:
```go
package githubapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchCommitsSincePassesSinceVar(t *testing.T) {
	var gotSince any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.Unmarshal(raw, &req)
		if !strings.Contains(req.Query, "since:") && !strings.Contains(req.Query, "$since") {
			t.Errorf("query missing since arg: %s", req.Query)
		}
		gotSince = req.Variables["since"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"repository":{"ref":{"target":{"history":{
			"pageInfo":{"endCursor":"DC1","hasNextPage":false},
			"nodes":[{"oid":"s1","additions":4,"deletions":1,
				"committedDate":"2026-05-10T08:00:00Z","messageHeadline":"delta",
				"author":{"user":{"login":"neo"}}}]
		}}}},"rateLimit":{"cost":1,"remaining":4999,"resetAt":"2026-06-01T13:00:00Z"}}}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	since := time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC)
	page, err := c.FetchCommitsSince(context.Background(), "octocat", "hello", "main", since, "")
	if err != nil {
		t.Fatal(err)
	}
	if gotSince != "2026-05-09T00:00:00Z" {
		t.Fatalf("since variable = %v, want RFC3339 UTC", gotSince)
	}
	if len(page.Commits) != 1 || page.Commits[0].SHA != "s1" || page.EndCursor != "DC1" {
		t.Fatalf("commits-since page = %+v", page)
	}
}

func TestFetchPullRequestsUpdatedOrdersByUpdatedAt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		if !strings.Contains(req.Query, "UPDATED_AT") || !strings.Contains(req.Query, "DESC") {
			t.Errorf("PR query not ordered by UPDATED_AT DESC: %s", req.Query)
		}
		if !strings.Contains(req.Query, "updatedAt") {
			t.Errorf("PR query missing updatedAt selection: %s", req.Query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"repository":{"pullRequests":{
			"pageInfo":{"endCursor":"DP1","hasNextPage":true},
			"nodes":[
				{"number":9,"state":"MERGED","title":"recent","createdAt":"2026-05-01T07:00:00Z",
				 "updatedAt":"2026-05-11T10:00:00Z","mergedAt":"2026-05-11T10:00:00Z","closedAt":"2026-05-11T10:00:00Z",
				 "additions":2,"deletions":1,"changedFiles":1,"author":{"login":"neo"},
				 "comments":{"totalCount":1},"reviews":{"nodes":[]}}
			]
		}},"rateLimit":{"cost":1,"remaining":4998,"resetAt":"2026-06-01T13:00:00Z"}}}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	page, err := c.FetchPullRequestsUpdated(context.Background(), "octocat", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.PRs) != 1 || !page.HasNextPage || page.EndCursor != "DP1" {
		t.Fatalf("PR updated page = %+v", page)
	}
	pr := page.PRs[0]
	if pr.PullRequest.Number != 9 || pr.PullRequest.State != "MERGED" || pr.UpdatedAt.IsZero() {
		t.Fatalf("PR[0] = %+v (UpdatedAt=%v)", pr, pr.UpdatedAt)
	}
	if !pr.UpdatedAt.Equal(time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("UpdatedAt = %v, want 2026-05-11T10:00:00Z", pr.UpdatedAt)
	}
}

func TestFetchIssuesUpdatedOrdersByUpdatedAt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		if !strings.Contains(req.Query, "UPDATED_AT") || !strings.Contains(req.Query, "DESC") {
			t.Errorf("issue query not ordered by UPDATED_AT DESC: %s", req.Query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"repository":{"issues":{
			"pageInfo":{"endCursor":"DI1","hasNextPage":false},
			"nodes":[
				{"number":3,"state":"CLOSED","title":"fixed","createdAt":"2026-05-02T06:00:00Z",
				 "updatedAt":"2026-05-12T09:00:00Z","closedAt":"2026-05-12T09:00:00Z",
				 "author":{"login":"trinity"},"comments":{"totalCount":2}}
			]
		}},"rateLimit":{"cost":1,"remaining":4997,"resetAt":"2026-06-01T13:00:00Z"}}}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	page, err := c.FetchIssuesUpdated(context.Background(), "octocat", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Issues) != 1 || page.HasNextPage {
		t.Fatalf("issue updated page = %+v", page)
	}
	is := page.Issues[0]
	if is.Issue.Number != 3 || is.Issue.State != "CLOSED" || is.UpdatedAt.IsZero() {
		t.Fatalf("issue[0] = %+v (UpdatedAt=%v)", is, is.UpdatedAt)
	}
}
```

> Design note: the delta fetchers need an `UpdatedAt time.Time` on each returned PR/issue so the caller can decide when to stop paging. To avoid touching M2's `store.PullRequest`/`store.Issue` (which have no `UpdatedAt` field/column), the delta fetchers return **wrapper** page types whose nodes carry the `store` event plus the extra `UpdatedAt`: `UpdatedPRPage{PRs []UpdatedPR}` with `UpdatedPR{PullRequest store.PullRequest; UpdatedAt time.Time}`, and `UpdatedIssuePage{Issues []UpdatedIssue}` with `UpdatedIssue{Issue store.Issue; UpdatedAt time.Time}`. That is why the assertions above read `page.PRs[0].PullRequest.Number` / `page.Issues[0].Issue.Number` and `.UpdatedAt`. (`FetchCommitsSince` reuses `CommitPage` unchanged — commits have no "updated" notion.) The test bodies above are complete and final; no further edits to them are needed.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/githubapi/ -run 'TestFetchCommitsSince|TestFetchPullRequestsUpdated|TestFetchIssuesUpdated' -v`
Expected: FAIL — `undefined: (*Client).FetchCommitsSince`.

- [ ] **Step 3: Write minimal implementation**

`internal/githubapi/delta.go`:
```go
package githubapi

import (
	"context"
	"time"

	"github-stats/internal/store"
)

// --- Commits since ---------------------------------------------------------

const commitsSinceQuery = `
query($owner:String!, $name:String!, $branch:String!, $since:GitTimestamp!, $after:String) {
  repository(owner:$owner, name:$name) {
    ref(qualifiedName:$branch) {
      target {
        ... on Commit {
          history(first:100, after:$after, since:$since) {
            pageInfo { endCursor hasNextPage }
            nodes {
              oid
              additions
              deletions
              committedDate
              messageHeadline
              author { user { login } }
            }
          }
        }
      }
    }
  }
  rateLimit { cost remaining resetAt }
}`

// FetchCommitsSince pages commit history restricted to commits at or after
// `since` (delta sync). It reuses CommitPage; commits have no "updated" notion.
func (c *Client) FetchCommitsSince(ctx context.Context, owner, name, branch string, since time.Time, after string) (*CommitPage, error) {
	var data struct {
		Repository struct {
			Ref struct {
				Target struct {
					History struct {
						PageInfo pageInfo `json:"pageInfo"`
						Nodes    []struct {
							OID             string `json:"oid"`
							Additions       int64  `json:"additions"`
							Deletions       int64  `json:"deletions"`
							CommittedDate   string `json:"committedDate"`
							MessageHeadline string `json:"messageHeadline"`
							Author          struct {
								User struct {
									Login string `json:"login"`
								} `json:"user"`
							} `json:"author"`
						} `json:"nodes"`
					} `json:"history"`
				} `json:"target"`
			} `json:"ref"`
		} `json:"repository"`
		RateLimit RateLimit `json:"rateLimit"`
	}
	vars := map[string]any{
		"owner":  owner,
		"name":   name,
		"branch": "refs/heads/" + branch,
		"since":  since.UTC().Format(time.RFC3339),
	}
	if after != "" {
		vars["after"] = after
	}
	if err := c.graphql(ctx, commitsSinceQuery, vars, &data); err != nil {
		return nil, err
	}
	h := data.Repository.Ref.Target.History
	page := &CommitPage{EndCursor: h.PageInfo.EndCursor, HasNextPage: h.PageInfo.HasNextPage}
	for _, n := range h.Nodes {
		login := n.Author.User.Login
		page.Commits = append(page.Commits, store.Commit{
			SHA:          n.OID,
			AuthorLogin:  login,
			CommittedAt:  parseTime(n.CommittedDate),
			Additions:    n.Additions,
			Deletions:    n.Deletions,
			IsBot:        IsBot(login),
			MsgFirstLine: n.MessageHeadline,
		})
	}
	return page, nil
}

// --- Pull requests updated -------------------------------------------------

// UpdatedPR pairs a store.PullRequest with its updatedAt so callers can stop
// paging once items predate the overlap window.
type UpdatedPR struct {
	PullRequest store.PullRequest
	UpdatedAt   time.Time
}

// UpdatedPRPage is one page of pull requests ordered by UPDATED_AT DESC.
type UpdatedPRPage struct {
	PRs         []UpdatedPR
	EndCursor   string
	HasNextPage bool
}

const pullRequestsUpdatedQuery = `
query($owner:String!, $name:String!, $after:String) {
  repository(owner:$owner, name:$name) {
    pullRequests(first:100, after:$after, orderBy:{field:UPDATED_AT, direction:DESC}) {
      pageInfo { endCursor hasNextPage }
      nodes {
        number
        state
        title
        createdAt
        updatedAt
        mergedAt
        closedAt
        additions
        deletions
        changedFiles
        author { login }
        comments { totalCount }
        reviews(first:1) { nodes { submittedAt } }
      }
    }
  }
  rateLimit { cost remaining resetAt }
}`

// FetchPullRequestsUpdated pages PRs newest-updated first. The caller stops once
// UpdatedAt falls before its overlap-adjusted cutoff.
func (c *Client) FetchPullRequestsUpdated(ctx context.Context, owner, name, after string) (*UpdatedPRPage, error) {
	var data struct {
		Repository struct {
			PullRequests struct {
				PageInfo pageInfo `json:"pageInfo"`
				Nodes    []struct {
					Number       int64  `json:"number"`
					State        string `json:"state"`
					Title        string `json:"title"`
					CreatedAt    string `json:"createdAt"`
					UpdatedAt    string `json:"updatedAt"`
					MergedAt     string `json:"mergedAt"`
					ClosedAt     string `json:"closedAt"`
					Additions    int64  `json:"additions"`
					Deletions    int64  `json:"deletions"`
					ChangedFiles int64  `json:"changedFiles"`
					Author       struct {
						Login string `json:"login"`
					} `json:"author"`
					Comments struct {
						TotalCount int64 `json:"totalCount"`
					} `json:"comments"`
					Reviews struct {
						Nodes []struct {
							SubmittedAt string `json:"submittedAt"`
						} `json:"nodes"`
					} `json:"reviews"`
				} `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
		RateLimit RateLimit `json:"rateLimit"`
	}
	vars := map[string]any{"owner": owner, "name": name}
	if after != "" {
		vars["after"] = after
	}
	if err := c.graphql(ctx, pullRequestsUpdatedQuery, vars, &data); err != nil {
		return nil, err
	}
	prs := data.Repository.PullRequests
	page := &UpdatedPRPage{EndCursor: prs.PageInfo.EndCursor, HasNextPage: prs.PageInfo.HasNextPage}
	for _, n := range prs.Nodes {
		var firstReview *time.Time
		if len(n.Reviews.Nodes) > 0 {
			firstReview = parseTimePtr(n.Reviews.Nodes[0].SubmittedAt)
		}
		login := n.Author.Login
		page.PRs = append(page.PRs, UpdatedPR{
			PullRequest: store.PullRequest{
				Number:        n.Number,
				AuthorLogin:   login,
				State:         n.State,
				CreatedAt:     parseTime(n.CreatedAt),
				MergedAt:      parseTimePtr(n.MergedAt),
				ClosedAt:      parseTimePtr(n.ClosedAt),
				Additions:     n.Additions,
				Deletions:     n.Deletions,
				ChangedFiles:  n.ChangedFiles,
				CommentsCount: n.Comments.TotalCount,
				FirstReviewAt: firstReview,
				IsBot:         IsBot(login),
				Title:         n.Title,
			},
			UpdatedAt: parseTime(n.UpdatedAt),
		})
	}
	return page, nil
}

// --- Issues updated --------------------------------------------------------

// UpdatedIssue pairs a store.Issue with its updatedAt.
type UpdatedIssue struct {
	Issue     store.Issue
	UpdatedAt time.Time
}

// UpdatedIssuePage is one page of issues ordered by UPDATED_AT DESC.
type UpdatedIssuePage struct {
	Issues      []UpdatedIssue
	EndCursor   string
	HasNextPage bool
}

const issuesUpdatedQuery = `
query($owner:String!, $name:String!, $after:String) {
  repository(owner:$owner, name:$name) {
    issues(first:100, after:$after, orderBy:{field:UPDATED_AT, direction:DESC}) {
      pageInfo { endCursor hasNextPage }
      nodes {
        number
        state
        title
        createdAt
        updatedAt
        closedAt
        author { login }
        comments { totalCount }
      }
    }
  }
  rateLimit { cost remaining resetAt }
}`

// FetchIssuesUpdated pages issues newest-updated first.
func (c *Client) FetchIssuesUpdated(ctx context.Context, owner, name, after string) (*UpdatedIssuePage, error) {
	var data struct {
		Repository struct {
			Issues struct {
				PageInfo pageInfo `json:"pageInfo"`
				Nodes    []struct {
					Number    int64  `json:"number"`
					State     string `json:"state"`
					Title     string `json:"title"`
					CreatedAt string `json:"createdAt"`
					UpdatedAt string `json:"updatedAt"`
					ClosedAt  string `json:"closedAt"`
					Author    struct {
						Login string `json:"login"`
					} `json:"author"`
					Comments struct {
						TotalCount int64 `json:"totalCount"`
					} `json:"comments"`
				} `json:"nodes"`
			} `json:"issues"`
		} `json:"repository"`
		RateLimit RateLimit `json:"rateLimit"`
	}
	vars := map[string]any{"owner": owner, "name": name}
	if after != "" {
		vars["after"] = after
	}
	if err := c.graphql(ctx, issuesUpdatedQuery, vars, &data); err != nil {
		return nil, err
	}
	iss := data.Repository.Issues
	page := &UpdatedIssuePage{EndCursor: iss.PageInfo.EndCursor, HasNextPage: iss.PageInfo.HasNextPage}
	for _, n := range iss.Nodes {
		login := n.Author.Login
		page.Issues = append(page.Issues, UpdatedIssue{
			Issue: store.Issue{
				Number:        n.Number,
				AuthorLogin:   login,
				State:         n.State,
				CreatedAt:     parseTime(n.CreatedAt),
				ClosedAt:      parseTimePtr(n.ClosedAt),
				CommentsCount: n.Comments.TotalCount,
				IsBot:         IsBot(login),
				Title:         n.Title,
			},
			UpdatedAt: parseTime(n.UpdatedAt),
		})
	}
	return page, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/githubapi/ -run 'TestFetchCommitsSince|TestFetchPullRequestsUpdated|TestFetchIssuesUpdated' -v`
Expected: PASS.

- [ ] **Step 5: Run the whole githubapi package**

Run: `go test ./internal/githubapi/ -v`
Expected: PASS — M2 (bot, budget, etag, graphql, fetch) plus the new delta fetchers.

- [ ] **Step 6: Commit**

```bash
git add internal/githubapi/delta.go internal/githubapi/delta_test.go
git commit -m "feat: graphql delta fetchers (commits-since, PRs/issues updated)"
```

---

## Task 5: Progress broadcaster (SSE pub-sub)

**Files:**
- Create: `internal/sync/broadcaster.go`, `internal/sync/broadcaster_test.go`

A `Broadcaster` is a per-repo fan-out: workers `publish(repoID, Event)` and the SSE handler `Subscribe(repoID)`s for a buffered channel plus a `cancel` to unsubscribe. Publishing is **non-blocking** (a full subscriber buffer drops the oldest-style event rather than stalling a worker). This is the first file in `package sync`; it aliases the stdlib mutex as `stdsync` to avoid the package-name clash.

- [ ] **Step 1: Write the failing test**

`internal/sync/broadcaster_test.go`:
```go
package sync

import (
	"testing"
)

func TestBroadcasterDeliversToSubscriber(t *testing.T) {
	b := NewBroadcaster()
	ch, cancel := b.Subscribe(7)
	defer cancel()

	b.publish(7, Event{RepoID: 7, Phase: "commits", Message: "page 1", Done: false})

	select {
	case ev := <-ch:
		if ev.RepoID != 7 || ev.Phase != "commits" || ev.Message != "page 1" {
			t.Fatalf("event = %+v", ev)
		}
	default:
		t.Fatal("expected an event to be delivered")
	}
}

func TestBroadcasterIsolatesRepos(t *testing.T) {
	b := NewBroadcaster()
	ch7, cancel7 := b.Subscribe(7)
	defer cancel7()
	ch8, cancel8 := b.Subscribe(8)
	defer cancel8()

	b.publish(8, Event{RepoID: 8, Phase: "done", Done: true})

	select {
	case <-ch7:
		t.Fatal("repo 7 subscriber should not see repo 8 events")
	default:
	}
	select {
	case ev := <-ch8:
		if !ev.Done {
			t.Fatalf("expected done event, got %+v", ev)
		}
	default:
		t.Fatal("repo 8 subscriber missed its event")
	}
}

func TestBroadcasterCancelUnsubscribes(t *testing.T) {
	b := NewBroadcaster()
	ch, cancel := b.Subscribe(7)
	cancel()
	// Publishing after cancel must not panic (closed/removed channel) and the
	// channel is closed so a receive returns the zero value with ok=false.
	b.publish(7, Event{RepoID: 7, Phase: "commits"})
	if _, ok := <-ch; ok {
		t.Fatal("expected channel closed after cancel")
	}
}

func TestBroadcasterDoesNotBlockWhenBufferFull(t *testing.T) {
	b := NewBroadcaster()
	_, cancel := b.Subscribe(7)
	defer cancel()
	// Flood far beyond the buffer; publish must never block.
	for i := 0; i < 10000; i++ {
		b.publish(7, Event{RepoID: 7, Phase: "commits", Message: "x"})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/sync/ -run TestBroadcaster -v`
Expected: FAIL — `undefined: NewBroadcaster`.

- [ ] **Step 3: Write minimal implementation**

`internal/sync/broadcaster.go`:
```go
// Package sync is the github-stats sync engine: a durable job queue worker
// pool, a periodic delta scheduler, the delta-sync routine, and a per-repo
// progress broadcaster for SSE. It orchestrates only — fetching is delegated to
// githubapi and persistence to store (spec §4/§6). The stdlib sync package is
// imported under the alias stdsync to avoid the package-name clash.
package sync

import stdsync "sync"

// Event is one progress update for a repo's sync.
type Event struct {
	RepoID  int64  `json:"repo_id"`
	Phase   string `json:"phase"`   // "backfill" | "delta" | "commits" | "prs" | "issues" | "releases" | "done" | "error"
	Message string `json:"message"` // human-readable detail
	Done    bool   `json:"done"`    // terminal event for this run
}

const subscriberBuffer = 32

// Broadcaster fans progress events out to per-repo subscribers.
type Broadcaster struct {
	mu   stdsync.Mutex
	subs map[int64]map[chan Event]struct{}
}

// NewBroadcaster builds an empty Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: make(map[int64]map[chan Event]struct{})}
}

// Subscribe returns a buffered channel of events for repoID plus a cancel func
// that unsubscribes and closes the channel. Cancel is idempotent.
func (b *Broadcaster) Subscribe(repoID int64) (<-chan Event, func()) {
	ch := make(chan Event, subscriberBuffer)
	b.mu.Lock()
	if b.subs[repoID] == nil {
		b.subs[repoID] = make(map[chan Event]struct{})
	}
	b.subs[repoID][ch] = struct{}{}
	b.mu.Unlock()

	var once stdsync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			if set, ok := b.subs[repoID]; ok {
				if _, ok := set[ch]; ok {
					delete(set, ch)
					close(ch)
				}
				if len(set) == 0 {
					delete(b.subs, repoID)
				}
			}
			b.mu.Unlock()
		})
	}
	return ch, cancel
}

// publish delivers ev to every subscriber of ev.RepoID. It never blocks: if a
// subscriber's buffer is full the event is dropped for that subscriber.
func (b *Broadcaster) publish(repoID int64, ev Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs[repoID] {
		select {
		case ch <- ev:
		default: // subscriber lagging; drop rather than stall the worker
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/sync/ -run TestBroadcaster -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sync/broadcaster.go internal/sync/broadcaster_test.go
git commit -m "feat: per-repo SSE progress broadcaster"
```

---

## Task 6: Delta sync routine

**Files:**
- Create: `internal/sync/delta.go`, `internal/sync/delta_test.go`

`RunDelta(ctx, st, client, repoID, now)` pulls commits since `sync_state.LastCommitAt`, and PRs/issues updated since the same cutoff **minus an overlap window** (`overlapWindow`, catches late edits). It upserts events, tracks the touched date span, calls `RecomputeDailyStats` over that span, then advances `sync_state.LastCommitAt` to the newest commit seen. PR/issue paging **stops** as soon as a page's items predate `cutoff = LastCommitAt - overlap` (because `UPDATED_AT DESC`). A fresh repo (no `LastCommitAt`) uses a wide cutoff so the first delta is effectively a small catch-up; full history is the backfill job's responsibility.

- [ ] **Step 1: Write the failing test**

`internal/sync/delta_test.go`:
```go
package sync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github-stats/internal/githubapi"
	"github-stats/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func ptime(s string) time.Time {
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return v
}

// fakeDeltaGraphQL serves the three delta queries with one page each.
func fakeDeltaGraphQL(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		w.Header().Set("Content-Type", "application/json")
		const rl = `"rateLimit":{"cost":1,"remaining":4990,"resetAt":"2026-06-01T13:00:00Z"}`
		switch {
		case strings.Contains(req.Query, "since:"):
			w.Write([]byte(`{"data":{"repository":{"ref":{"target":{"history":{
				"pageInfo":{"endCursor":"DC1","hasNextPage":false},
				"nodes":[{"oid":"d1","additions":7,"deletions":1,
					"committedDate":"2026-05-20T08:00:00Z","messageHeadline":"delta commit",
					"author":{"user":{"login":"neo"}}}]}}}}},` + rl + `}}`))
		case strings.Contains(req.Query, "pullRequests"):
			w.Write([]byte(`{"data":{"repository":{"pullRequests":{
				"pageInfo":{"endCursor":"DP1","hasNextPage":false},
				"nodes":[{"number":5,"state":"MERGED","title":"recent","createdAt":"2026-05-19T07:00:00Z",
					"updatedAt":"2026-05-20T10:00:00Z","mergedAt":"2026-05-20T10:00:00Z","closedAt":"2026-05-20T10:00:00Z",
					"additions":3,"deletions":1,"changedFiles":1,"author":{"login":"neo"},
					"comments":{"totalCount":2},"reviews":{"nodes":[]}}]
			}},` + rl + `}}`))
		case strings.Contains(req.Query, "issues"):
			w.Write([]byte(`{"data":{"repository":{"issues":{
				"pageInfo":{"endCursor":"DI1","hasNextPage":false},
				"nodes":[{"number":2,"state":"CLOSED","title":"fixed","createdAt":"2026-05-18T06:00:00Z",
					"updatedAt":"2026-05-20T09:00:00Z","closedAt":"2026-05-20T09:00:00Z",
					"author":{"login":"trinity"},"comments":{"totalCount":1}}]
			}},` + rl + `}}`))
		default:
			t.Errorf("unexpected delta query: %s", req.Query)
			w.WriteHeader(500)
		}
	}
}

func TestRunDeltaIngestsAndRecomputes(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	srv := httptest.NewServer(fakeDeltaGraphQL(t))
	defer srv.Close()
	client := githubapi.NewClient(githubapi.Options{
		Token: "gho_test", GraphQLURL: srv.URL, RESTBaseURL: srv.URL, Store: st, HTTP: &http.Client{},
	})

	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 1, FullName: "octocat/hello", DefaultBranch: "main"})
	// Seed prior sync state so the delta has a cutoff.
	last := ptime("2026-05-15T00:00:00Z")
	if err := st.UpsertSyncState(ctx, &store.SyncState{RepoID: repoID, LastCommitAt: &last, Status: "complete"}); err != nil {
		t.Fatal(err)
	}

	now := ptime("2026-05-21T00:00:00Z")
	if err := RunDelta(ctx, st, client, repoID, func() time.Time { return now }); err != nil {
		t.Fatalf("RunDelta: %v", err)
	}

	// Events ingested.
	var c, p, i int
	st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM commits WHERE repo_id=?`, repoID).Scan(&c)
	st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pull_requests WHERE repo_id=?`, repoID).Scan(&p)
	st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM issues WHERE repo_id=?`, repoID).Scan(&i)
	if c != 1 || p != 1 || i != 1 {
		t.Fatalf("ingested commits=%d prs=%d issues=%d, want 1/1/1", c, p, i)
	}

	// Aggregates recomputed for the touched dates.
	var commitsDay int
	st.DB.QueryRowContext(ctx,
		`SELECT commits FROM daily_repo_stats WHERE repo_id=? AND date='2026-05-20'`, repoID).Scan(&commitsDay)
	if commitsDay != 1 {
		t.Fatalf("2026-05-20 commits aggregate = %d, want 1", commitsDay)
	}

	// LastCommitAt advanced to newest commit.
	ss, _ := st.GetSyncState(ctx, repoID)
	if ss.LastCommitAt == nil || !ss.LastCommitAt.Equal(ptime("2026-05-20T08:00:00Z")) {
		t.Fatalf("LastCommitAt = %v, want 2026-05-20T08:00:00Z", ss.LastCommitAt)
	}
}

func TestRunDeltaStopsAtOverlapCutoff(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	// PR page returns one recent PR (newer than cutoff) and one stale PR (older
	// than cutoff-overlap). The stale one must NOT be ingested because paging
	// stops at the first item past the cutoff.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		w.Header().Set("Content-Type", "application/json")
		const rl = `"rateLimit":{"cost":1,"remaining":4990,"resetAt":"2026-06-01T13:00:00Z"}`
		switch {
		case strings.Contains(req.Query, "since:"):
			w.Write([]byte(`{"data":{"repository":{"ref":{"target":{"history":{
				"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[]}}}}},` + rl + `}}`))
		case strings.Contains(req.Query, "pullRequests"):
			w.Write([]byte(`{"data":{"repository":{"pullRequests":{
				"pageInfo":{"endCursor":"P","hasNextPage":true},
				"nodes":[
					{"number":9,"state":"OPEN","title":"recent","createdAt":"2026-05-19T07:00:00Z",
					 "updatedAt":"2026-05-20T10:00:00Z","mergedAt":null,"closedAt":null,
					 "additions":1,"deletions":0,"changedFiles":1,"author":{"login":"neo"},
					 "comments":{"totalCount":0},"reviews":{"nodes":[]}},
					{"number":1,"state":"MERGED","title":"ancient","createdAt":"2020-01-01T07:00:00Z",
					 "updatedAt":"2020-01-01T07:00:00Z","mergedAt":"2020-01-01T07:00:00Z","closedAt":"2020-01-01T07:00:00Z",
					 "additions":1,"deletions":0,"changedFiles":1,"author":{"login":"neo"},
					 "comments":{"totalCount":0},"reviews":{"nodes":[]}}
				]
			}},` + rl + `}}`))
		case strings.Contains(req.Query, "issues"):
			w.Write([]byte(`{"data":{"repository":{"issues":{
				"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[]}},` + rl + `}}`))
		default:
			t.Errorf("unexpected query: %s", req.Query)
		}
	}))
	defer srv.Close()
	client := githubapi.NewClient(githubapi.Options{
		Token: "gho_test", GraphQLURL: srv.URL, RESTBaseURL: srv.URL, Store: st, HTTP: &http.Client{},
	})

	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 1, FullName: "octocat/hello", DefaultBranch: "main"})
	last := ptime("2026-05-18T00:00:00Z")
	st.UpsertSyncState(ctx, &store.SyncState{RepoID: repoID, LastCommitAt: &last, Status: "complete"})

	now := ptime("2026-05-21T00:00:00Z")
	if err := RunDelta(ctx, st, client, repoID, func() time.Time { return now }); err != nil {
		t.Fatal(err)
	}

	// Only the recent PR (#9) ingested; the ancient PR (#1) is past the cutoff.
	var n int
	st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pull_requests WHERE repo_id=?`, repoID).Scan(&n)
	if n != 1 {
		t.Fatalf("PR count = %d, want 1 (stale PR must be skipped at cutoff)", n)
	}
	var exists int
	st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pull_requests WHERE repo_id=? AND number=9`, repoID).Scan(&exists)
	if exists != 1 {
		t.Fatal("recent PR #9 should be ingested")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/sync/ -run TestRunDelta -v`
Expected: FAIL — `undefined: RunDelta`.

- [ ] **Step 3: Write minimal implementation**

`internal/sync/delta.go`:
```go
package sync

import (
	"context"
	"strings"
	"time"

	"github-stats/internal/githubapi"
	"github-stats/internal/store"
)

// overlapWindow is subtracted from the last-sync cutoff so edits to items that
// changed shortly before the previous sync are re-pulled (spec §6).
const overlapWindow = 24 * time.Hour

// freshLookback bounds the first delta of a repo that has no recorded
// LastCommitAt yet (full history is the backfill job's responsibility).
const freshLookback = 14 * 24 * time.Hour

// RunDelta performs an incremental sync of repoID: commits since the last
// recorded commit time, and PRs/issues updated since that time minus an overlap
// window. It upserts events, recomputes daily aggregates for the touched span,
// and advances sync_state. `now` is injected for deterministic tests.
func RunDelta(ctx context.Context, st *store.Store, client *githubapi.Client, repoID int64, now func() time.Time) error {
	repo, err := st.GetRepo(ctx, repoID)
	if err != nil {
		return err
	}
	owner, name := splitFullName(repo.FullName)

	ss, err := st.GetSyncState(ctx, repoID)
	if err != nil {
		return err
	}

	var since time.Time
	if ss.LastCommitAt != nil {
		since = *ss.LastCommitAt
	} else {
		since = now().Add(-freshLookback)
	}
	cutoff := since.Add(-overlapWindow)

	span := &dateSpan{}
	newest := since

	// --- Commits since ---
	after := ""
	for {
		page, err := client.FetchCommitsSince(ctx, owner, name, repo.DefaultBranch, since, after)
		if err != nil {
			return err
		}
		if err := st.UpsertCommits(ctx, repoID, page.Commits); err != nil {
			return err
		}
		for _, c := range page.Commits {
			span.add(c.CommittedAt)
			if c.CommittedAt.After(newest) {
				newest = c.CommittedAt
			}
		}
		if !page.HasNextPage {
			break
		}
		after = page.EndCursor
	}

	// --- Pull requests updated (stop at cutoff) ---
	after = ""
prLoop:
	for {
		page, err := client.FetchPullRequestsUpdated(ctx, owner, name, after)
		if err != nil {
			return err
		}
		var batch []store.PullRequest
		for _, up := range page.PRs {
			if up.UpdatedAt.Before(cutoff) {
				if len(batch) > 0 {
					if err := st.UpsertPullRequests(ctx, repoID, batch); err != nil {
						return err
					}
					addPRSpan(span, batch)
				}
				break prLoop
			}
			batch = append(batch, up.PullRequest)
		}
		if err := st.UpsertPullRequests(ctx, repoID, batch); err != nil {
			return err
		}
		addPRSpan(span, batch)
		if !page.HasNextPage {
			break
		}
		after = page.EndCursor
	}

	// --- Issues updated (stop at cutoff) ---
	after = ""
issueLoop:
	for {
		page, err := client.FetchIssuesUpdated(ctx, owner, name, after)
		if err != nil {
			return err
		}
		var batch []store.Issue
		for _, ui := range page.Issues {
			if ui.UpdatedAt.Before(cutoff) {
				if len(batch) > 0 {
					if err := st.UpsertIssues(ctx, repoID, batch); err != nil {
						return err
					}
					addIssueSpan(span, batch)
				}
				break issueLoop
			}
			batch = append(batch, ui.Issue)
		}
		if err := st.UpsertIssues(ctx, repoID, batch); err != nil {
			return err
		}
		addIssueSpan(span, batch)
		if !page.HasNextPage {
			break
		}
		after = page.EndCursor
	}

	// Recompute aggregates over the touched span.
	if !span.min.IsZero() {
		from := span.min.UTC().Format("2006-01-02")
		to := span.max.UTC().Format("2006-01-02")
		if err := st.RecomputeDailyStats(ctx, repoID, from, to); err != nil {
			return err
		}
	}

	// Advance sync state.
	ss.LastCommitAt = &newest
	ss.Status = "complete"
	return st.UpsertSyncState(ctx, ss)
}

func addPRSpan(span *dateSpan, prs []store.PullRequest) {
	for _, p := range prs {
		span.add(p.CreatedAt)
		if p.MergedAt != nil {
			span.add(*p.MergedAt)
		}
		if p.ClosedAt != nil {
			span.add(*p.ClosedAt)
		}
	}
}

func addIssueSpan(span *dateSpan, issues []store.Issue) {
	for _, is := range issues {
		span.add(is.CreatedAt)
		if is.ClosedAt != nil {
			span.add(*is.ClosedAt)
		}
	}
}

// dateSpan tracks the min/max event dates touched during a delta so the
// aggregate recompute covers exactly the affected range.
type dateSpan struct {
	min, max time.Time
}

func (d *dateSpan) add(t time.Time) {
	if t.IsZero() {
		return
	}
	if d.min.IsZero() || t.Before(d.min) {
		d.min = t
	}
	if d.max.IsZero() || t.After(d.max) {
		d.max = t
	}
}

// splitFullName splits "owner/name" into its parts.
func splitFullName(fullName string) (owner, name string) {
	if i := strings.IndexByte(fullName, '/'); i >= 0 {
		return fullName[:i], fullName[i+1:]
	}
	return fullName, ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/sync/ -run TestRunDelta -v`
Expected: PASS (both delta tests).

- [ ] **Step 5: Commit**

```bash
git add internal/sync/delta.go internal/sync/delta_test.go
git commit -m "feat: delta sync routine with overlap cutoff and recompute"
```

---

## Task 7: Engine — worker pool, scheduler, processNextJob

**Files:**
- Create: `internal/sync/engine.go`, `internal/sync/engine_test.go`

`Engine` owns the worker pool, the scheduler ticker, and the broadcaster. It is built with `Config` (injectable `Now func() time.Time`, `Concurrency`, `SchedulerInterval`, `DeltaCadence`, `MaxAttempts`, `FailBackoff`) and a **client factory** `NewClient func(repoID int64) (*githubapi.Client, error)` so workers can mint a per-repo/per-user client (the API wires a factory that decrypts the owner's token). The synchronous `processNextJob(ctx)` leases one job, runs `backfill.Run` or `RunDelta`, emits progress, and completes/fails/reschedules it — tests call it directly. `Start(ctx)` launches `Concurrency` worker goroutines (each loops `processNextJob` then waits a short idle) plus the scheduler goroutine; `Stop()` cancels and waits. `TriggerBackfill`/`TriggerDelta` enqueue jobs immediately. `enqueueDueDeltas` is the scheduler's body, also directly testable.

- [ ] **Step 1: Write the failing test**

`internal/sync/engine_test.go`:
```go
package sync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github-stats/internal/githubapi"
	"github-stats/internal/store"
)

// fakeBackfillGraphQL answers a full backfill (meta + one page each) so a
// 'backfill' job can run to completion in-process.
func fakeBackfillGraphQL(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		w.Header().Set("Content-Type", "application/json")
		const rl = `"rateLimit":{"cost":1,"remaining":4990,"resetAt":"2026-06-01T13:00:00Z"}`
		switch {
		case strings.Contains(req.Query, "databaseId"):
			w.Write([]byte(`{"data":{"repository":{"databaseId":1,"nameWithOwner":"octocat/hello",
				"isPrivate":false,"description":"hi","stargazerCount":1,"forkCount":0,
				"defaultBranchRef":{"name":"main"}},` + rl + `}}`))
		case strings.Contains(req.Query, "history"):
			w.Write([]byte(`{"data":{"repository":{"ref":{"target":{"history":{
				"pageInfo":{"endCursor":"C1","hasNextPage":false},
				"nodes":[{"oid":"sha1","additions":1,"deletions":0,
					"committedDate":"2026-05-01T08:00:00Z","messageHeadline":"x",
					"author":{"user":{"login":"neo"}}}]}}}}},` + rl + `}}`))
		case strings.Contains(req.Query, "pullRequests"):
			w.Write([]byte(`{"data":{"repository":{"pullRequests":{"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[]}},` + rl + `}}`))
		case strings.Contains(req.Query, "issues"):
			w.Write([]byte(`{"data":{"repository":{"issues":{"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[]}},` + rl + `}}`))
		case strings.Contains(req.Query, "releases"):
			w.Write([]byte(`{"data":{"repository":{"releases":{"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[]}},` + rl + `}}`))
		default:
			t.Errorf("unexpected query: %s", req.Query)
		}
	}
}

func newEngine(t *testing.T, st *store.Store, srvURL string, now func() time.Time) *Engine {
	t.Helper()
	factory := func(repoID int64) (*githubapi.Client, error) {
		return githubapi.NewClient(githubapi.Options{
			Token: "gho_test", GraphQLURL: srvURL, RESTBaseURL: srvURL, Store: st, HTTP: &http.Client{},
		}), nil
	}
	return NewEngine(st, factory, Config{
		Now:               now,
		Concurrency:       2,
		SchedulerInterval: time.Minute,
		DeltaCadence:      30 * time.Minute,
		MaxAttempts:       3,
		FailBackoff:       5 * time.Minute,
	})
}

func TestProcessNextJobRunsBackfill(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	srv := httptest.NewServer(fakeBackfillGraphQL(t))
	defer srv.Close()

	now := ptime("2026-05-21T00:00:00Z")
	eng := newEngine(t, st, srv.URL, func() time.Time { return now })

	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 1, FullName: "octocat/hello", DefaultBranch: "main"})
	jobID, err := eng.TriggerBackfill(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}

	ran, err := eng.processNextJob(ctx)
	if err != nil {
		t.Fatalf("processNextJob: %v", err)
	}
	if !ran {
		t.Fatal("expected a job to be processed")
	}

	// Job marked done.
	jobs, _ := st.ListJobsForRepo(ctx, repoID)
	if len(jobs) != 1 || jobs[0].ID != jobID || jobs[0].Status != "done" {
		t.Fatalf("job not done: %+v", jobs)
	}
	// Backfill wrote a commit.
	var n int
	st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM commits WHERE repo_id=?`, repoID).Scan(&n)
	if n != 1 {
		t.Fatalf("commits = %d, want 1", n)
	}
}

func TestProcessNextJobNoWork(t *testing.T) {
	st := openTestStore(t)
	eng := newEngine(t, st, "http://unused", func() time.Time { return ptime("2026-05-21T00:00:00Z") })
	ran, err := eng.processNextJob(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Fatal("expected no job to process on an empty queue")
	}
}

func TestProcessNextJobFailIsRecorded(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	// A server that errors every GraphQL query forces the backfill to fail.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":null,"errors":[{"message":"boom"}]}`))
	}))
	defer srv.Close()

	now := ptime("2026-05-21T00:00:00Z")
	eng := newEngine(t, st, srv.URL, func() time.Time { return now })
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 1, FullName: "octocat/hello", DefaultBranch: "main"})
	if _, err := eng.TriggerBackfill(ctx, repoID); err != nil {
		t.Fatal(err)
	}

	ran, err := eng.processNextJob(ctx)
	if err != nil {
		t.Fatalf("processNextJob should not surface job errors: %v", err)
	}
	if !ran {
		t.Fatal("expected the job to be processed (and fail)")
	}
	jobs, _ := st.ListJobsForRepo(ctx, repoID)
	if len(jobs) != 1 || jobs[0].Attempts != 1 || jobs[0].LastError == "" {
		t.Fatalf("failed job not recorded: %+v", jobs)
	}
	// With MaxAttempts 3 the first failure reschedules to pending.
	if jobs[0].Status != "pending" {
		t.Fatalf("status = %q, want pending after first failure", jobs[0].Status)
	}
}

func TestEnqueueDueDeltasRespectsCadence(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	uid := mustUser(t, st)
	now := ptime("2026-05-21T12:00:00Z")
	eng := newEngine(t, st, "http://unused", func() time.Time { return now })

	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 1, FullName: "octocat/hello", DefaultBranch: "main"})
	if err := st.TrackRepo(ctx, uid, repoID); err != nil {
		t.Fatal(err)
	}

	// Stale repo: last backfill long ago → a delta job is enqueued.
	old := now.Add(-2 * time.Hour)
	st.UpsertSyncState(ctx, &store.SyncState{RepoID: repoID, LastBackfillAt: &old, Status: "complete"})

	if err := eng.enqueueDueDeltas(ctx); err != nil {
		t.Fatal(err)
	}
	jobs, _ := st.ListJobsForRepo(ctx, repoID)
	deltas := 0
	for _, j := range jobs {
		if j.Kind == "delta" {
			deltas++
		}
	}
	if deltas != 1 {
		t.Fatalf("expected 1 delta enqueued, got %d", deltas)
	}

	// Running again immediately must NOT enqueue a second (cadence not elapsed,
	// and a pending delta already exists).
	if err := eng.enqueueDueDeltas(ctx); err != nil {
		t.Fatal(err)
	}
	jobs, _ = st.ListJobsForRepo(ctx, repoID)
	deltas = 0
	for _, j := range jobs {
		if j.Kind == "delta" {
			deltas++
		}
	}
	if deltas != 1 {
		t.Fatalf("cadence guard failed: %d delta jobs", deltas)
	}
}

func TestStartStopLifecycleDrainsQueue(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	srv := httptest.NewServer(fakeBackfillGraphQL(t))
	defer srv.Close()

	now := ptime("2026-05-21T00:00:00Z")
	eng := newEngine(t, st, srv.URL, func() time.Time { return now })
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 1, FullName: "octocat/hello", DefaultBranch: "main"})
	if _, err := eng.TriggerBackfill(ctx, repoID); err != nil {
		t.Fatal(err)
	}

	eng.Start(ctx)
	// Poll the DB (no fixed sleep) until the job is done or we time out.
	deadline := time.Now().Add(5 * time.Second)
	done := false
	for time.Now().Before(deadline) {
		jobs, _ := st.ListJobsForRepo(ctx, repoID)
		if len(jobs) == 1 && jobs[0].Status == "done" {
			done = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	eng.Stop()
	if !done {
		t.Fatal("worker pool did not drain the backfill job")
	}
}

func mustUser(t *testing.T, st *store.Store) int64 {
	t.Helper()
	id, err := st.UpsertUser(context.Background(), &store.User{GitHubID: 1, Login: "u"})
	if err != nil {
		t.Fatal(err)
	}
	return id
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/sync/ -run 'TestProcessNextJob|TestEnqueueDueDeltas|TestStartStop' -v`
Expected: FAIL — `undefined: NewEngine`.

- [ ] **Step 3: Write minimal implementation**

`internal/sync/engine.go`:
```go
package sync

import (
	"context"
	"fmt"
	stdsync "sync"
	"time"

	"github-stats/internal/backfill"
	"github-stats/internal/githubapi"
	"github-stats/internal/store"
)

// ClientFactory mints a GitHub client for a repo (the API wires this to decrypt
// the tracking user's OAuth token). Returning an error fails the job.
type ClientFactory func(repoID int64) (*githubapi.Client, error)

// Config tunes the engine. All fields have sane defaults applied by NewEngine.
type Config struct {
	Now               func() time.Time // injected clock (defaults to time.Now)
	Concurrency       int              // worker goroutines (default 4)
	SchedulerInterval time.Duration    // scheduler tick (default 1m)
	DeltaCadence      time.Duration    // min age before a repo is re-delta'd (default 30m)
	MaxAttempts       int              // job failures before terminal error (default 5)
	FailBackoff       time.Duration    // base backoff between attempts (default 1m)
	IdleWait          time.Duration    // worker sleep when the queue is empty (default 200ms)
}

// Engine owns the worker pool, the scheduler, and the progress broadcaster.
type Engine struct {
	store     *store.Store
	newClient ClientFactory
	cfg       Config
	bc        *Broadcaster

	cancel context.CancelFunc
	wg     stdsync.WaitGroup
}

// NewEngine builds an Engine, applying defaults to any zero Config fields.
func NewEngine(st *store.Store, factory ClientFactory, cfg Config) *Engine {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	if cfg.SchedulerInterval <= 0 {
		cfg.SchedulerInterval = time.Minute
	}
	if cfg.DeltaCadence <= 0 {
		cfg.DeltaCadence = 30 * time.Minute
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 5
	}
	if cfg.FailBackoff <= 0 {
		cfg.FailBackoff = time.Minute
	}
	if cfg.IdleWait <= 0 {
		cfg.IdleWait = 200 * time.Millisecond
	}
	return &Engine{store: st, newClient: factory, cfg: cfg, bc: NewBroadcaster()}
}

// Broadcaster exposes the engine's progress broadcaster (the SSE handler
// subscribes to it).
func (e *Engine) Broadcaster() *Broadcaster { return e.bc }

// TriggerBackfill enqueues a backfill job for repoID, runnable now.
func (e *Engine) TriggerBackfill(ctx context.Context, repoID int64) (int64, error) {
	return e.store.EnqueueJob(ctx, repoID, "backfill", e.cfg.Now())
}

// TriggerDelta enqueues a delta job for repoID, runnable now.
func (e *Engine) TriggerDelta(ctx context.Context, repoID int64) (int64, error) {
	return e.store.EnqueueJob(ctx, repoID, "delta", e.cfg.Now())
}

// processNextJob leases one runnable job and runs it to completion, returning
// whether a job was processed. Job-level errors are recorded via FailJob (and
// do NOT propagate as the returned error); the returned error is reserved for
// infrastructure failures (e.g. the lease query itself). Safe to call from many
// goroutines concurrently — the lease is atomic.
func (e *Engine) processNextJob(ctx context.Context) (bool, error) {
	now := e.cfg.Now()
	job, err := e.store.LeaseNextJob(ctx, now)
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, nil
	}

	client, err := e.newClient(job.RepoID)
	if err != nil {
		e.bc.publish(job.RepoID, Event{RepoID: job.RepoID, Phase: "error", Message: err.Error(), Done: true})
		_ = e.store.FailJob(ctx, job.ID, "client: "+err.Error(), now, e.cfg.FailBackoff, e.cfg.MaxAttempts)
		return true, nil
	}

	e.bc.publish(job.RepoID, Event{RepoID: job.RepoID, Phase: job.Kind, Message: "started"})

	runErr := e.runJob(ctx, job, client)

	// Success → mark done.
	if runErr == nil {
		_ = e.store.CompleteJob(ctx, job.ID, e.cfg.Now())
		e.bc.publish(job.RepoID, Event{RepoID: job.RepoID, Phase: "done", Message: "complete", Done: true})
		return true, nil
	}
	// Budget exhausted → reschedule at the bucket reset WITHOUT counting a
	// failure (the cursor is already persisted by backfill/delta).
	if reset, exhausted := e.budgetReset(client); exhausted {
		_ = e.store.RescheduleJob(ctx, job.ID, reset)
		e.bc.publish(job.RepoID, Event{RepoID: job.RepoID, Phase: job.Kind, Message: "rate-limited; rescheduled"})
		return true, nil
	}
	// Genuine error → record the failure (reschedule-with-backoff or terminal).
	_ = e.store.FailJob(ctx, job.ID, runErr.Error(), e.cfg.Now(), e.cfg.FailBackoff, e.cfg.MaxAttempts)
	e.bc.publish(job.RepoID, Event{RepoID: job.RepoID, Phase: "error", Message: runErr.Error(), Done: true})
	return true, nil
}

// runJob dispatches by kind.
func (e *Engine) runJob(ctx context.Context, job *store.SyncJob, client *githubapi.Client) error {
	switch job.Kind {
	case "backfill":
		return backfill.Run(ctx, e.store, client, job.RepoID)
	case "delta":
		return RunDelta(ctx, e.store, client, job.RepoID, e.cfg.Now)
	default:
		return fmt.Errorf("unknown job kind %q", job.Kind)
	}
}

// budgetReset reports whether either bucket is exhausted and, if so, the soonest
// reset time to wait until.
func (e *Engine) budgetReset(client *githubapi.Client) (time.Time, bool) {
	if client.Budget == nil {
		return time.Time{}, false
	}
	gqlRem, gqlReset := client.Budget.GraphQL()
	restRem, restReset := client.Budget.REST()
	switch {
	case gqlRem <= 0 && !gqlReset.IsZero():
		return gqlReset, true
	case restRem <= 0 && !restReset.IsZero():
		return restReset, true
	default:
		return time.Time{}, false
	}
}

// enqueueDueDeltas enqueues a delta job for every tracked repo whose last sync
// is older than DeltaCadence and that has no pending/running job already. It is
// the scheduler's body, exposed for direct testing.
func (e *Engine) enqueueDueDeltas(ctx context.Context) error {
	now := e.cfg.Now()
	userIDs, err := e.trackingUserIDs(ctx)
	if err != nil {
		return err
	}
	seen := make(map[int64]bool)
	for _, uid := range userIDs {
		repos, err := e.store.ListTrackedRepos(ctx, uid)
		if err != nil {
			return err
		}
		for _, r := range repos {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true

			ss, err := e.store.GetSyncState(ctx, r.ID)
			if err != nil {
				return err
			}
			// Skip repos synced more recently than the cadence.
			if ss.LastBackfillAt != nil && now.Sub(*ss.LastBackfillAt) < e.cfg.DeltaCadence {
				continue
			}
			pending, err := e.hasOpenJob(ctx, r.ID)
			if err != nil {
				return err
			}
			if pending {
				continue
			}
			if _, err := e.store.EnqueueJob(ctx, r.ID, "delta", now); err != nil {
				return err
			}
		}
	}
	return nil
}

// hasOpenJob reports whether a repo already has a pending or running job (so the
// scheduler does not pile duplicates).
func (e *Engine) hasOpenJob(ctx context.Context, repoID int64) (bool, error) {
	jobs, err := e.store.ListJobsForRepo(ctx, repoID)
	if err != nil {
		return false, err
	}
	for _, j := range jobs {
		if j.Status == "pending" || j.Status == "running" {
			return true, nil
		}
	}
	return false, nil
}

// trackingUserIDs returns the distinct user ids that track any repo.
func (e *Engine) trackingUserIDs(ctx context.Context) ([]int64, error) {
	rows, err := e.store.DB.QueryContext(ctx, `SELECT DISTINCT user_id FROM repo_tracking`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Start launches the worker pool and the scheduler in background goroutines.
func (e *Engine) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	for i := 0; i < e.cfg.Concurrency; i++ {
		e.wg.Add(1)
		go e.worker(ctx)
	}
	e.wg.Add(1)
	go e.scheduler(ctx)
}

// worker loops processing jobs until ctx is cancelled, idling briefly when the
// queue is empty.
func (e *Engine) worker(ctx context.Context) {
	defer e.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		ran, err := e.processNextJob(ctx)
		if err != nil || !ran {
			select {
			case <-ctx.Done():
				return
			case <-time.After(e.cfg.IdleWait):
			}
		}
	}
}

// scheduler ticks on SchedulerInterval, enqueuing due delta jobs.
func (e *Engine) scheduler(ctx context.Context) {
	defer e.wg.Done()
	ticker := time.NewTicker(e.cfg.SchedulerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = e.enqueueDueDeltas(ctx)
		}
	}
}

// Stop cancels the engine and waits for all goroutines to exit. Safe to call
// even if Start was never called.
func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/sync/ -run 'TestProcessNextJob|TestEnqueueDueDeltas|TestStartStop' -v`
Expected: PASS.

- [ ] **Step 5: Run the whole sync package with the race detector**

Run: `go test ./internal/sync/ -race -v`
Expected: PASS — broadcaster, delta, and engine tests all green with no data races (worker pool + broadcaster are concurrency-safe).

- [ ] **Step 6: Commit**

```bash
git add internal/sync/engine.go internal/sync/engine_test.go
git commit -m "feat: sync engine worker pool + scheduler + job dispatch"
```

---

## Task 8: Extend api.NewServer signature (engine + cipher) and update M1 tests

**Files:**
- Modify: `internal/api/server.go`
- Modify: `internal/api/server_test.go`, `internal/api/me_test.go`

`NewServer` must now also receive the `*sync.Engine` and the `*crypto.Cipher` (to decrypt the caller's OAuth token when minting a per-user client). This task changes the signature and the M1 test helpers **without** adding routes yet (routes come in Tasks 9–10), keeping the change isolated and the suite green.

- [ ] **Step 1: Update the `NewServer` signature and store the new deps**

Replace the `Server` struct and `NewServer` in `internal/api/server.go` with:
```go
// Server holds HTTP dependencies and the router.
type Server struct {
	cfg    config.Config
	store  *store.Store
	auth   *auth.Service
	engine *sync.Engine
	cipher *crypto.Cipher
	router chi.Router
}

// NewServer builds the router with all routes mounted. It now also takes the
// sync Engine (for triggering/streaming syncs) and the Cipher (to decrypt the
// caller's OAuth token when minting a per-user GitHub client).
func NewServer(cfg config.Config, st *store.Store, authSvc *auth.Service, engine *sync.Engine, cipher *crypto.Cipher) *Server {
	s := &Server{cfg: cfg, store: st, auth: authSvc, engine: engine, cipher: cipher}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	// Auth routes.
	r.Get("/auth/github", authSvc.Login)
	r.Get("/auth/github/callback", authSvc.Callback)
	r.Get("/auth/logout", authSvc.Logout)

	// JSON API (auth-gated).
	r.Route("/api", func(api chi.Router) {
		api.Group(func(pr chi.Router) {
			pr.Use(authSvc.RequireUser)
			pr.Get("/me", s.me)
			pr.Post("/repos", s.addRepo)
			pr.Get("/repos", s.listRepos)
			pr.Delete("/repos/{id}", s.untrackRepo)
			pr.Post("/repos/{id}/refresh", s.refreshRepo)
			pr.Get("/repos/{id}/sync/stream", s.syncStream)
		})
		// Unknown /api/* paths return JSON 404 (not the SPA fallback).
		api.NotFound(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		})
	})

	// Embedded SPA (must be last; serves everything else).
	r.NotFound(web.Handler().ServeHTTP)

	s.router = r
	return s
}
```

- [ ] **Step 2: Update the imports in `internal/api/server.go`**

Replace the import block with (adds `encoding/json`, `crypto`, `sync`):
```go
import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github-stats/internal/auth"
	"github-stats/internal/config"
	"github-stats/internal/crypto"
	"github-stats/internal/store"
	"github-stats/internal/sync"
	"github-stats/web"
)
```

> Note: M1 already mounts the auth-gated group; this rewrite preserves `/me` and adds the repo routes plus the JSON-404 within the `/api` subtree. The M1 spec ("JSON-404 for unknown `/api/*`") is now realized explicitly via `api.NotFound`.

- [ ] **Step 3: Update the M1 `me_test.go` helper to pass the new args**

In `internal/api/me_test.go`, replace `testServer` with:
```go
func testServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cph, _ := crypto.NewCipher(make([]byte, 32))
	cfg := config.Config{SessionTTL: time.Hour, BaseURL: "http://localhost:8080"}
	svc := auth.NewService(cfg, st, &auth.OAuthClient{}, cph)
	eng := sync.NewEngine(st, func(repoID int64) (*githubapi.Client, error) {
		return githubapi.NewClient(githubapi.Options{Token: "t", GraphQLURL: "http://unused", RESTBaseURL: "http://unused", Store: st, HTTP: &http.Client{}}), nil
	}, sync.Config{})
	return NewServer(cfg, st, svc, eng, cph), st
}
```
and update its import block to add `githubapi` and `sync`:
```go
import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github-stats/internal/auth"
	"github-stats/internal/config"
	"github-stats/internal/crypto"
	"github-stats/internal/githubapi"
	"github-stats/internal/store"
	"github-stats/internal/sync"
)
```

- [ ] **Step 4: Check `server_test.go`**

If M1's `internal/api/server_test.go` also constructs a `Server` via `NewServer`, apply the identical helper change there (same five-argument call). If `server_test.go` reuses `me_test.go`'s `testServer` helper (same package), no change is needed beyond Step 3. Inspect and reconcile:

Run: `grep -n "NewServer(" internal/api/*_test.go`
Expected: every call site passes the new `(cfg, st, svc, eng, cph)` five-argument form.

- [ ] **Step 5: Add a stub for the not-yet-written handlers so the package compiles**

The route mounts reference `s.addRepo/listRepos/untrackRepo/refreshRepo/syncStream`, written in Tasks 9–10. To keep this task's commit compiling, create `internal/api/repos.go` and `internal/api/stream.go` as minimal stubs now, then flesh them out next:

`internal/api/repos.go` (stub):
```go
package api

import "net/http"

func (s *Server) addRepo(w http.ResponseWriter, r *http.Request)      { http.Error(w, "not implemented", http.StatusNotImplemented) }
func (s *Server) listRepos(w http.ResponseWriter, r *http.Request)    { http.Error(w, "not implemented", http.StatusNotImplemented) }
func (s *Server) untrackRepo(w http.ResponseWriter, r *http.Request)  { http.Error(w, "not implemented", http.StatusNotImplemented) }
func (s *Server) refreshRepo(w http.ResponseWriter, r *http.Request)  { http.Error(w, "not implemented", http.StatusNotImplemented) }
```

`internal/api/stream.go` (stub):
```go
package api

import "net/http"

func (s *Server) syncStream(w http.ResponseWriter, r *http.Request) { http.Error(w, "not implemented", http.StatusNotImplemented) }
```

- [ ] **Step 6: Verify the api package compiles and M1 tests pass**

Run: `go test ./internal/api/ -run 'TestMe|TestSPAFallback' -v`
Expected: PASS — `/api/me`, unauthorized, and SPA fallback still work with the new signature.

- [ ] **Step 7: Commit**

```bash
git add internal/api/server.go internal/api/me_test.go internal/api/server_test.go internal/api/repos.go internal/api/stream.go
git commit -m "refactor: NewServer takes sync engine + cipher; mount repo routes (stubs)"
```

---

## Task 9: Repo HTTP endpoints (add/list/untrack/refresh)

**Files:**
- Modify: `internal/api/repos.go`
- Create: `internal/api/repos_test.go`

`addRepo` builds a per-user `githubapi.Client` from the caller's decrypted `oauth` credential, `FetchRepoMeta`, `UpsertRepo`, `TrackRepo`, enqueues a `backfill` job, and returns the repo JSON. `listRepos` returns the user's tracked repos joined with their sync status (`sync_status`, `last_synced_at` from `sync_state`). `untrackRepo` removes tracking. `refreshRepo` enqueues a `delta` job. The per-user client builder is shared by add/refresh.

- [ ] **Step 1: Write the failing test**

`internal/api/repos_test.go`:
```go
package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github-stats/internal/auth"
	"github-stats/internal/config"
	"github-stats/internal/crypto"
	"github-stats/internal/githubapi"
	"github-stats/internal/store"
	"github-stats/internal/sync"
)

// serverWithGitHub builds a Server whose per-user client factory points at the
// given fake GitHub URL, plus a seeded logged-in user with an encrypted oauth
// credential. Returns the server, store, and the user's session cookie.
func serverWithGitHub(t *testing.T, ghURL string) (*Server, *store.Store, *http.Cookie) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cph, _ := crypto.NewCipher(make([]byte, 32))
	cfg := config.Config{SessionTTL: time.Hour, BaseURL: "http://localhost:8080"}
	svc := auth.NewService(cfg, st, &auth.OAuthClient{}, cph)

	ctx := context.Background()
	uid, _ := st.UpsertUser(ctx, &store.User{GitHubID: 1, Login: "neo"})
	enc, _ := cph.Encrypt([]byte("gho_user_token"))
	if err := st.UpsertCredential(ctx, &store.Credential{UserID: uid, Kind: "oauth", EncToken: enc, Scopes: "repo"}); err != nil {
		t.Fatal(err)
	}
	sess, _ := st.CreateSession(ctx, uid, time.Hour)

	factory := func(repoID int64) (*githubapi.Client, error) {
		return githubapi.NewClient(githubapi.Options{
			Token: "gho_user_token", GraphQLURL: ghURL, RESTBaseURL: ghURL, Store: st, HTTP: &http.Client{},
		}), nil
	}
	eng := sync.NewEngine(st, factory, sync.Config{})
	srv := NewServer(cfg, st, svc, eng, cph)
	return srv, st, &http.Cookie{Name: "gs_session", Value: sess.ID}
}

func TestAddRepoFetchesTracksAndEnqueues(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		if r.Header.Get("Authorization") != "Bearer gho_user_token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"repository":{"databaseId":777,"nameWithOwner":"octocat/hello",
			"isPrivate":true,"description":"hi","stargazerCount":3,"forkCount":1,
			"defaultBranchRef":{"name":"main"}},
			"rateLimit":{"cost":1,"remaining":4999,"resetAt":"2026-06-01T13:00:00Z"}}}`))
	}))
	defer gh.Close()

	srv, st, cookie := serverWithGitHub(t, gh.URL)

	body := strings.NewReader(`{"full_name":"octocat/hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/repos", body)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["full_name"] != "octocat/hello" || got["is_private"] != true || got["default_branch"] != "main" {
		t.Fatalf("repo json = %v", got)
	}

	// Repo upserted, tracked, and a backfill job enqueued.
	ctx := context.Background()
	r, err := st.GetRepoByFullName(ctx, "octocat/hello")
	if err != nil {
		t.Fatal(err)
	}
	if tracked, _ := st.IsTracked(ctx, 1, r.ID); !tracked {
		t.Fatal("repo not tracked")
	}
	jobs, _ := st.ListJobsForRepo(ctx, r.ID)
	if len(jobs) != 1 || jobs[0].Kind != "backfill" {
		t.Fatalf("expected 1 backfill job, got %+v", jobs)
	}
}

func TestAddRepoRejectsBadBody(t *testing.T) {
	srv, _, cookie := serverWithGitHub(t, "http://unused")
	req := httptest.NewRequest(http.MethodPost, "/api/repos", strings.NewReader(`{"full_name":""}`))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestListReposIncludesSyncStatus(t *testing.T) {
	srv, st, cookie := serverWithGitHub(t, "http://unused")
	ctx := context.Background()

	repoID, _ := st.UpsertRepo(ctx, &store.Repo{
		GitHubID: 5, FullName: "octocat/hello", IsPrivate: false, DefaultBranch: "main",
	})
	st.TrackRepo(ctx, 1, repoID)
	last := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	st.UpsertSyncState(ctx, &store.SyncState{RepoID: repoID, LastBackfillAt: &last, Status: "complete"})

	req := httptest.NewRequest(http.MethodGet, "/api/repos", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var repos []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &repos); err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("len = %d, want 1", len(repos))
	}
	r := repos[0]
	for _, k := range []string{"id", "full_name", "is_private", "default_branch", "sync_status", "last_synced_at"} {
		if _, ok := r[k]; !ok {
			t.Fatalf("repo json missing key %q: %v", k, r)
		}
	}
	if r["sync_status"] != "complete" {
		t.Fatalf("sync_status = %v, want complete", r["sync_status"])
	}
}

func TestUntrackRepo(t *testing.T) {
	srv, st, cookie := serverWithGitHub(t, "http://unused")
	ctx := context.Background()
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 5, FullName: "a/b", DefaultBranch: "main"})
	st.TrackRepo(ctx, 1, repoID)

	req := httptest.NewRequest(http.MethodDelete, "/api/repos/"+strconv.FormatInt(repoID, 10), nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if tracked, _ := st.IsTracked(ctx, 1, repoID); tracked {
		t.Fatal("repo still tracked after delete")
	}
}

func TestRefreshRepoEnqueuesDelta(t *testing.T) {
	srv, st, cookie := serverWithGitHub(t, "http://unused")
	ctx := context.Background()
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 5, FullName: "a/b", DefaultBranch: "main"})
	st.TrackRepo(ctx, 1, repoID)

	req := httptest.NewRequest(http.MethodPost, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/refresh", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	jobs, _ := st.ListJobsForRepo(ctx, repoID)
	if len(jobs) != 1 || jobs[0].Kind != "delta" {
		t.Fatalf("expected 1 delta job, got %+v", jobs)
	}
}

func TestRefreshRejectsUntrackedRepo(t *testing.T) {
	srv, st, cookie := serverWithGitHub(t, "http://unused")
	ctx := context.Background()
	// Repo exists but the caller does not track it.
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 5, FullName: "a/b", DefaultBranch: "main"})

	req := httptest.NewRequest(http.MethodPost, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/refresh", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for untracked repo", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run 'TestAddRepo|TestListRepos|TestUntrackRepo|TestRefreshRepo|TestRefreshRejects' -v`
Expected: FAIL — the stub handlers return 501, so assertions fail.

- [ ] **Step 3: Replace the stub `internal/api/repos.go` with the real handlers**

`internal/api/repos.go`:
```go
package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github-stats/internal/auth"
	"github-stats/internal/githubapi"
	"github-stats/internal/store"
)

// repoJSON is the wire shape for a tracked repo (M4/M5 depend on these keys).
type repoJSON struct {
	ID            int64   `json:"id"`
	FullName      string  `json:"full_name"`
	IsPrivate     bool    `json:"is_private"`
	DefaultBranch string  `json:"default_branch"`
	Description   string  `json:"description"`
	Stargazers    int64   `json:"stargazers"`
	Forks         int64   `json:"forks"`
	SyncStatus    string  `json:"sync_status"`
	LastSyncedAt  *string `json:"last_synced_at"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// userClient mints a per-user GitHub client from the caller's decrypted oauth token.
func (s *Server) userClient(r *http.Request, userID int64) (*githubapi.Client, error) {
	cred, err := s.store.GetCredential(r.Context(), userID, "oauth")
	if err != nil {
		return nil, err
	}
	token, err := s.cipher.Decrypt(cred.EncToken)
	if err != nil {
		return nil, err
	}
	return githubapi.NewClient(githubapi.Options{
		Token:       string(token),
		GraphQLURL:  s.cfg.GitHubAPIBaseURL + "/graphql",
		RESTBaseURL: s.cfg.GitHubAPIBaseURL,
		Store:       s.store,
	}), nil
}

// addRepo handles POST /api/repos: fetch meta with the caller's token, upsert,
// track, and enqueue a backfill job.
func (s *Server) addRepo(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		FullName string `json:"full_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	owner, name := splitFullName(body.FullName)
	if owner == "" || name == "" {
		http.Error(w, "full_name must be owner/name", http.StatusBadRequest)
		return
	}

	client, err := s.userClient(r, u.ID)
	if err != nil {
		http.Error(w, "no github credential", http.StatusBadGateway)
		return
	}
	meta, err := client.FetchRepoMeta(r.Context(), owner, name)
	if err != nil {
		http.Error(w, "fetch repo failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	repoID, err := s.store.UpsertRepo(r.Context(), meta)
	if err != nil {
		http.Error(w, "persist repo failed", http.StatusInternalServerError)
		return
	}
	if err := s.store.TrackRepo(r.Context(), u.ID, repoID); err != nil {
		http.Error(w, "track failed", http.StatusInternalServerError)
		return
	}
	if _, err := s.engine.TriggerBackfill(r.Context(), repoID); err != nil {
		http.Error(w, "enqueue failed", http.StatusInternalServerError)
		return
	}

	ss, _ := s.store.GetSyncState(r.Context(), repoID)
	writeJSON(w, http.StatusCreated, toRepoJSON(meta, repoID, ss))
}

// listRepos handles GET /api/repos: the caller's tracked repos with sync status.
func (s *Server) listRepos(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	repos, err := s.store.ListTrackedRepos(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	out := make([]repoJSON, 0, len(repos))
	for i := range repos {
		ss, _ := s.store.GetSyncState(r.Context(), repos[i].ID)
		out = append(out, toRepoJSON(&repos[i], repos[i].ID, ss))
	}
	writeJSON(w, http.StatusOK, out)
}

// untrackRepo handles DELETE /api/repos/{id}.
func (s *Server) untrackRepo(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	repoID, err := repoIDParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := s.store.UntrackRepo(r.Context(), u.ID, repoID); err != nil {
		http.Error(w, "untrack failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// refreshRepo handles POST /api/repos/{id}/refresh: enqueue a delta job.
func (s *Server) refreshRepo(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	repoID, err := repoIDParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	tracked, err := s.store.IsTracked(r.Context(), u.ID, repoID)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	if !tracked {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if _, err := s.engine.TriggerDelta(r.Context(), repoID); err != nil {
		http.Error(w, "enqueue failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func repoIDParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func toRepoJSON(repo *store.Repo, repoID int64, ss *store.SyncState) repoJSON {
	j := repoJSON{
		ID:            repoID,
		FullName:      repo.FullName,
		IsPrivate:     repo.IsPrivate,
		DefaultBranch: repo.DefaultBranch,
		Description:   repo.Description,
		Stargazers:    repo.Stargazers,
		Forks:         repo.Forks,
	}
	if ss != nil {
		j.SyncStatus = ss.Status
		if ss.LastBackfillAt != nil {
			formatted := ss.LastBackfillAt.UTC().Format("2006-01-02T15:04:05Z07:00")
			j.LastSyncedAt = &formatted
		}
	}
	return j
}

// splitFullName splits "owner/name" into its parts.
func splitFullName(fullName string) (owner, name string) {
	for i := 0; i < len(fullName); i++ {
		if fullName[i] == '/' {
			return fullName[:i], fullName[i+1:]
		}
	}
	return fullName, ""
}
```

> The `meta` returned by `FetchRepoMeta` has `ID == 0`; `UpsertRepo` returns the real local id, which `toRepoJSON` uses for the `id` field and the enqueue. `addRepo` builds its GitHub client via `s.userClient`, which derives its base URLs from `s.cfg.GitHubAPIBaseURL` (`https://api.github.com` in prod) — **not** from the engine's client factory. So the `addRepo` test must point `cfg.GitHubAPIBaseURL` at its fake GitHub server; Step 4 does exactly that in `serverWithGitHub`. (The engine factory in `serverWithGitHub` only matters for jobs the engine itself runs, which these endpoint tests do not drive.)

- [ ] **Step 4: Point the test config's API base URL at the fake server**

In `internal/api/repos_test.go`'s `serverWithGitHub`, set the config's GitHub API base URL so `addRepo`'s `userClient` reaches the fake server. Replace the `cfg :=` line with:
```go
	cfg := config.Config{
		SessionTTL:       time.Hour,
		BaseURL:          "http://localhost:8080",
		GitHubAPIBaseURL: ghURL,
	}
```

> The fake GitHub handler matches GraphQL by request body (`query` field) regardless of path, so pointing both `GraphQLURL` (`ghURL+"/graphql"`) and `RESTBaseURL` (`ghURL`) at the same `httptest` server works for `FetchRepoMeta`.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/api/ -run 'TestAddRepo|TestListRepos|TestUntrackRepo|TestRefreshRepo|TestRefreshRejects' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/repos.go internal/api/repos_test.go
git commit -m "feat: /api/repos add/list/untrack/refresh endpoints"
```

---

## Task 10: SSE sync-progress stream

**Files:**
- Modify: `internal/api/stream.go`
- Create: `internal/api/stream_test.go`

`syncStream` handles `GET /api/repos/{id}/sync/stream`: it verifies the caller tracks the repo, sets `text/event-stream` headers, subscribes to the engine broadcaster, and writes each `Event` as an SSE `data:` frame, flushing after each. The handler returns when the client disconnects (`r.Context().Done()`) or a terminal `Done` event is sent. Tests use a flushable recorder and bound the read so the test always terminates.

- [ ] **Step 1: Write the failing test**

`internal/api/stream_test.go`:
```go
package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	syncpkg "github-stats/internal/sync"
)

func TestSyncStreamForwardsEvents(t *testing.T) {
	srv, st, cookie := serverWithGitHub(t, "http://unused")
	ctx := context.Background()
	repoID, _ := mustTrackedRepo(t, st)

	// Use the real chi server over a listener so streaming + flush behave.
	httpSrv := httptest.NewServer(srv.Router())
	defer httpSrv.Close()

	req, _ := http.NewRequest(http.MethodGet,
		httpSrv.URL+"/api/repos/"+strconv.FormatInt(repoID, 10)+"/sync/stream", nil)
	req.AddCookie(cookie)

	// Cancel the request after we have read one event so the handler returns.
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req = req.WithContext(reqCtx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}

	// Publish a terminal event from the engine broadcaster; the handler should
	// forward it and then close.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Give the handler a moment to subscribe, then publish.
		for i := 0; i < 200; i++ {
			srv.engine.Broadcaster().PublishForTest(repoID, syncpkg.Event{RepoID: repoID, Phase: "done", Message: "complete", Done: true})
			time.Sleep(5 * time.Millisecond)
		}
	}()

	reader := bufio.NewReader(resp.Body)
	var got string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if strings.HasPrefix(line, "data:") {
			got = line
			break
		}
		if err != nil {
			break
		}
	}
	cancel()
	wg.Wait()

	if !strings.Contains(got, `"phase":"done"`) || !strings.Contains(got, `"done":true`) {
		t.Fatalf("did not receive forwarded SSE event, got %q", got)
	}
}

func TestSyncStreamRejectsUntracked(t *testing.T) {
	srv, _, cookie := serverWithGitHub(t, "http://unused")
	req := httptest.NewRequest(http.MethodGet, "/api/repos/999/sync/stream", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for untracked repo", rec.Code)
	}
}
```

`mustTrackedRepo` is a small helper — add it to `internal/api/repos_test.go`:
```go
func mustTrackedRepo(t *testing.T, st *store.Store) (int64, *store.Repo) {
	t.Helper()
	ctx := context.Background()
	repoID, err := st.UpsertRepo(ctx, &store.Repo{GitHubID: 42, FullName: "octocat/hello", DefaultBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.TrackRepo(ctx, 1, repoID); err != nil {
		t.Fatal(err)
	}
	r, _ := st.GetRepo(ctx, repoID)
	return repoID, r
}
```

The SSE test publishes via a test-only broadcaster method. Add `PublishForTest` to `internal/sync/broadcaster.go` (a thin exported wrapper over `publish`, used only by tests in other packages):
```go
// PublishForTest exposes publish for cross-package tests (e.g. the api SSE
// handler test). Production code in this package calls publish directly.
func (b *Broadcaster) PublishForTest(repoID int64, ev Event) { b.publish(repoID, ev) }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run TestSyncStream -v`
Expected: FAIL — the stub `syncStream` returns 501; `PublishForTest` undefined.

- [ ] **Step 3: Add `PublishForTest` to the broadcaster**

Apply the `PublishForTest` method shown above to `internal/sync/broadcaster.go`.

- [ ] **Step 4: Replace the stub `internal/api/stream.go` with the real handler**

`internal/api/stream.go`:
```go
package api

import (
	"encoding/json"
	"net/http"

	"github-stats/internal/auth"
)

// syncStream handles GET /api/repos/{id}/sync/stream as Server-Sent Events of
// the repo's sync progress. It streams events published to the engine
// broadcaster until the client disconnects or a terminal Done event is sent.
func (s *Server) syncStream(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	repoID, err := repoIDParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	tracked, err := s.store.IsTracked(r.Context(), u.ID, repoID)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	if !tracked {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, cancel := s.engine.Broadcaster().Subscribe(repoID)
	defer cancel()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if _, err := w.Write([]byte("data: ")); err != nil {
				return
			}
			if _, err := w.Write(payload); err != nil {
				return
			}
			if _, err := w.Write([]byte("\n\n")); err != nil {
				return
			}
			flusher.Flush()
			if ev.Done {
				return
			}
		}
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/api/ -run TestSyncStream -v`
Expected: PASS — content-type is `text/event-stream`, the published `done` event is forwarded, and the untracked case returns 404.

- [ ] **Step 6: Run the whole api package**

Run: `go test ./internal/api/ -v`
Expected: PASS — M1 (`me`, SPA fallback) plus M3 (repos, stream).

- [ ] **Step 7: Commit**

```bash
git add internal/api/stream.go internal/api/stream_test.go internal/api/repos_test.go internal/sync/broadcaster.go
git commit -m "feat: SSE /api/repos/{id}/sync/stream progress endpoint"
```

---

## Task 11: Wire the engine into main.go

**Files:**
- Modify: `cmd/server/main.go`

Construct the `Engine` with a per-user client factory that decrypts each tracking owner's OAuth token, `Start` it, inject it (and the cipher) into `NewServer`, run the HTTP server with graceful shutdown, and `Stop` the engine on exit.

- [ ] **Step 1: Replace `cmd/server/main.go` with the engine-aware wiring**

`cmd/server/main.go`:
```go
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github-stats/internal/api"
	"github-stats/internal/auth"
	"github-stats/internal/config"
	"github-stats/internal/crypto"
	"github-stats/internal/githubapi"
	"github-stats/internal/store"
	"github-stats/internal/sync"
)

func main() {
	_ = godotenv.Load() // optional .env in dev; ignored if absent

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	cipher, err := crypto.NewCipher(cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("cipher: %v", err)
	}

	oauth := &auth.OAuthClient{
		ClientID:     cfg.GitHubClientID,
		ClientSecret: cfg.GitHubClientSecret,
		RedirectURL:  cfg.RedirectURL(),
		OAuthBaseURL: cfg.GitHubOAuthBaseURL,
		APIBaseURL:   cfg.GitHubAPIBaseURL,
		HTTP:         http.DefaultClient,
	}
	authSvc := auth.NewService(cfg, st, oauth, cipher)

	// Per-repo client factory: mint a GitHub client using the OAuth token of the
	// repo's first tracking user (decrypted with the cipher). This is the minimal
	// per-user client construction M3 needs to add and sync a repo.
	factory := newClientFactory(st, cipher, cfg)

	engine := sync.NewEngine(st, factory, sync.Config{
		Concurrency:       4,
		SchedulerInterval: time.Minute,
		DeltaCadence:      30 * time.Minute,
		MaxAttempts:       5,
		FailBackoff:       time.Minute,
	})

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	engine.Start(rootCtx)
	defer engine.Stop()

	srv := api.NewServer(cfg, st, authSvc, engine, cipher)
	httpSrv := &http.Server{Addr: cfg.Addr, Handler: srv}

	go func() {
		log.Printf("listening on %s", cfg.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-rootCtx.Done()
	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
}

// newClientFactory builds a sync.ClientFactory that resolves a repo to a
// tracking user's decrypted OAuth token and returns a GitHub client for it.
func newClientFactory(st *store.Store, cipher *crypto.Cipher, cfg config.Config) sync.ClientFactory {
	return func(repoID int64) (*githubapi.Client, error) {
		ctx := context.Background()
		var userID int64
		if err := st.DB.QueryRowContext(ctx,
			`SELECT user_id FROM repo_tracking WHERE repo_id = ? ORDER BY created_at ASC LIMIT 1`,
			repoID,
		).Scan(&userID); err != nil {
			return nil, err
		}
		cred, err := st.GetCredential(ctx, userID, "oauth")
		if err != nil {
			return nil, err
		}
		token, err := cipher.Decrypt(cred.EncToken)
		if err != nil {
			return nil, err
		}
		return githubapi.NewClient(githubapi.Options{
			Token:       string(token),
			GraphQLURL:  cfg.GitHubAPIBaseURL + "/graphql",
			RESTBaseURL: cfg.GitHubAPIBaseURL,
			Store:       st,
		}), nil
	}
}
```

- [ ] **Step 2: Verify the whole module builds and all tests pass**

Run: `go build ./... && go test ./...`
Expected: build succeeds; all packages PASS (M1 + M2 unchanged, M3 store/githubapi/sync/api green).

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat: wire sync engine into server entrypoint with graceful shutdown"
```

---

## Task 12: Full-suite verification

**Files:**
- None (verification only)

- [ ] **Step 1: Run the entire test suite (with race detector for the engine)**

Run: `go build ./... && go test ./... && go test ./internal/sync/ ./internal/api/ -race`
Expected: build succeeds; **all** packages PASS; no data races reported.

- [ ] **Step 2: Vet for static issues**

Run: `go vet ./...`
Expected: no findings. (In particular, confirm the local `package sync` does not shadow the stdlib in a way `vet` flags — the `stdsync` alias is used wherever the stdlib mutex/once/waitgroup is needed.)

- [ ] **Step 3: Commit (only if `go mod tidy` changed anything)**

```bash
go mod tidy
git add go.mod go.sum
git commit -m "chore: tidy modules after M3" || echo "nothing to commit"
```

---

## Out of scope (later milestones)

M3 deliberately excludes the following; they belong to later plans and are **not** to be built here:

- **Metrics computation** (`commit_rate`, `time_to_merge`, `review_latency`, leaderboards, the `Metric` interface + `Registry`, and `GET /api/repos/{id}/metrics`, `GET /api/repos/{id}` overview bundle, `GET /api/repos/{id}/latest/{commits|prs|issues}`) — **M4**. M3 only keeps the `daily_*` aggregates fresh via delta + recompute; it adds no read/metrics endpoints. (`POST/GET /api/repos`, `DELETE /api/repos/{id}`, `refresh`, and `sync/stream` are the only repo endpoints M3 ships.)
- **Dashboard UI** (React views, uPlot charts, window/exclude-bots toggles, URL shortcut, live sync status UI) — **M5**.
- **Collections / `collection_repos`, save/load, optional PAT credential, "log out everywhere", bot-detection UX, rate-limit UX, self-host hardening docs** — **M6**. M3's per-user client construction is the minimal needed to add and sync a repo; richer credential management is M6.

---

## Self-Review notes

- **Spec coverage:** every M3 bullet maps to a task. (1) `0003` migration (sync_jobs + repo_tracking) → Task 1; job-queue DAO with atomic `LeaseNextJob`, `EnqueueJob`/`CompleteJob`/`FailJob`(backoff + max-attempts)/`RescheduleJob`/`ListJobsForRepo` → Task 2; `TrackRepo`/`UntrackRepo`/`ListTrackedRepos`/`IsTracked` → Task 3. (2) delta fetchers `FetchCommitsSince`(`history(since:)`), `FetchPullRequestsUpdated`/`FetchIssuesUpdated`(`UPDATED_AT DESC`, caller stops at cutoff) → Task 4. (3) delta sync with overlap window + recompute → Task 6; SSE broadcaster → Task 5; worker pool + scheduler + `Engine.Start/Stop/TriggerBackfill/TriggerDelta` + deterministic `processNextJob`/`enqueueDueDeltas` + budget-exhaustion reschedule → Task 7. (4) HTTP endpoints `POST/GET /api/repos`, `DELETE /api/repos/{id}`, `POST /api/repos/{id}/refresh`, `GET /api/repos/{id}/sync/stream` (typed JSON; JSON-404 via `api.NotFound`) → Tasks 9–10. (5) `NewServer` signature change + M1 test updates → Task 8; `main.go` engine wiring + graceful shutdown → Task 11.
- **No placeholders:** every code/test step contains complete, compilable Go and complete SQL. All three new GraphQL query strings (`commitsSinceQuery`, `pullRequestsUpdatedQuery`, `issuesUpdatedQuery`) are written out in full, as is the `LeaseNextJob` atomic-claim SQL and the `0003` schema.
- **Type consistency with M1/M2:** reuses `store.Store/Repo/Commit/PullRequest/Issue/SyncState`, `store.GetCredential/UpsertRepo/GetRepo/GetRepoByFullName/GetSyncState/UpsertSyncState/RecomputeDailyStats`, `store.UpsertCommits/UpsertPullRequests/UpsertIssues`; `githubapi.Client/Options/NewClient/Budget` (`GraphQL()`/`REST()` reset getters drive the exhaustion-reschedule), the M2 page type `CommitPage` (reused by `FetchCommitsSince`) and the M2 helpers `parseTime/parseTimePtr/IsBot/pageInfo`; `backfill.Run` (unchanged) for `kind="backfill"`; `crypto.Cipher.Decrypt`; `auth.Service/RequireUser/UserFromContext`; `config.Config.GitHubAPIBaseURL`; cookie name `gs_session`; `Server.Router()`. New types introduced early (`SyncJob`, `Event`, `Broadcaster`, `Engine`, `Config`, `ClientFactory`, `UpdatedPR(Page)`/`UpdatedIssue(Page)`, `repoJSON`) are used unchanged downstream.
- **Determinism (no flaky sleeps in assertions):** the clock is injected everywhere a comparison happens — `Engine.Config.Now`, `RunDelta(..., now func() time.Time)`, and the store DAOs (`LeaseNextJob(ctx, now)`, `FailJob(ctx, id, msg, now, backoff, max)`, `RescheduleJob(ctx, id, runAt)`, `EnqueueJob(ctx, repoID, kind, now)`) take explicit times. Tests drive the synchronous `processNextJob`/`enqueueDueDeltas`/`RunDelta` directly. The single `Start/Stop` lifecycle test polls the DB on a bounded deadline (no fixed-duration assertion sleep) and the SSE test bounds its read with a context timeout, so neither hangs nor flakes. `internal/sync` is tested with `-race`.
- **Atomic lease proven:** `TestLeaseIsAtomicAcrossLeasers` asserts two back-to-back leases of a single runnable job split into exactly one hit + one miss; `TestLeaseSkipsFutureAndLocked` covers `next_run_at`/`locked_at` gating. The claim-and-return is one conditional `UPDATE sync_jobs ... WHERE id = (SELECT ... LIMIT 1) RETURNING ...`, so concurrent leasers can never both win and there is no separate re-select to race (and SQLite's `SetMaxOpenConns(1)` from M1 serializes writers regardless). `RETURNING` requires SQLite 3.35+, which the pure-Go `modernc.org/sqlite` driver M1 pins satisfies.
- **Concrete signature change + wiring:** Task 8 spells out the exact new `NewServer(cfg, st, authSvc, engine, cipher)` signature, the import additions, and the M1 `me_test.go`/`server_test.go` helper updates (plus a `grep` reconciliation step); Task 11 gives the full `main.go` including the per-user client factory and graceful shutdown. No hand-waving.
- **SSE correctness:** the handler sets `text/event-stream`, subscribes to the engine broadcaster, writes `data: <json>\n\n` frames, flushes each, and returns on client disconnect or a terminal `Done` event. The broadcaster never blocks a worker (full-buffer events are dropped per subscriber), and `cancel` closes/removes the subscription. `PublishForTest` is the only test-only seam, clearly named.
- **Intentional choices (flagged):** (a) delta PR/issue fetchers return **wrapper** types (`UpdatedPR`/`UpdatedIssue` carrying `UpdatedAt`) rather than adding an `updated_at` column to M2's `store.PullRequest`/`store.Issue`, keeping the M2 store schema/types untouched. (b) The scheduler's "due" check uses `sync_state.LastBackfillAt` as the last-sync marker and guards against duplicate enqueues via `hasOpenJob`; `RunDelta` does not currently stamp `LastBackfillAt`, so delta cadence keys off the last *backfill* — acceptable for M3 (a backfill always precedes deltas); M4/M6 may add a dedicated `last_delta_at`. (c) `repoJSON.last_synced_at` derives from `LastBackfillAt`; it is `null` until the first backfill completes. (d) The `main.go` client factory resolves a repo to its *first* tracking user's token (sufficient for single-owner self-host); multi-owner token selection is out of scope.

---

## What M4 will add (next plan)

- **Metrics registry** (spec §7): the `Metric` interface (`Key()`, `Compute(ctx, store, repoID, Window, opts) (Result, error)`), a `Registry` mapping `key → Metric`, and the Extended metric set as independently-testable units (`commit_rate`, `pr_throughput`, `time_to_merge`, `review_latency`, `issue_lifetime`, `open_issue_age`, `code_churn`, `comment_volume`, `contributor_leaderboard`, plus the `ema` helper). All read **only** from the `daily_*` aggregates M2/M3 maintain.
- **Read endpoints**: `GET /api/repos/{id}/metrics?keys=&window=&exclude_bots=`, `GET /api/repos/{id}` (overview bundle), `GET /api/repos/{id}/latest/{commits|prs|issues}` — mounted in the same auth-gated `/api` group M3 established, returning the ~4 result shapes (time-series, scalar, distribution, leaderboard).
- **`exclude_bots`** filtering via the `is_bot` flag already populated by M2/M3 ingestion.

---

## Public API surface M3 exposes

For the M4 plan to build on precisely. All under module `github-stats`.

**`internal/store` (package `store`) — new in M3 (joins existing `Store{DB *sql.DB}`):**
```go
type SyncJob struct { ID, RepoID int64; Kind, Status, CursorState string; Attempts int; NextRunAt time.Time; LockedAt *time.Time; LastError string; CreatedAt time.Time }

func (s *Store) EnqueueJob(ctx context.Context, repoID int64, kind string, now time.Time) (int64, error)
func (s *Store) EnqueueJobAt(ctx context.Context, repoID int64, kind string, runAt time.Time) (int64, error)
func (s *Store) LeaseNextJob(ctx context.Context, now time.Time) (*SyncJob, error) // atomic claim; (nil,nil) when none
func (s *Store) CompleteJob(ctx context.Context, id int64, now time.Time) error
func (s *Store) FailJob(ctx context.Context, id int64, msg string, now time.Time, backoff time.Duration, maxAttempts int) error
func (s *Store) RescheduleJob(ctx context.Context, id int64, runAt time.Time) error // no attempt bump (budget yield)
func (s *Store) ListJobsForRepo(ctx context.Context, repoID int64) ([]SyncJob, error) // newest first

func (s *Store) TrackRepo(ctx context.Context, userID, repoID int64) error    // idempotent
func (s *Store) UntrackRepo(ctx context.Context, userID, repoID int64) error
func (s *Store) IsTracked(ctx context.Context, userID, repoID int64) (bool, error)
func (s *Store) ListTrackedRepos(ctx context.Context, userID int64) ([]Repo, error) // newest tracking first
```

**`internal/githubapi` (package `githubapi`) — new delta fetchers:**
```go
type UpdatedPR    struct { PullRequest store.PullRequest; UpdatedAt time.Time }
type UpdatedPRPage struct { PRs    []UpdatedPR;    EndCursor string; HasNextPage bool }
type UpdatedIssue struct { Issue store.Issue; UpdatedAt time.Time }
type UpdatedIssuePage struct { Issues []UpdatedIssue; EndCursor string; HasNextPage bool }

func (c *Client) FetchCommitsSince(ctx context.Context, owner, name, branch string, since time.Time, after string) (*CommitPage, error)
func (c *Client) FetchPullRequestsUpdated(ctx context.Context, owner, name, after string) (*UpdatedPRPage, error)
func (c *Client) FetchIssuesUpdated(ctx context.Context, owner, name, after string) (*UpdatedIssuePage, error)
```

**`internal/sync` (package `sync`) — the engine:**
```go
type Event struct { RepoID int64; Phase, Message string; Done bool } // JSON-tagged for SSE

type Broadcaster struct { /* unexported */ }
func NewBroadcaster() *Broadcaster
func (b *Broadcaster) Subscribe(repoID int64) (<-chan Event, func()) // channel + cancel
func (b *Broadcaster) PublishForTest(repoID int64, ev Event)         // test-only seam

type ClientFactory func(repoID int64) (*githubapi.Client, error)
type Config struct { Now func() time.Time; Concurrency int; SchedulerInterval, DeltaCadence, FailBackoff, IdleWait time.Duration; MaxAttempts int }

type Engine struct { /* unexported */ }
func NewEngine(st *store.Store, factory ClientFactory, cfg Config) *Engine
func (e *Engine) Broadcaster() *Broadcaster
func (e *Engine) TriggerBackfill(ctx context.Context, repoID int64) (int64, error)
func (e *Engine) TriggerDelta(ctx context.Context, repoID int64) (int64, error)
func (e *Engine) Start(ctx context.Context) // launches worker pool + scheduler
func (e *Engine) Stop()                     // cancels and waits

func RunDelta(ctx context.Context, st *store.Store, client *githubapi.Client, repoID int64, now func() time.Time) error
```

**`internal/api` (package `api`) — changed signature + new HTTP endpoints (all auth-gated under `/api`):**
```go
func NewServer(cfg config.Config, st *store.Store, authSvc *auth.Service, engine *sync.Engine, cipher *crypto.Cipher) *Server

// POST   /api/repos                    body {"full_name":"owner/name"} -> 201 repoJSON
// GET    /api/repos                    -> 200 []repoJSON (incl. id, full_name, is_private, default_branch, sync_status, last_synced_at)
// DELETE /api/repos/{id}               -> 204
// POST   /api/repos/{id}/refresh       -> 202 (enqueues a delta job; 404 if untracked)
// GET    /api/repos/{id}/sync/stream   -> 200 text/event-stream of sync.Event frames (404 if untracked)
// (unknown /api/* -> JSON 404)
```
