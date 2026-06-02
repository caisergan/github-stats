package store

import (
	"context"
	"testing"
)

// seedReadFixture inserts a small, deterministic dataset and recomputes
// aggregates so the read methods have both event rows and daily rows.
func seedReadFixture(t *testing.T, s *Store) int64 {
	t.Helper()
	ctx := context.Background()
	repoID := seedRepo(t, s)

	if err := s.UpsertCommits(ctx, repoID, []Commit{
		{SHA: "c1", AuthorLogin: "neo", CommittedAt: ts("2026-03-01T08:00:00Z"), Additions: 10, Deletions: 2},
		{SHA: "c2", AuthorLogin: "trinity", CommittedAt: ts("2026-03-01T20:00:00Z"), Additions: 5, Deletions: 1},
		{SHA: "c3", AuthorLogin: "neo", CommittedAt: ts("2026-03-02T09:00:00Z"), Additions: 3, Deletions: 0},
		{SHA: "c4", AuthorLogin: "dependabot[bot]", CommittedAt: ts("2026-03-02T10:00:00Z"), Additions: 7, Deletions: 1, IsBot: true},
	}); err != nil {
		t.Fatal(err)
	}
	merged1 := ts("2026-03-01T18:00:00Z")
	review1 := ts("2026-03-01T10:00:00Z")
	merged2 := ts("2026-03-03T12:00:00Z")
	closed3 := ts("2026-03-02T08:00:00Z")
	if err := s.UpsertPullRequests(ctx, repoID, []PullRequest{
		{Number: 1, AuthorLogin: "neo", State: "MERGED", CreatedAt: ts("2026-03-01T06:00:00Z"), MergedAt: &merged1, FirstReviewAt: &review1, Additions: 20, Deletions: 4, CommentsCount: 3, Title: "feature"},
		{Number: 2, AuthorLogin: "trinity", State: "MERGED", CreatedAt: ts("2026-03-02T06:00:00Z"), MergedAt: &merged2, Additions: 8, Deletions: 1, CommentsCount: 1, Title: "fix"},
		{Number: 3, AuthorLogin: "dependabot[bot]", State: "CLOSED", CreatedAt: ts("2026-03-01T07:00:00Z"), ClosedAt: &closed3, IsBot: true, Title: "bump"},
	}); err != nil {
		t.Fatal(err)
	}
	issClosed1 := ts("2026-03-04T12:00:00Z")
	if err := s.UpsertIssues(ctx, repoID, []Issue{
		{Number: 1, AuthorLogin: "neo", State: "CLOSED", CreatedAt: ts("2026-03-01T06:00:00Z"), ClosedAt: &issClosed1, CommentsCount: 2, Title: "bug"},
		{Number: 2, AuthorLogin: "trinity", State: "OPEN", CreatedAt: ts("2026-03-02T06:00:00Z"), CommentsCount: 0, Title: "open one"},
		{Number: 3, AuthorLogin: "dependabot[bot]", State: "OPEN", CreatedAt: ts("2026-02-01T06:00:00Z"), IsBot: true, Title: "old bot issue"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecomputeDailyStats(ctx, repoID, "2026-03-01", "2026-03-04"); err != nil {
		t.Fatal(err)
	}
	return repoID
}

func TestDailyRepoStatsRange(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedReadFixture(t, s)

	rows, err := s.DailyRepoStats(ctx, repoID, "2026-03-01", "2026-03-02")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].Date != "2026-03-01" || rows[1].Date != "2026-03-02" {
		t.Fatalf("rows not ordered by date: %+v", rows)
	}
	// Day 1: 2 commits (c1,c2); day 2: 2 commits (c3 + bot c4 — aggregates do not exclude bots).
	if rows[0].Commits != 2 || rows[1].Commits != 2 {
		t.Fatalf("commit counts: %d, %d", rows[0].Commits, rows[1].Commits)
	}
	if rows[0].Additions != 15 || rows[0].Deletions != 3 {
		t.Fatalf("day1 churn: adds=%d dels=%d", rows[0].Additions, rows[0].Deletions)
	}
}

func TestDailyContributorStatsRange(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedReadFixture(t, s)

	rows, err := s.DailyContributorStats(ctx, repoID, "2026-03-01", "2026-03-02")
	if err != nil {
		t.Fatal(err)
	}
	var neoCommits int64
	for _, r := range rows {
		if r.Login == "neo" {
			neoCommits += r.Commits
		}
	}
	if neoCommits != 2 {
		t.Fatalf("neo commits across window = %d, want 2", neoCommits)
	}
}

func TestMergedPRDurationsExcludeBots(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedReadFixture(t, s)

	// PR1 merged inside window; PR2 merged 2026-03-03 (in window). Bot PR3 was CLOSED not merged.
	all, err := s.MergedPRDurations(ctx, repoID, "2026-03-01", "2026-03-04", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("merged PRs (incl bots) = %d, want 2", len(all))
	}
	// PR1: created 03-01T06 → merged 03-01T18 = 12h = 43200s.
	if all[0].CreatedAt.UTC() != ts("2026-03-01T06:00:00Z") {
		t.Fatalf("first merged PR created = %v", all[0].CreatedAt)
	}
	// exclude_bots removes nothing here (both merged PRs are human), so count stays 2.
	human, err := s.MergedPRDurations(ctx, repoID, "2026-03-01", "2026-03-04", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(human) != 2 {
		t.Fatalf("merged PRs (excl bots) = %d, want 2", len(human))
	}
}

func TestReviewLatencies(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedReadFixture(t, s)

	rows, err := s.ReviewLatencies(ctx, repoID, "2026-03-01", "2026-03-04", false)
	if err != nil {
		t.Fatal(err)
	}
	// Only PR1 has a first_review_at; PR2 has none → excluded.
	if len(rows) != 1 {
		t.Fatalf("review latency rows = %d, want 1", len(rows))
	}
	if !rows[0].CreatedAt.UTC().Equal(ts("2026-03-01T06:00:00Z")) || !rows[0].FirstReviewAt.UTC().Equal(ts("2026-03-01T10:00:00Z")) {
		t.Fatalf("review latency row mismatch: %+v", rows[0])
	}
}

func TestClosedIssueLifetimes(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedReadFixture(t, s)

	rows, err := s.ClosedIssueLifetimes(ctx, repoID, "2026-03-01", "2026-03-05", false)
	if err != nil {
		t.Fatal(err)
	}
	// Issue1 closed 2026-03-04 inside window; issues 2 & 3 are OPEN → excluded.
	if len(rows) != 1 {
		t.Fatalf("closed issue rows = %d, want 1", len(rows))
	}
}

func TestOpenIssuesAsOf(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedReadFixture(t, s)

	asOf := ts("2026-03-05T00:00:00Z")
	rows, err := s.OpenIssuesAsOf(ctx, repoID, asOf, false)
	if err != nil {
		t.Fatal(err)
	}
	// As of 2026-03-05: issue1 closed (03-04) so not open; issue2 (human) open; issue3 (bot) open.
	if len(rows) != 2 {
		t.Fatalf("open issues (incl bots) = %d, want 2", len(rows))
	}
	human, err := s.OpenIssuesAsOf(ctx, repoID, asOf, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(human) != 1 {
		t.Fatalf("open issues (excl bots) = %d, want 1", len(human))
	}
}

func TestLatestCommitsPRsIssues(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedReadFixture(t, s)

	commits, err := s.LatestCommits(ctx, repoID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("latest commits = %d, want 2", len(commits))
	}
	// Newest first: c4 (2026-03-02T10) then c3 (2026-03-02T09).
	if commits[0].SHA != "c4" || commits[1].SHA != "c3" {
		t.Fatalf("latest commits order: %s, %s", commits[0].SHA, commits[1].SHA)
	}

	prs, err := s.LatestPRs(ctx, repoID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 3 || prs[0].Number != 2 {
		t.Fatalf("latest PRs: n=%d first=%d", len(prs), func() int64 { if len(prs) > 0 { return prs[0].Number }; return -1 }())
	}

	issues, err := s.LatestIssues(ctx, repoID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 3 || issues[0].Number != 2 {
		t.Fatalf("latest issues: n=%d first=%d", len(issues), func() int64 { if len(issues) > 0 { return issues[0].Number }; return -1 }())
	}
}

func TestEarliestEventDate(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedReadFixture(t, s)

	d, err := s.EarliestEventDate(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	// Earliest of all events is bot issue3 created 2026-02-01.
	if d != "2026-02-01" {
		t.Fatalf("earliest date = %q, want 2026-02-01", d)
	}
}

func TestCountsForOverview(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedReadFixture(t, s)

	asOf := ts("2026-03-05T00:00:00Z")
	openIssues, err := s.CountOpenIssues(ctx, repoID, asOf, false)
	if err != nil {
		t.Fatal(err)
	}
	if openIssues != 2 {
		t.Fatalf("open issues count = %d, want 2", openIssues)
	}
	openPRs, err := s.CountOpenPRs(ctx, repoID, asOf, false)
	if err != nil {
		t.Fatal(err)
	}
	// All 3 PRs are MERGED/CLOSED as of 03-05 → 0 open.
	if openPRs != 0 {
		t.Fatalf("open PRs count = %d, want 0", openPRs)
	}
	contribs, err := s.CountContributors(ctx, repoID, "2026-03-01", "2026-03-04", true)
	if err != nil {
		t.Fatal(err)
	}
	// Distinct human commit authors in window: neo, trinity (dependabot excluded).
	if contribs != 2 {
		t.Fatalf("contributors (excl bots) = %d, want 2", contribs)
	}
	releases, err := s.CountReleases(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if releases != 0 {
		t.Fatalf("releases = %d, want 0", releases)
	}
}
