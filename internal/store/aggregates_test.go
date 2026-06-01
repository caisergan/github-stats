package store

import (
	"context"
	"testing"
)

func TestRecomputeDailyRepoStats(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)

	// Two commits on the same day by two contributors, one the next day.
	if err := s.UpsertCommits(ctx, repoID, []Commit{
		{SHA: "c1", AuthorLogin: "neo", CommittedAt: ts("2026-03-01T08:00:00Z"), Additions: 10, Deletions: 2},
		{SHA: "c2", AuthorLogin: "trinity", CommittedAt: ts("2026-03-01T20:00:00Z"), Additions: 5, Deletions: 1},
		{SHA: "c3", AuthorLogin: "neo", CommittedAt: ts("2026-03-02T09:00:00Z"), Additions: 3, Deletions: 0},
	}); err != nil {
		t.Fatal(err)
	}
	merged := ts("2026-03-01T18:00:00Z")
	closed := ts("2026-03-02T10:00:00Z")
	if err := s.UpsertPullRequests(ctx, repoID, []PullRequest{
		{Number: 1, AuthorLogin: "neo", State: "MERGED", CreatedAt: ts("2026-03-01T07:00:00Z"), MergedAt: &merged, CommentsCount: 2},
		{Number: 2, AuthorLogin: "trinity", State: "CLOSED", CreatedAt: ts("2026-03-01T09:00:00Z"), ClosedAt: &closed, CommentsCount: 1},
	}); err != nil {
		t.Fatal(err)
	}
	issueClosed := ts("2026-03-02T11:00:00Z")
	if err := s.UpsertIssues(ctx, repoID, []Issue{
		{Number: 1, AuthorLogin: "neo", State: "CLOSED", CreatedAt: ts("2026-03-01T06:00:00Z"), ClosedAt: &issueClosed, CommentsCount: 4},
	}); err != nil {
		t.Fatal(err)
	}
	pub := ts("2026-03-01T12:00:00Z")
	if err := s.UpsertReleases(ctx, repoID, []Release{
		{Tag: "v1.0.0", Name: "First", PublishedAt: &pub, AuthorLogin: "neo"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.RecomputeDailyStats(ctx, repoID, "2026-03-01", "2026-03-02"); err != nil {
		t.Fatal(err)
	}

	// Day 1 repo stats.
	var commits, adds, dels, prsOpened, prsMerged, prsClosed, issuesOpened, issuesClosed, comments, releases, contribs int
	row := s.DB.QueryRowContext(ctx, `
		SELECT commits, additions, deletions, prs_opened, prs_merged, prs_closed,
			issues_opened, issues_closed, comments, releases, active_contributors
		FROM daily_repo_stats WHERE repo_id = ? AND date = '2026-03-01'`, repoID)
	if err := row.Scan(&commits, &adds, &dels, &prsOpened, &prsMerged, &prsClosed,
		&issuesOpened, &issuesClosed, &comments, &releases, &contribs); err != nil {
		t.Fatal(err)
	}
	if commits != 2 || adds != 15 || dels != 3 {
		t.Fatalf("day1 commit totals: commits=%d adds=%d dels=%d", commits, adds, dels)
	}
	if prsOpened != 2 || prsMerged != 1 || prsClosed != 0 {
		t.Fatalf("day1 PR totals: opened=%d merged=%d closed=%d", prsOpened, prsMerged, prsClosed)
	}
	if issuesOpened != 1 || issuesClosed != 0 {
		t.Fatalf("day1 issue totals: opened=%d closed=%d", issuesOpened, issuesClosed)
	}
	// comments = PR1(2) + PR2(1) on created date + issue1(4) on created date = 7 on day1.
	if comments != 7 {
		t.Fatalf("day1 comments = %d, want 7", comments)
	}
	if releases != 1 {
		t.Fatalf("day1 releases = %d, want 1", releases)
	}
	if contribs != 2 {
		t.Fatalf("day1 active_contributors = %d, want 2", contribs)
	}

	// Day 2: PR2 closed, issue1 closed, 1 commit by neo.
	row = s.DB.QueryRowContext(ctx, `
		SELECT commits, prs_closed, issues_closed, active_contributors
		FROM daily_repo_stats WHERE repo_id = ? AND date = '2026-03-02'`, repoID)
	var c2, prClosed2, issClosed2, contribs2 int
	if err := row.Scan(&c2, &prClosed2, &issClosed2, &contribs2); err != nil {
		t.Fatal(err)
	}
	if c2 != 1 || prClosed2 != 1 || issClosed2 != 1 || contribs2 != 1 {
		t.Fatalf("day2: commits=%d prClosed=%d issClosed=%d contribs=%d", c2, prClosed2, issClosed2, contribs2)
	}
}

func TestRecomputeDailyContributorStats(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)

	if err := s.UpsertCommits(ctx, repoID, []Commit{
		{SHA: "c1", AuthorLogin: "neo", CommittedAt: ts("2026-03-01T08:00:00Z"), Additions: 10, Deletions: 2},
		{SHA: "c2", AuthorLogin: "neo", CommittedAt: ts("2026-03-01T20:00:00Z"), Additions: 5, Deletions: 1},
		{SHA: "c3", AuthorLogin: "trinity", CommittedAt: ts("2026-03-01T21:00:00Z"), Additions: 4, Deletions: 0},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecomputeDailyStats(ctx, repoID, "2026-03-01", "2026-03-01"); err != nil {
		t.Fatal(err)
	}

	var commits, adds, dels int
	row := s.DB.QueryRowContext(ctx, `
		SELECT commits, additions, deletions FROM daily_contributor_stats
		WHERE repo_id = ? AND date = '2026-03-01' AND login = 'neo'`, repoID)
	if err := row.Scan(&commits, &adds, &dels); err != nil {
		t.Fatal(err)
	}
	if commits != 2 || adds != 15 || dels != 3 {
		t.Fatalf("neo contributor stats: commits=%d adds=%d dels=%d", commits, adds, dels)
	}

	var n int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM daily_contributor_stats WHERE repo_id = ?`, repoID).Scan(&n)
	if n != 2 {
		t.Fatalf("contributor rows = %d, want 2 (neo, trinity)", n)
	}
}

func TestRecomputeIsIdempotentAndScoped(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)

	if err := s.UpsertCommits(ctx, repoID, []Commit{
		{SHA: "c1", AuthorLogin: "neo", CommittedAt: ts("2026-03-01T08:00:00Z"), Additions: 1},
	}); err != nil {
		t.Fatal(err)
	}
	// Recompute twice — must not double count.
	if err := s.RecomputeDailyStats(ctx, repoID, "2026-03-01", "2026-03-01"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecomputeDailyStats(ctx, repoID, "2026-03-01", "2026-03-01"); err != nil {
		t.Fatal(err)
	}
	var commits int
	s.DB.QueryRowContext(ctx, `SELECT commits FROM daily_repo_stats WHERE repo_id=? AND date='2026-03-01'`, repoID).Scan(&commits)
	if commits != 1 {
		t.Fatalf("commits after double recompute = %d, want 1", commits)
	}

	// A recompute window that excludes the event date wipes its aggregate row.
	if err := s.RecomputeDailyStats(ctx, repoID, "2026-03-05", "2026-03-06"); err != nil {
		t.Fatal(err)
	}
	var stillThere int
	s.DB.QueryRowContext(ctx, `SELECT commits FROM daily_repo_stats WHERE repo_id=? AND date='2026-03-01'`, repoID).Scan(&stillThere)
	if stillThere != 1 {
		t.Fatalf("out-of-window recompute must not touch 2026-03-01, got commits=%d", stillThere)
	}
}
