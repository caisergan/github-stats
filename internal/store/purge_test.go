package store

import (
	"context"
	"testing"
)

func TestPurgeRepoHardDeletesAllData(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	uid := seedUser(t, s)
	repoID := seedRepo(t, s) // full_name "a/b"

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.DB.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("seed exec failed (%s): %v", q, err)
		}
	}
	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := s.DB.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
			t.Fatalf("count failed (%s): %v", q, err)
		}
		return n
	}

	if err := s.TrackRepo(ctx, uid, repoID); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO commits(repo_id, sha, committed_at) VALUES (?, 'sha1', '2026-01-01T00:00:00Z')`, repoID)
	exec(`INSERT INTO pull_requests(repo_id, number, state, created_at) VALUES (?, 1, 'open', '2026-01-01T00:00:00Z')`, repoID)
	exec(`INSERT INTO issues(repo_id, number, state, created_at) VALUES (?, 1, 'open', '2026-01-01T00:00:00Z')`, repoID)
	exec(`INSERT INTO sync_state(repo_id, status) VALUES (?, 'complete')`, repoID)
	exec(`INSERT INTO sync_jobs(repo_id, kind) VALUES (?, 'backfill')`, repoID)
	exec(`INSERT INTO etags(url, etag, status, body) VALUES ('https://api.github.com/repos/a/b/commits', 'e1', 200, x'00')`)

	if err := s.PurgeRepo(ctx, repoID); err != nil {
		t.Fatal(err)
	}

	if _, err := s.GetRepoByFullName(ctx, "a/b"); err != ErrNotFound {
		t.Fatalf("repo still present after purge: %v", err)
	}
	for _, tbl := range []string{"commits", "pull_requests", "issues", "sync_state", "sync_jobs", "repo_tracking"} {
		if n := count("SELECT COUNT(*) FROM "+tbl+" WHERE repo_id = ?", repoID); n != 0 {
			t.Fatalf("%s not purged: %d rows remain", tbl, n)
		}
	}
	if n := count(`SELECT COUNT(*) FROM etags WHERE url LIKE '%/repos/a/b/%'`); n != 0 {
		t.Fatalf("etags not purged: %d rows remain", n)
	}

	// Idempotent: purging an already-deleted repo is a no-op.
	if err := s.PurgeRepo(ctx, repoID); err != nil {
		t.Fatalf("second purge errored: %v", err)
	}
}
