package store

import (
	"context"
	"testing"
	"time"
)

func seedRepo(t *testing.T, s *Store) int64 {
	t.Helper()
	id, err := s.UpsertRepo(context.Background(), &Repo{
		GitHubID: 1, FullName: "a/b", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func ts(s string) time.Time {
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return v
}

func TestUpsertCommitsBatchIdempotent(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)

	batch := []Commit{
		{SHA: "a1", AuthorLogin: "neo", CommittedAt: ts("2026-01-02T10:00:00Z"), Additions: 5, Deletions: 1, MsgFirstLine: "first"},
		{SHA: "a2", AuthorLogin: "trinity", CommittedAt: ts("2026-01-02T11:00:00Z"), Additions: 3, Deletions: 0, IsBot: false},
	}
	if err := s.UpsertCommits(ctx, repoID, batch); err != nil {
		t.Fatal(err)
	}
	// Re-ingest the same page: must not duplicate.
	if err := s.UpsertCommits(ctx, repoID, batch); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM commits WHERE repo_id = ?`, repoID,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("commit count = %d, want 2", n)
	}
}

func TestUpsertPullRequestsBatch(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)

	mergedAt := ts("2026-01-05T12:00:00Z")
	reviewAt := ts("2026-01-04T09:00:00Z")
	batch := []PullRequest{
		{
			Number: 1, AuthorLogin: "neo", State: "MERGED",
			CreatedAt: ts("2026-01-03T08:00:00Z"), MergedAt: &mergedAt,
			Additions: 10, Deletions: 2, ChangedFiles: 3, CommentsCount: 4,
			FirstReviewAt: &reviewAt, Title: "add feature",
		},
		{
			Number: 2, AuthorLogin: "dependabot[bot]", State: "OPEN",
			CreatedAt: ts("2026-01-06T08:00:00Z"), IsBot: true, Title: "bump dep",
		},
	}
	if err := s.UpsertPullRequests(ctx, repoID, batch); err != nil {
		t.Fatal(err)
	}

	var state string
	var mergedNull bool
	row := s.DB.QueryRowContext(ctx,
		`SELECT state, merged_at IS NULL FROM pull_requests WHERE repo_id = ? AND number = 1`,
		repoID)
	if err := row.Scan(&state, &mergedNull); err != nil {
		t.Fatal(err)
	}
	if state != "MERGED" || mergedNull {
		t.Fatalf("PR1 state=%q mergedNull=%v", state, mergedNull)
	}

	// Updating PR2 to merged must overwrite, not duplicate.
	closedAt := ts("2026-01-07T08:00:00Z")
	if err := s.UpsertPullRequests(ctx, repoID, []PullRequest{{
		Number: 2, AuthorLogin: "dependabot[bot]", State: "CLOSED",
		CreatedAt: ts("2026-01-06T08:00:00Z"), ClosedAt: &closedAt, IsBot: true, Title: "bump dep",
	}}); err != nil {
		t.Fatal(err)
	}
	var n int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pull_requests WHERE repo_id = ?`, repoID).Scan(&n)
	if n != 2 {
		t.Fatalf("PR count = %d, want 2", n)
	}
	var newState string
	s.DB.QueryRowContext(ctx, `SELECT state FROM pull_requests WHERE repo_id=? AND number=2`, repoID).Scan(&newState)
	if newState != "CLOSED" {
		t.Fatalf("PR2 state = %q, want CLOSED", newState)
	}
}

func TestUpsertIssuesBatch(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)

	closedAt := ts("2026-01-10T12:00:00Z")
	batch := []Issue{
		{Number: 1, AuthorLogin: "neo", State: "CLOSED", CreatedAt: ts("2026-01-09T08:00:00Z"), ClosedAt: &closedAt, CommentsCount: 2, Title: "bug"},
		{Number: 2, AuthorLogin: "trinity", State: "OPEN", CreatedAt: ts("2026-01-11T08:00:00Z"), Title: "feature"},
	}
	if err := s.UpsertIssues(ctx, repoID, batch); err != nil {
		t.Fatal(err)
	}
	var n int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM issues WHERE repo_id = ?`, repoID).Scan(&n)
	if n != 2 {
		t.Fatalf("issue count = %d, want 2", n)
	}
}

func TestUpsertReleasesBatch(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)

	pub := ts("2026-01-12T12:00:00Z")
	batch := []Release{
		{Tag: "v1.0.0", Name: "First", PublishedAt: &pub, AuthorLogin: "neo"},
		{Tag: "v1.1.0", Name: "Second", PublishedAt: nil, AuthorLogin: "trinity"},
	}
	if err := s.UpsertReleases(ctx, repoID, batch); err != nil {
		t.Fatal(err)
	}
	var n int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM releases WHERE repo_id = ?`, repoID).Scan(&n)
	if n != 2 {
		t.Fatalf("release count = %d, want 2", n)
	}
}

func TestUpsertEmptyBatchIsNoop(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)
	if err := s.UpsertCommits(ctx, repoID, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPullRequests(ctx, repoID, []PullRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertIssues(ctx, repoID, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertReleases(ctx, repoID, nil); err != nil {
		t.Fatal(err)
	}
}
