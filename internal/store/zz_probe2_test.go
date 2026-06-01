package store

import (
	"context"
	"testing"
	"time"
)

func TestZZProbe2(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)

	// Non-UTC time: does the driver convert to UTC or store with offset?
	loc := time.FixedZone("PST", -8*3600)
	localTime := time.Date(2026, 3, 1, 23, 30, 0, 0, loc) // = 2026-03-02T07:30Z
	if err := s.UpsertCommits(ctx, repoID, []Commit{
		{SHA: "loc1", AuthorLogin: "neo", CommittedAt: localTime, Additions: 1},
	}); err != nil {
		t.Fatal(err)
	}
	var raw string
	s.DB.QueryRowContext(ctx, `SELECT committed_at FROM commits WHERE sha='loc1'`).Scan(&raw)
	t.Logf("non-UTC committed_at RAW = %q", raw)
	var sub string
	s.DB.QueryRowContext(ctx, `SELECT substr(committed_at,1,10) FROM commits WHERE sha='loc1'`).Scan(&sub)
	t.Logf("non-UTC substr day = %q  (UTC day should be 2026-03-02)", sub)

	// Fractional seconds / nanoseconds: GitHub RFC3339 has no fractions, but check
	nano := time.Date(2026, 3, 1, 8, 0, 0, 123456789, time.UTC)
	s.UpsertCommits(ctx, repoID, []Commit{{SHA: "nano1", AuthorLogin: "x", CommittedAt: nano}})
	s.DB.QueryRowContext(ctx, `SELECT committed_at FROM commits WHERE sha='nano1'`).Scan(&raw)
	t.Logf("nanosecond committed_at RAW = %q", raw)
	s.DB.QueryRowContext(ctx, `SELECT substr(committed_at,1,10) FROM commits WHERE sha='nano1'`).Scan(&sub)
	t.Logf("nanosecond substr day = %q", sub)

	// Empty-string author (null GraphQL author) attribution
	s.UpsertCommits(ctx, repoID, []Commit{
		{SHA: "anon1", AuthorLogin: "", CommittedAt: time.Date(2026,3,3,0,0,0,0,time.UTC)},
		{SHA: "anon2", AuthorLogin: "", CommittedAt: time.Date(2026,3,3,1,0,0,0,time.UTC)},
	})
	if err := s.RecomputeDailyStats(ctx, repoID, "2026-03-01", "2026-03-31"); err != nil {
		t.Fatal(err)
	}
	var ac int
	s.DB.QueryRowContext(ctx, `SELECT active_contributors FROM daily_repo_stats WHERE repo_id=? AND date='2026-03-03'`, repoID).Scan(&ac)
	t.Logf("active_contributors on day with 2 empty-login commits = %d (COUNT DISTINCT '' = 1)", ac)
	var contribRows int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM daily_contributor_stats WHERE repo_id=? AND date='2026-03-03'`, repoID).Scan(&contribRows)
	t.Logf("daily_contributor_stats rows for empty login day = %d", contribRows)
}
