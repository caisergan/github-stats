package store

import (
	"context"
	"testing"
	"time"
)

func TestZZProbeTimestampStorage(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)

	created := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
	merged := time.Date(2026, 3, 1, 18, 0, 0, 0, time.UTC)
	if err := s.UpsertPullRequests(ctx, repoID, []PullRequest{
		{Number: 1, AuthorLogin: "neo", State: "MERGED", CreatedAt: created, MergedAt: &merged},
	}); err != nil {
		t.Fatal(err)
	}

	// 1. What is the raw stored text?
	var raw string
	s.DB.QueryRowContext(ctx, `SELECT created_at FROM pull_requests WHERE repo_id=? AND number=1`, repoID).Scan(&raw)
	t.Logf("RAW created_at stored = %q", raw)
	var rawMerged string
	s.DB.QueryRowContext(ctx, `SELECT merged_at FROM pull_requests WHERE repo_id=? AND number=1`, repoID).Scan(&rawMerged)
	t.Logf("RAW merged_at stored = %q", rawMerged)

	// 2. Does date() parse it?
	var d interface{}
	s.DB.QueryRowContext(ctx, `SELECT date(created_at) FROM pull_requests WHERE repo_id=? AND number=1`, repoID).Scan(&d)
	t.Logf("date(created_at) = %v (nil means NULL)", d)

	// 3. Does substr work?
	var sub string
	s.DB.QueryRowContext(ctx, `SELECT substr(created_at,1,10) FROM pull_requests WHERE repo_id=? AND number=1`, repoID).Scan(&sub)
	t.Logf("substr(created_at,1,10) = %q", sub)

	// 4. julianday?
	var jd interface{}
	s.DB.QueryRowContext(ctx, `SELECT julianday(created_at) FROM pull_requests WHERE repo_id=? AND number=1`, repoID).Scan(&jd)
	t.Logf("julianday(created_at) = %v (nil means NULL)", jd)

	// 5. Now the M4 binding pattern: bind a Go time.Time to created_at <= ?
	asOf := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	var cnt int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pull_requests WHERE repo_id=? AND created_at <= ?`, repoID, asOf).Scan(&cnt)
	t.Logf("M4 pattern: COUNT created_at <= asOf(2026-03-02) = %d (want 1)", cnt)

	asOfBefore := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pull_requests WHERE repo_id=? AND created_at <= ?`, repoID, asOfBefore).Scan(&cnt)
	t.Logf("M4 pattern: COUNT created_at <= asOf(2026-02-01) = %d (want 0)", cnt)

	// 6. M4 closed_at > ? pattern with pointer
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pull_requests WHERE repo_id=? AND (merged_at IS NULL OR merged_at > ?)`, repoID, asOf).Scan(&cnt)
	t.Logf("M4 open-as-of pattern (merged_at > 2026-03-02) = %d", cnt)

	// 7. ORDER BY created_at lexicographic check: insert a second row a year later and earlier, confirm order
	c2 := time.Date(2025, 12, 31, 23, 0, 0, 0, time.UTC)
	c3 := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	m2 := c2
	s.UpsertPullRequests(ctx, repoID, []PullRequest{
		{Number: 2, AuthorLogin: "x", State: "MERGED", CreatedAt: c2, MergedAt: &m2},
		{Number: 3, AuthorLogin: "y", State: "OPEN", CreatedAt: c3},
	})
	rows, _ := s.DB.QueryContext(ctx, `SELECT number FROM pull_requests WHERE repo_id=? ORDER BY created_at ASC`, repoID)
	var order []int
	for rows.Next() {
		var n int
		rows.Scan(&n)
		order = append(order, n)
	}
	rows.Close()
	t.Logf("ORDER BY created_at ASC -> %v (want [2 1 3] = 2025,2026,2027)", order)

	// 8. M4 date(merged_at) range with substr-derived fromDate/toDate strings
	var mcnt int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pull_requests WHERE repo_id=? AND merged_at IS NOT NULL AND date(merged_at) >= ? AND date(merged_at) <= ?`, repoID, "2026-03-01", "2026-03-01").Scan(&mcnt)
	t.Logf("M4 MergedPRDurations pattern date(merged_at) in 2026-03-01 = %d (want 1)", mcnt)

	// 9. EarliestEventDate pattern: MIN(date(created_at))
	var mind interface{}
	s.DB.QueryRowContext(ctx, `SELECT MIN(date(created_at)) FROM pull_requests WHERE repo_id=?`, repoID).Scan(&mind)
	t.Logf("M4 EarliestEventDate MIN(date(created_at)) = %v (want 2025-12-31)", mind)
}
