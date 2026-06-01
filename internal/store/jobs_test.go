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
	if err := s.CompleteJob(ctx, id); err != nil {
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
