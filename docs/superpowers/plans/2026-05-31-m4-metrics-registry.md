# M4 — Metrics Registry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the **modular statistics generator** (spec §7) — a `metrics` package of self-contained, independently testable stat units that read **only** from the store, plus the auth-gated read endpoints (spec §10) that serve them as JSON. This delivers: a narrow read-only `Source` port that `*store.Store` satisfies; a `Window`/`Opts`/`Result` vocabulary; a `Registry` that maps `key → Metric` and runs requested metrics; the Extended metric set (`commit_rate`, `pr_throughput`, `time_to_merge`, `review_latency`, `issue_lifetime`, `open_issue_age`, `code_churn`, `comment_volume`, `contributor_leaderboard`) each in its own file plus an `ema` smoothing helper; the new `store` read methods those metrics need; and three HTTP endpoints — `GET /api/repos/{id}/metrics`, `GET /api/repos/{id}` (overview bundle), and `GET /api/repos/{id}/latest/{commits|prs|issues}`. M4 is exercised entirely by Go tests (unit tests against a fake `Source`; HTTP integration tests against a real `openTemp` store). The React dashboard that renders these is **M5**.

**Architecture:** Builds directly on M1 (`docs/superpowers/plans/2026-05-30-m1-skeleton-and-auth.md`), M2 (`docs/superpowers/plans/2026-05-31-m2-collector-and-backfill.md`), and M3 (`docs/superpowers/plans/2026-05-31-m3-sync-engine.md`). Spec §4 boundaries are enforced by construction: `metrics` reads **only** through a `Source` interface (never GitHub, never HTTP), so every metric is unit-testable against a hand-built fake `Source` with deterministic rows and an injected `now`. Chart metrics read the **precomputed daily aggregates** (`daily_repo_stats`, `daily_contributor_stats`) that M2 materializes and M3 keeps fresh — O(days), not O(events). Duration metrics (`time_to_merge`, `review_latency`, `issue_lifetime`) and the `open_issue_age` distribution read the **event tables** (`pull_requests`, `issues`) directly. `ExcludeBots` filters on the `is_bot` flag M2 already sets. The HTTP layer builds the `Registry` once inside `NewServer` from the store (the `*store.Store` is passed as the `Source`); **the `NewServer` signature is unchanged** (M3's `NewServer(cfg, st, authSvc, engine, cipher)` already carries everything needed). New routes mount in the **same auth-gated `/api` group** M3 established and reuse M3's `IsTracked` for authorization, M3's `writeJSON`/`repoIDParam`/`splitFullName` helpers, and M1's `auth.UserFromContext`.

**Tech Stack:** Go 1.25+, `modernc.org/sqlite` (driver `"sqlite"`, WAL, `db.SetMaxOpenConns(1)` — inherited), Go stdlib `net/http` + `encoding/json` + `time` + `math`, `github.com/go-chi/chi/v5` (router + `URLParam`), `net/http/httptest` for endpoint tests. No new third-party dependencies are required.

---

## File Structure

```
github-stats/
├── internal/
│   ├── store/
│   │   ├── reads.go                          # NEW: read methods backing the metrics Source port
│   │   └── reads_test.go                     # NEW: read-method tests against openTemp
│   ├── metrics/                              # NEW package: the modular statistics generator
│   │   ├── source.go                         # Source port interface + row structs
│   │   ├── window.go                         # Window type + Parse(window, now); Opts
│   │   ├── window_test.go
│   │   ├── result.go                         # Result + ResultKind + constructors (time-series/scalar/buckets/leaderboard)
│   │   ├── result_test.go
│   │   ├── registry.go                       # Metric interface + Registry + Compute
│   │   ├── registry_test.go
│   │   ├── fakesource_test.go                # fakeSource: in-memory Source for unit tests
│   │   ├── ema.go                            # EMA helper (5d/14d smoothing over a daily series)
│   │   ├── ema_test.go
│   │   ├── commit_rate.go                    # commit_rate metric (time-series)
│   │   ├── commit_rate_test.go
│   │   ├── pr_throughput.go                  # pr_throughput metric (time-series, opened/merged/closed)
│   │   ├── pr_throughput_test.go
│   │   ├── time_to_merge.go                  # time_to_merge metric (scalar: avg/median hours)
│   │   ├── time_to_merge_test.go
│   │   ├── review_latency.go                 # review_latency metric (scalar: avg/median hours)
│   │   ├── review_latency_test.go
│   │   ├── issue_lifetime.go                 # issue_lifetime metric (scalar: avg/median hours)
│   │   ├── issue_lifetime_test.go
│   │   ├── open_issue_age.go                 # open_issue_age metric (buckets <24h/7d/30d/90d/180d/older)
│   │   ├── open_issue_age_test.go
│   │   ├── code_churn.go                     # code_churn metric (time-series: additions+deletions)
│   │   ├── code_churn_test.go
│   │   ├── comment_volume.go                 # comment_volume metric (time-series)
│   │   ├── comment_volume_test.go
│   │   ├── contributor_leaderboard.go        # contributor_leaderboard metric (leaderboard rows)
│   │   ├── contributor_leaderboard_test.go
│   │   ├── default.go                        # DefaultRegistry(): registers every shipped metric
│   │   └── default_test.go
│   └── api/
│       ├── metrics.go                        # NEW: GET /api/repos/{id}/metrics, /api/repos/{id} overview, /latest/{kind}
│       ├── metrics_test.go                   # NEW: HTTP integration tests (window, exclude_bots, 403/404)
│       └── server.go                         # MODIFIED: build registry; mount the three read routes
```

> All files under `internal/store/` join the **existing** `package store`; all files under `internal/metrics/` are the new `package metrics`; `internal/api/` joins the existing `package api`. The M1 `openTemp(t)` helper in `internal/store/store_test.go` is reused by `reads_test.go`. The `metrics` package's unit tests use a local `fakeSource` (defined in `fakesource_test.go`); the API integration tests use a real `store.Store` via `store.Open(t.TempDir()+...)`. **`NewServer`'s signature is unchanged from M3** — the registry is constructed from the already-injected `*store.Store`.

---

## Task 1: Store read methods — the data the metrics Source needs

**Files:**
- Create: `internal/store/reads.go`, `internal/store/reads_test.go`

These methods back the `metrics.Source` port (Task 2). They return **plain rows** (no business logic — that lives in the metrics) and keep SQL parameterized. Chart metrics read `daily_repo_stats`/`daily_contributor_stats` (O(days)); duration metrics and the open-issue-age distribution read the event tables. `ExcludeBots` is threaded as a boolean that, when true, adds `AND is_bot = 0` to the event-table queries. `EarliestEventDate` backs the `"all"` window — it returns the earliest UTC date across commits/PRs/issues so `Window.Parse` can resolve `"all"` to a concrete `[from,to]`.

- [ ] **Step 1: Write the failing test**

`internal/store/reads_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestDailyRepoStatsRange|TestDailyContributorStatsRange|TestMergedPRDurations|TestReviewLatencies|TestClosedIssueLifetimes|TestOpenIssuesAsOf|TestLatestCommitsPRsIssues|TestEarliestEventDate|TestCountsForOverview' -v`
Expected: FAIL — `undefined: (*Store).DailyRepoStats` (and the other new methods).

- [ ] **Step 3: Write minimal implementation**

`internal/store/reads.go`:
```go
package store

import (
	"context"
	"database/sql"
	"time"
)

// DailyRepoStatsRow is one row of daily_repo_stats for a date.
type DailyRepoStatsRow struct {
	Date          string // 'YYYY-MM-DD' (UTC)
	Commits       int64
	Additions     int64
	Deletions     int64
	PRsOpened     int64
	PRsMerged     int64
	PRsClosed     int64
	IssuesOpened  int64
	IssuesClosed  int64
	Comments      int64
	Releases      int64
	ActiveContrib int64
}

// DailyContribRow is one row of daily_contributor_stats for a (date, login).
type DailyContribRow struct {
	Date      string
	Login     string
	Commits   int64
	Additions int64
	Deletions int64
}

// PRDurationRow holds the timestamps a duration metric needs for one PR.
type PRDurationRow struct {
	Number    int64
	CreatedAt time.Time
	MergedAt  time.Time
}

// ReviewLatencyRow holds the timestamps a review-latency metric needs.
type ReviewLatencyRow struct {
	Number        int64
	CreatedAt     time.Time
	FirstReviewAt time.Time
}

// IssueLifetimeRow holds the timestamps an issue-lifetime metric needs.
type IssueLifetimeRow struct {
	Number    int64
	CreatedAt time.Time
	ClosedAt  time.Time
}

// OpenIssueRow holds the data the open-issue-age distribution needs.
type OpenIssueRow struct {
	Number    int64
	CreatedAt time.Time
}

// DailyRepoStats returns daily_repo_stats rows in [fromDate, toDate], ordered by date.
func (s *Store) DailyRepoStats(ctx context.Context, repoID int64, fromDate, toDate string) ([]DailyRepoStatsRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT date, commits, additions, deletions, prs_opened, prs_merged, prs_closed,
			issues_opened, issues_closed, comments, releases, active_contributors
		FROM daily_repo_stats
		WHERE repo_id = ? AND date >= ? AND date <= ?
		ORDER BY date`, repoID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DailyRepoStatsRow
	for rows.Next() {
		var r DailyRepoStatsRow
		if err := rows.Scan(&r.Date, &r.Commits, &r.Additions, &r.Deletions,
			&r.PRsOpened, &r.PRsMerged, &r.PRsClosed, &r.IssuesOpened, &r.IssuesClosed,
			&r.Comments, &r.Releases, &r.ActiveContrib); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DailyContributorStats returns daily_contributor_stats rows in [fromDate, toDate].
func (s *Store) DailyContributorStats(ctx context.Context, repoID int64, fromDate, toDate string) ([]DailyContribRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT date, login, commits, additions, deletions
		FROM daily_contributor_stats
		WHERE repo_id = ? AND date >= ? AND date <= ?
		ORDER BY date, login`, repoID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DailyContribRow
	for rows.Next() {
		var r DailyContribRow
		if err := rows.Scan(&r.Date, &r.Login, &r.Commits, &r.Additions, &r.Deletions); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// botFilter returns the SQL fragment that drops bot rows when excludeBots is set.
func botFilter(excludeBots bool) string {
	if excludeBots {
		return " AND is_bot = 0"
	}
	return ""
}

// MergedPRDurations returns created/merged timestamps for PRs merged in [from, to].
func (s *Store) MergedPRDurations(ctx context.Context, repoID int64, fromDate, toDate string, excludeBots bool) ([]PRDurationRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT number, created_at, merged_at
		FROM pull_requests
		WHERE repo_id = ? AND merged_at IS NOT NULL
			AND date(merged_at) >= ? AND date(merged_at) <= ?`+botFilter(excludeBots)+`
		ORDER BY merged_at`, repoID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PRDurationRow
	for rows.Next() {
		var r PRDurationRow
		if err := rows.Scan(&r.Number, &r.CreatedAt, &r.MergedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReviewLatencies returns created/first-review timestamps for PRs whose first
// review landed in [from, to].
func (s *Store) ReviewLatencies(ctx context.Context, repoID int64, fromDate, toDate string, excludeBots bool) ([]ReviewLatencyRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT number, created_at, first_review_at
		FROM pull_requests
		WHERE repo_id = ? AND first_review_at IS NOT NULL
			AND date(first_review_at) >= ? AND date(first_review_at) <= ?`+botFilter(excludeBots)+`
		ORDER BY first_review_at`, repoID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReviewLatencyRow
	for rows.Next() {
		var r ReviewLatencyRow
		if err := rows.Scan(&r.Number, &r.CreatedAt, &r.FirstReviewAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ClosedIssueLifetimes returns created/closed timestamps for issues closed in [from, to].
func (s *Store) ClosedIssueLifetimes(ctx context.Context, repoID int64, fromDate, toDate string, excludeBots bool) ([]IssueLifetimeRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT number, created_at, closed_at
		FROM issues
		WHERE repo_id = ? AND closed_at IS NOT NULL
			AND date(closed_at) >= ? AND date(closed_at) <= ?`+botFilter(excludeBots)+`
		ORDER BY closed_at`, repoID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IssueLifetimeRow
	for rows.Next() {
		var r IssueLifetimeRow
		if err := rows.Scan(&r.Number, &r.CreatedAt, &r.ClosedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// OpenIssuesAsOf returns issues open at asOf: created on/before asOf and either
// never closed or closed after asOf.
func (s *Store) OpenIssuesAsOf(ctx context.Context, repoID int64, asOf time.Time, excludeBots bool) ([]OpenIssueRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT number, created_at
		FROM issues
		WHERE repo_id = ? AND created_at <= ?
			AND (closed_at IS NULL OR closed_at > ?)`+botFilter(excludeBots)+`
		ORDER BY created_at`, repoID, asOf, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OpenIssueRow
	for rows.Next() {
		var r OpenIssueRow
		if err := rows.Scan(&r.Number, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LatestCommits returns the newest commits for a repo (newest first).
func (s *Store) LatestCommits(ctx context.Context, repoID int64, limit int) ([]Commit, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT sha, author_login, committed_at, additions, deletions, is_bot, msg_first_line
		FROM commits WHERE repo_id = ?
		ORDER BY committed_at DESC LIMIT ?`, repoID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Commit
	for rows.Next() {
		var c Commit
		var bot int
		if err := rows.Scan(&c.SHA, &c.AuthorLogin, &c.CommittedAt, &c.Additions, &c.Deletions, &bot, &c.MsgFirstLine); err != nil {
			return nil, err
		}
		c.IsBot = bot != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// LatestPRs returns the newest pull requests for a repo (newest created first).
func (s *Store) LatestPRs(ctx context.Context, repoID int64, limit int) ([]PullRequest, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT number, author_login, state, created_at, merged_at, closed_at,
			additions, deletions, changed_files, comments_count, first_review_at, is_bot, title
		FROM pull_requests WHERE repo_id = ?
		ORDER BY created_at DESC LIMIT ?`, repoID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PullRequest
	for rows.Next() {
		var p PullRequest
		var bot int
		if err := rows.Scan(&p.Number, &p.AuthorLogin, &p.State, &p.CreatedAt, &p.MergedAt, &p.ClosedAt,
			&p.Additions, &p.Deletions, &p.ChangedFiles, &p.CommentsCount, &p.FirstReviewAt, &bot, &p.Title); err != nil {
			return nil, err
		}
		p.IsBot = bot != 0
		out = append(out, p)
	}
	return out, rows.Err()
}

// LatestIssues returns the newest issues for a repo (newest created first).
func (s *Store) LatestIssues(ctx context.Context, repoID int64, limit int) ([]Issue, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT number, author_login, state, created_at, closed_at, comments_count, is_bot, title
		FROM issues WHERE repo_id = ?
		ORDER BY created_at DESC LIMIT ?`, repoID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Issue
	for rows.Next() {
		var is Issue
		var bot int
		if err := rows.Scan(&is.Number, &is.AuthorLogin, &is.State, &is.CreatedAt, &is.ClosedAt, &is.CommentsCount, &bot, &is.Title); err != nil {
			return nil, err
		}
		is.IsBot = bot != 0
		out = append(out, is)
	}
	return out, rows.Err()
}

// EarliestEventDate returns the earliest UTC 'YYYY-MM-DD' across commits/PRs/issues,
// backing the "all" window. Returns ErrNotFound when the repo has no events.
func (s *Store) EarliestEventDate(ctx context.Context, repoID int64) (string, error) {
	var d sql.NullString
	err := s.DB.QueryRowContext(ctx, `
		SELECT MIN(day) FROM (
			SELECT MIN(date(committed_at)) AS day FROM commits WHERE repo_id = ?1
			UNION ALL
			SELECT MIN(date(created_at)) FROM pull_requests WHERE repo_id = ?1
			UNION ALL
			SELECT MIN(date(created_at)) FROM issues WHERE repo_id = ?1
		)`, repoID).Scan(&d)
	if err != nil {
		return "", err
	}
	if !d.Valid {
		return "", ErrNotFound
	}
	return d.String, nil
}

// CountOpenIssues counts issues open at asOf.
func (s *Store) CountOpenIssues(ctx context.Context, repoID int64, asOf time.Time, excludeBots bool) (int64, error) {
	var n int64
	err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM issues
		WHERE repo_id = ? AND created_at <= ?
			AND (closed_at IS NULL OR closed_at > ?)`+botFilter(excludeBots),
		repoID, asOf, asOf).Scan(&n)
	return n, err
}

// CountOpenPRs counts pull requests open at asOf (created on/before asOf, not yet
// merged or closed at asOf).
func (s *Store) CountOpenPRs(ctx context.Context, repoID int64, asOf time.Time, excludeBots bool) (int64, error) {
	var n int64
	err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pull_requests
		WHERE repo_id = ? AND created_at <= ?
			AND (merged_at IS NULL OR merged_at > ?)
			AND (closed_at IS NULL OR closed_at > ?)`+botFilter(excludeBots),
		repoID, asOf, asOf, asOf).Scan(&n)
	return n, err
}

// CountContributors counts distinct commit authors in [fromDate, toDate].
func (s *Store) CountContributors(ctx context.Context, repoID int64, fromDate, toDate string, excludeBots bool) (int64, error) {
	var n int64
	err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT author_login) FROM commits
		WHERE repo_id = ? AND date(committed_at) >= ? AND date(committed_at) <= ?`+botFilter(excludeBots),
		repoID, fromDate, toDate).Scan(&n)
	return n, err
}

// CountReleases counts releases for a repo.
func (s *Store) CountReleases(ctx context.Context, repoID int64) (int64, error) {
	var n int64
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM releases WHERE repo_id = ?`, repoID).Scan(&n)
	return n, err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run 'TestDailyRepoStatsRange|TestDailyContributorStatsRange|TestMergedPRDurations|TestReviewLatencies|TestClosedIssueLifetimes|TestOpenIssuesAsOf|TestLatestCommitsPRsIssues|TestEarliestEventDate|TestCountsForOverview' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/reads.go internal/store/reads_test.go
git commit -m "feat: store read methods backing the metrics Source port"
```

---

## Task 2: Source port + Window + Opts

**Files:**
- Create: `internal/metrics/source.go`, `internal/metrics/window.go`, `internal/metrics/window_test.go`

The `Source` interface is the **narrow read-only port** (spec §4/§7) the metrics depend on; `*store.Store` satisfies it because Task 1 gave the store exactly these methods. Metrics never import GitHub or HTTP. `Window` parses the spec's window vocabulary (`30d|90d|6m|1y|all`) into a concrete `[From,To]` date pair with an injected `now` for deterministic tests; `"all"` spans from the repo's earliest event date. `Opts{ExcludeBots bool}` carries the bot toggle.

To keep the `Source` interface decoupled from the store's row types, `Source` is declared in terms of the **store's exported row structs** (re-exported as type aliases in `source.go`) so a metric reads `metrics.DailyRepoStatsRow` etc. without importing `store` directly — but the concrete `*store.Store` still satisfies the interface because the alias *is* the store type.

- [ ] **Step 1: Write the failing test**

`internal/metrics/window_test.go`:
```go
package metrics

import (
	"context"
	"testing"
	"time"
)

func mustTime(s string) time.Time {
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return v
}

func TestParseWindowRelative(t *testing.T) {
	// Anchor on a mid-month date so AddDate month/year math never overflows a
	// short month-end (e.g. AddDate(0,-6,0) on the 31st would normalize forward).
	now := mustTime("2026-03-15T12:00:00Z")
	cases := []struct {
		spec     string
		wantFrom string
		wantTo   string
	}{
		{"30d", "2026-02-13", "2026-03-15"},
		{"90d", "2025-12-15", "2026-03-15"},
		{"6m", "2025-09-15", "2026-03-15"},
		{"1y", "2025-03-15", "2026-03-15"},
	}
	for _, c := range cases {
		w, err := ParseWindow(context.Background(), c.spec, 0, nil, func() time.Time { return now })
		if err != nil {
			t.Fatalf("%s: %v", c.spec, err)
		}
		if w.From != c.wantFrom || w.To != c.wantTo {
			t.Errorf("%s: got [%s,%s], want [%s,%s]", c.spec, w.From, w.To, c.wantFrom, c.wantTo)
		}
	}
}

func TestParseWindowDefaultsTo30d(t *testing.T) {
	now := mustTime("2026-03-15T12:00:00Z")
	w, err := ParseWindow(context.Background(), "", 0, nil, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if w.From != "2026-02-13" || w.To != "2026-03-15" {
		t.Fatalf("default window = [%s,%s], want 30d", w.From, w.To)
	}
}

func TestParseWindowAllUsesEarliest(t *testing.T) {
	now := mustTime("2026-03-31T12:00:00Z")
	src := &windowFakeSource{earliest: "2025-01-15"}
	w, err := ParseWindow(context.Background(), "all", 7, src, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if w.From != "2025-01-15" || w.To != "2026-03-31" {
		t.Fatalf("all window = [%s,%s], want [2025-01-15,2026-03-31]", w.From, w.To)
	}
}

func TestParseWindowAllNoData(t *testing.T) {
	now := mustTime("2026-03-31T12:00:00Z")
	src := &windowFakeSource{earliestErr: errNoData}
	w, err := ParseWindow(context.Background(), "all", 7, src, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	// No data: "all" falls back to a single day (To == From == today).
	if w.From != "2026-03-31" || w.To != "2026-03-31" {
		t.Fatalf("all/no-data window = [%s,%s], want today/today", w.From, w.To)
	}
}

func TestParseWindowRejectsBadSpec(t *testing.T) {
	now := mustTime("2026-03-31T12:00:00Z")
	if _, err := ParseWindow(context.Background(), "7w", 0, nil, func() time.Time { return now }); err == nil {
		t.Fatal("expected error for bad window spec")
	}
}

func TestWindowDays(t *testing.T) {
	w := Window{From: "2026-03-01", To: "2026-03-03"}
	got, err := w.Dates()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-03-01", "2026-03-02", "2026-03-03"}
	if len(got) != len(want) {
		t.Fatalf("dates len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dates[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

// windowFakeSource is a tiny Source used only by the window tests; it implements
// just EarliestEventDate (the rest panic if called).
type windowFakeSource struct {
	earliest    string
	earliestErr error
}

var errNoData = &windowErr{"no data"}

type windowErr struct{ s string }

func (e *windowErr) Error() string { return e.s }

func (f *windowFakeSource) EarliestEventDate(ctx context.Context, repoID int64) (string, error) {
	return f.earliest, f.earliestErr
}
func (f *windowFakeSource) DailyRepoStats(context.Context, int64, string, string) ([]DailyRepoStatsRow, error) {
	panic("unused")
}
func (f *windowFakeSource) DailyContributorStats(context.Context, int64, string, string) ([]DailyContribRow, error) {
	panic("unused")
}
func (f *windowFakeSource) MergedPRDurations(context.Context, int64, string, string, bool) ([]PRDurationRow, error) {
	panic("unused")
}
func (f *windowFakeSource) ReviewLatencies(context.Context, int64, string, string, bool) ([]ReviewLatencyRow, error) {
	panic("unused")
}
func (f *windowFakeSource) ClosedIssueLifetimes(context.Context, int64, string, string, bool) ([]IssueLifetimeRow, error) {
	panic("unused")
}
func (f *windowFakeSource) OpenIssuesAsOf(context.Context, int64, time.Time, bool) ([]OpenIssueRow, error) {
	panic("unused")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run 'TestParseWindow|TestWindowDays' -v`
Expected: FAIL — `undefined: ParseWindow` / `undefined: Window` / `undefined: DailyRepoStatsRow`.

- [ ] **Step 3: Write minimal implementation**

`internal/metrics/source.go`:
```go
// Package metrics is the modular statistics generator (spec §7). Each metric is
// a self-contained, independently testable unit that reads ONLY from a Source
// port — never from GitHub or HTTP (spec §4). *store.Store satisfies Source.
package metrics

import (
	"context"
	"time"

	"github-stats/internal/store"
)

// Row aliases re-export the store's read-method row types so metrics depend on
// the metrics package surface, not on package store directly. Because these are
// type aliases (not new types), *store.Store still satisfies Source.
type (
	DailyRepoStatsRow = store.DailyRepoStatsRow
	DailyContribRow   = store.DailyContribRow
	PRDurationRow     = store.PRDurationRow
	ReviewLatencyRow  = store.ReviewLatencyRow
	IssueLifetimeRow  = store.IssueLifetimeRow
	OpenIssueRow      = store.OpenIssueRow
)

// Source is the narrow read-only port the metrics compute against. *store.Store
// satisfies it. Tests use an in-memory fakeSource.
type Source interface {
	DailyRepoStats(ctx context.Context, repoID int64, fromDate, toDate string) ([]DailyRepoStatsRow, error)
	DailyContributorStats(ctx context.Context, repoID int64, fromDate, toDate string) ([]DailyContribRow, error)
	MergedPRDurations(ctx context.Context, repoID int64, fromDate, toDate string, excludeBots bool) ([]PRDurationRow, error)
	ReviewLatencies(ctx context.Context, repoID int64, fromDate, toDate string, excludeBots bool) ([]ReviewLatencyRow, error)
	ClosedIssueLifetimes(ctx context.Context, repoID int64, fromDate, toDate string, excludeBots bool) ([]IssueLifetimeRow, error)
	OpenIssuesAsOf(ctx context.Context, repoID int64, asOf time.Time, excludeBots bool) ([]OpenIssueRow, error)
	EarliestEventDate(ctx context.Context, repoID int64) (string, error)
}

// Opts carries cross-cutting metric options.
type Opts struct {
	ExcludeBots bool
}
```

`internal/metrics/window.go`:
```go
package metrics

import (
	"context"
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// Window is a concrete inclusive UTC date range [From, To] (each 'YYYY-MM-DD').
type Window struct {
	From string
	To   string
}

// EarliestSource is the slice of Source that ParseWindow needs for "all".
type EarliestSource interface {
	EarliestEventDate(ctx context.Context, repoID int64) (string, error)
}

// ParseWindow turns a window spec ("30d"|"90d"|"6m"|"1y"|"all"|"") into a concrete
// [From, To] range anchored at now() (UTC). An empty spec defaults to "30d".
// "all" spans from the repo's earliest event date (via src) to today; when the
// repo has no events it collapses to today/today.
func ParseWindow(ctx context.Context, spec string, repoID int64, src EarliestSource, now func() time.Time) (Window, error) {
	if now == nil {
		now = time.Now
	}
	today := now().UTC()
	to := today.Format(dateLayout)

	if spec == "" {
		spec = "30d"
	}
	if spec == "all" {
		from := to
		if src != nil {
			earliest, err := src.EarliestEventDate(ctx, repoID)
			if err == nil && earliest != "" {
				from = earliest
			}
		}
		return Window{From: from, To: to}, nil
	}

	var from time.Time
	switch spec {
	case "30d":
		from = today.AddDate(0, 0, -30)
	case "90d":
		from = today.AddDate(0, 0, -90)
	case "6m":
		from = today.AddDate(0, -6, 0)
	case "1y":
		from = today.AddDate(-1, 0, 0)
	default:
		return Window{}, fmt.Errorf("unknown window spec %q", spec)
	}
	return Window{From: from.Format(dateLayout), To: to}, nil
}

// Dates returns every date in [From, To] inclusive as 'YYYY-MM-DD' strings.
func (w Window) Dates() ([]string, error) {
	from, err := time.Parse(dateLayout, w.From)
	if err != nil {
		return nil, err
	}
	to, err := time.Parse(dateLayout, w.To)
	if err != nil {
		return nil, err
	}
	var out []string
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format(dateLayout))
	}
	return out, nil
}

// ToTime returns w.To parsed as the end-of-day instant (To 23:59:59.999... is
// approximated as the next midnight) for "as of" queries. Callers that need an
// inclusive cutoff use the returned time directly.
func (w Window) ToTime() (time.Time, error) {
	to, err := time.Parse(dateLayout, w.To)
	if err != nil {
		return time.Time{}, err
	}
	return to.AddDate(0, 0, 1), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/ -run 'TestParseWindow|TestWindowDays' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/source.go internal/metrics/window.go internal/metrics/window_test.go
git commit -m "feat: metrics Source port, Window parsing, Opts"
```

---

## Task 3: Result shapes (~4 JSON-friendly kinds)

**Files:**
- Create: `internal/metrics/result.go`, `internal/metrics/result_test.go`

`Result` is a small tagged union of the ~4 JSON-friendly shapes the spec calls for (§7): **time-series** (`[]{date,value}`), **scalar** (a labeled `float64`, optionally with a unit + a sample count), **distribution/buckets** (named buckets with counts — e.g. open-issue-age), and **leaderboard** (ranked rows). Each `Result` carries a `Kind` tag so the M5 frontend can pick one renderer per shape and marshal it directly. Only the populated field is emitted (the others stay `nil`/`omitempty`).

- [ ] **Step 1: Write the failing test**

`internal/metrics/result_test.go`:
```go
package metrics

import (
	"encoding/json"
	"testing"
)

func TestTimeSeriesResultJSON(t *testing.T) {
	r := TimeSeries("commits", []Point{{Date: "2026-03-01", Value: 2}, {Date: "2026-03-02", Value: 5}})
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(b, &got)
	if got["kind"] != "time_series" {
		t.Fatalf("kind = %v, want time_series", got["kind"])
	}
	series, ok := got["series"].([]any)
	if !ok || len(series) != 2 {
		t.Fatalf("series = %v", got["series"])
	}
	first := series[0].(map[string]any)
	if first["date"] != "2026-03-01" || first["value"].(float64) != 2 {
		t.Fatalf("first point = %v", first)
	}
	// scalar/buckets/rows must be omitted.
	if _, present := got["value"]; present {
		t.Fatal("time-series result must not emit scalar value")
	}
}

func TestScalarResultJSON(t *testing.T) {
	r := Scalar("avg_hours", 12.5, "hours", 7)
	b, _ := json.Marshal(r)
	var got map[string]any
	json.Unmarshal(b, &got)
	if got["kind"] != "scalar" {
		t.Fatalf("kind = %v", got["kind"])
	}
	if got["value"].(float64) != 12.5 || got["unit"] != "hours" || got["count"].(float64) != 7 {
		t.Fatalf("scalar = %v", got)
	}
}

func TestBucketsResultJSON(t *testing.T) {
	r := Buckets("open_issue_age", []Bucket{{Label: "<24h", Count: 3}, {Label: "older", Count: 1}})
	b, _ := json.Marshal(r)
	var got map[string]any
	json.Unmarshal(b, &got)
	if got["kind"] != "buckets" {
		t.Fatalf("kind = %v", got["kind"])
	}
	buckets := got["buckets"].([]any)
	if len(buckets) != 2 || buckets[0].(map[string]any)["label"] != "<24h" {
		t.Fatalf("buckets = %v", buckets)
	}
}

func TestLeaderboardResultJSON(t *testing.T) {
	r := Leaderboard("contributors", []LeaderRow{
		{Login: "neo", Commits: 10, Additions: 100, Deletions: 5},
	})
	b, _ := json.Marshal(r)
	var got map[string]any
	json.Unmarshal(b, &got)
	if got["kind"] != "leaderboard" {
		t.Fatalf("kind = %v", got["kind"])
	}
	rows := got["rows"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["login"] != "neo" {
		t.Fatalf("rows = %v", rows)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run 'TestTimeSeriesResultJSON|TestScalarResultJSON|TestBucketsResultJSON|TestLeaderboardResultJSON' -v`
Expected: FAIL — `undefined: TimeSeries`.

- [ ] **Step 3: Write minimal implementation**

`internal/metrics/result.go`:
```go
package metrics

// ResultKind tags a Result so the frontend can pick one renderer per shape.
type ResultKind string

const (
	KindTimeSeries  ResultKind = "time_series"
	KindScalar      ResultKind = "scalar"
	KindBuckets     ResultKind = "buckets"
	KindLeaderboard ResultKind = "leaderboard"
)

// Point is one (date, value) sample in a time series.
type Point struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// Bucket is one named count in a distribution.
type Bucket struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// LeaderRow is one ranked contributor row.
type LeaderRow struct {
	Login     string `json:"login"`
	Commits   int64  `json:"commits"`
	Additions int64  `json:"additions"`
	Deletions int64  `json:"deletions"`
}

// Result is the JSON-friendly tagged union returned by every metric. Exactly one
// of Series / (Value,Unit,Count) / Buckets / Rows is populated, per Kind.
type Result struct {
	Kind   ResultKind `json:"kind"`
	Label  string     `json:"label,omitempty"`
	Series []Point    `json:"series,omitempty"`

	// Scalar payload.
	Value *float64 `json:"value,omitempty"`
	Unit  string   `json:"unit,omitempty"`
	Count *int64   `json:"count,omitempty"`

	Buckets []Bucket    `json:"buckets,omitempty"`
	Rows    []LeaderRow `json:"rows,omitempty"`
}

// TimeSeries builds a time-series Result.
func TimeSeries(label string, series []Point) Result {
	if series == nil {
		series = []Point{}
	}
	return Result{Kind: KindTimeSeries, Label: label, Series: series}
}

// Scalar builds a scalar Result with a unit and a sample count.
func Scalar(label string, value float64, unit string, count int64) Result {
	v, c := value, count
	return Result{Kind: KindScalar, Label: label, Value: &v, Unit: unit, Count: &c}
}

// Buckets builds a distribution Result.
func Buckets(label string, buckets []Bucket) Result {
	if buckets == nil {
		buckets = []Bucket{}
	}
	return Result{Kind: KindBuckets, Label: label, Buckets: buckets}
}

// Leaderboard builds a leaderboard Result.
func Leaderboard(label string, rows []LeaderRow) Result {
	if rows == nil {
		rows = []LeaderRow{}
	}
	return Result{Kind: KindLeaderboard, Label: label, Rows: rows}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/ -run 'TestTimeSeriesResultJSON|TestScalarResultJSON|TestBucketsResultJSON|TestLeaderboardResultJSON' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/result.go internal/metrics/result_test.go
git commit -m "feat: metrics Result shapes (time-series/scalar/buckets/leaderboard)"
```

---

## Task 4: Metric interface + Registry + fakeSource

**Files:**
- Create: `internal/metrics/registry.go`, `internal/metrics/registry_test.go`, `internal/metrics/fakesource_test.go`

The `Metric` interface (spec §7) is `Key()` + `Compute(ctx, src, repoID, w, opts)`. The `Registry` maps `key → Metric` with `Register`/`Get`/`Keys` and a `Compute(ctx, src, repoID, keys, w, opts)` that runs the requested metrics and returns `map[string]Result`. `fakeSource` is the shared in-memory `Source` every metric unit test builds rows against — it lives in `fakesource_test.go` so it is compiled only for tests but visible to all `_test.go` files in the package.

- [ ] **Step 1: Write the failing test**

`internal/metrics/fakesource_test.go`:
```go
package metrics

import (
	"context"
	"time"

	"github-stats/internal/store"
)

// fakeSource is an in-memory Source for deterministic metric unit tests. Tests
// populate the slices directly; the methods filter/return them. ExcludeBots is
// honored by the duration/open-issue readers via the IsBot flags on rows.
type fakeSource struct {
	daily        []DailyRepoStatsRow
	contrib      []DailyContribRow
	mergedPRs    []prRow
	reviews      []reviewRow
	closedIssues []issueRow
	openIssues   []openRow
	earliest     string
	earliestErr  error
}

type prRow struct {
	row   PRDurationRow
	isBot bool
}
type reviewRow struct {
	row   ReviewLatencyRow
	isBot bool
}
type issueRow struct {
	row   IssueLifetimeRow
	isBot bool
}
type openRow struct {
	row   OpenIssueRow
	isBot bool
}

func inRange(date, from, to string) bool { return date >= from && date <= to }

func (f *fakeSource) DailyRepoStats(_ context.Context, _ int64, from, to string) ([]DailyRepoStatsRow, error) {
	var out []DailyRepoStatsRow
	for _, r := range f.daily {
		if inRange(r.Date, from, to) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeSource) DailyContributorStats(_ context.Context, _ int64, from, to string) ([]DailyContribRow, error) {
	var out []DailyContribRow
	for _, r := range f.contrib {
		if inRange(r.Date, from, to) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeSource) MergedPRDurations(_ context.Context, _ int64, from, to string, excludeBots bool) ([]PRDurationRow, error) {
	var out []PRDurationRow
	for _, r := range f.mergedPRs {
		if excludeBots && r.isBot {
			continue
		}
		if inRange(r.row.MergedAt.UTC().Format(dateLayout), from, to) {
			out = append(out, r.row)
		}
	}
	return out, nil
}

func (f *fakeSource) ReviewLatencies(_ context.Context, _ int64, from, to string, excludeBots bool) ([]ReviewLatencyRow, error) {
	var out []ReviewLatencyRow
	for _, r := range f.reviews {
		if excludeBots && r.isBot {
			continue
		}
		if inRange(r.row.FirstReviewAt.UTC().Format(dateLayout), from, to) {
			out = append(out, r.row)
		}
	}
	return out, nil
}

func (f *fakeSource) ClosedIssueLifetimes(_ context.Context, _ int64, from, to string, excludeBots bool) ([]IssueLifetimeRow, error) {
	var out []IssueLifetimeRow
	for _, r := range f.closedIssues {
		if excludeBots && r.isBot {
			continue
		}
		if inRange(r.row.ClosedAt.UTC().Format(dateLayout), from, to) {
			out = append(out, r.row)
		}
	}
	return out, nil
}

func (f *fakeSource) OpenIssuesAsOf(_ context.Context, _ int64, asOf time.Time, excludeBots bool) ([]OpenIssueRow, error) {
	var out []OpenIssueRow
	for _, r := range f.openIssues {
		if excludeBots && r.isBot {
			continue
		}
		if !r.row.CreatedAt.After(asOf) {
			out = append(out, r.row)
		}
	}
	return out, nil
}

func (f *fakeSource) EarliestEventDate(_ context.Context, _ int64) (string, error) {
	return f.earliest, f.earliestErr
}

// compile-time assertion that *store.Store also satisfies Source (the prod impl).
var _ Source = (*store.Store)(nil)

// fixed reference clock for metric unit tests.
func refNow() time.Time { return mustTime("2026-03-31T12:00:00Z") }
```

`internal/metrics/registry_test.go`:
```go
package metrics

import (
	"context"
	"testing"
)

// stubMetric is a trivial Metric for registry tests.
type stubMetric struct {
	key string
	res Result
}

func (m stubMetric) Key() string { return m.key }
func (m stubMetric) Compute(_ context.Context, _ Source, _ int64, _ Window, _ Opts) (Result, error) {
	return m.res, nil
}

func TestRegistryRegisterGetKeys(t *testing.T) {
	reg := NewRegistry()
	reg.Register(stubMetric{key: "a", res: Scalar("a", 1, "", 0)})
	reg.Register(stubMetric{key: "b", res: Scalar("b", 2, "", 0)})

	if _, ok := reg.Get("a"); !ok {
		t.Fatal("Get(a) missing")
	}
	if _, ok := reg.Get("missing"); ok {
		t.Fatal("Get(missing) should be false")
	}
	keys := reg.Keys()
	if len(keys) != 2 {
		t.Fatalf("Keys() = %v, want 2 sorted", keys)
	}
	if keys[0] != "a" || keys[1] != "b" {
		t.Fatalf("Keys() not sorted: %v", keys)
	}
}

func TestRegistryComputeSelectedKeys(t *testing.T) {
	reg := NewRegistry()
	reg.Register(stubMetric{key: "a", res: Scalar("a", 1, "", 0)})
	reg.Register(stubMetric{key: "b", res: Scalar("b", 2, "", 0)})

	out, err := reg.Compute(context.Background(), &fakeSource{}, 1, []string{"a"}, Window{From: "2026-03-01", To: "2026-03-31"}, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("computed %d results, want 1", len(out))
	}
	if out["a"].Value == nil || *out["a"].Value != 1 {
		t.Fatalf("a result = %+v", out["a"])
	}
}

func TestRegistryComputeAllWhenNoKeys(t *testing.T) {
	reg := NewRegistry()
	reg.Register(stubMetric{key: "a", res: Scalar("a", 1, "", 0)})
	reg.Register(stubMetric{key: "b", res: Scalar("b", 2, "", 0)})

	out, err := reg.Compute(context.Background(), &fakeSource{}, 1, nil, Window{From: "2026-03-01", To: "2026-03-31"}, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("computed %d results, want all 2", len(out))
	}
}

func TestRegistryComputeUnknownKeyErrors(t *testing.T) {
	reg := NewRegistry()
	reg.Register(stubMetric{key: "a", res: Scalar("a", 1, "", 0)})
	if _, err := reg.Compute(context.Background(), &fakeSource{}, 1, []string{"nope"}, Window{}, Opts{}); err == nil {
		t.Fatal("expected error for unknown key")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run 'TestRegistry' -v`
Expected: FAIL — `undefined: NewRegistry`.

- [ ] **Step 3: Write minimal implementation**

`internal/metrics/registry.go`:
```go
package metrics

import (
	"context"
	"fmt"
	"sort"
)

// Metric is a single, self-contained statistic generator (spec §7). Compute reads
// ONLY from the Source port — never from GitHub or HTTP.
type Metric interface {
	Key() string
	Compute(ctx context.Context, src Source, repoID int64, w Window, opts Opts) (Result, error)
}

// Registry maps metric key → Metric. Adding a stat is one Register call.
type Registry struct {
	metrics map[string]Metric
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{metrics: make(map[string]Metric)}
}

// Register adds a metric, overwriting any prior metric with the same key.
func (r *Registry) Register(m Metric) {
	r.metrics[m.Key()] = m
}

// Get returns the metric for a key.
func (r *Registry) Get(key string) (Metric, bool) {
	m, ok := r.metrics[key]
	return m, ok
}

// Keys returns all registered keys, sorted.
func (r *Registry) Keys() []string {
	keys := make([]string, 0, len(r.metrics))
	for k := range r.metrics {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Compute runs the requested metrics (all registered, sorted, when keys is empty)
// and returns key → Result. An unknown key is an error.
func (r *Registry) Compute(ctx context.Context, src Source, repoID int64, keys []string, w Window, opts Opts) (map[string]Result, error) {
	if len(keys) == 0 {
		keys = r.Keys()
	}
	out := make(map[string]Result, len(keys))
	for _, key := range keys {
		m, ok := r.metrics[key]
		if !ok {
			return nil, fmt.Errorf("unknown metric %q", key)
		}
		res, err := m.Compute(ctx, src, repoID, w, opts)
		if err != nil {
			return nil, fmt.Errorf("metric %q: %w", key, err)
		}
		out[key] = res
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/ -run 'TestRegistry' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/registry.go internal/metrics/registry_test.go internal/metrics/fakesource_test.go
git commit -m "feat: Metric interface, Registry, and test fakeSource"
```

---

## Task 5: EMA helper + shared stat math

**Files:**
- Create: `internal/metrics/ema.go`, `internal/metrics/ema_test.go`

`EMA` is the spec's smoothing helper (5d/14d) over a daily series; it is used by chart metrics that want a smoothed companion line. The conventional seed is the first value; thereafter `ema = alpha*x + (1-alpha)*prev` with `alpha = 2/(span+1)`. This file also holds the small shared stat helpers (`mean`, `median`) the duration metrics reuse, kept here so they have one tested home.

- [ ] **Step 1: Write the failing test**

`internal/metrics/ema_test.go`:
```go
package metrics

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestEMASpan5KnownValues(t *testing.T) {
	in := []float64{10, 12, 14, 16, 18}
	got := EMA(in, 5)
	// alpha = 2/(5+1) = 1/3. Seed = 10.
	// e1 = 10
	// e2 = 12/3 + 10*2/3 = 4 + 6.6666667 = 10.6666667
	// e3 = 14/3 + 10.6666667*2/3 = 4.6666667 + 7.1111111 = 11.7777778
	// e4 = 16/3 + 11.7777778*2/3 = 5.3333333 + 7.8518519 = 13.1851852
	// e5 = 18/3 + 13.1851852*2/3 = 6 + 8.7901235 = 14.7901235
	want := []float64{10, 10.6666666667, 11.7777777778, 13.1851851852, 14.7901234568}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-6 {
			t.Errorf("ema[%d] = %.10f, want %.10f", i, got[i], want[i])
		}
	}
}

func TestEMAEmptyAndSingle(t *testing.T) {
	if got := EMA(nil, 5); len(got) != 0 {
		t.Fatalf("EMA(nil) = %v, want empty", got)
	}
	got := EMA([]float64{7}, 14)
	if len(got) != 1 || !approx(got[0], 7) {
		t.Fatalf("EMA([7]) = %v, want [7]", got)
	}
}

func TestEMAInvalidSpanFallsBackToInput(t *testing.T) {
	in := []float64{1, 2, 3}
	got := EMA(in, 0) // span <= 0 → no smoothing, copy of input
	for i := range in {
		if !approx(got[i], in[i]) {
			t.Fatalf("EMA span 0 should copy input: %v", got)
		}
	}
}

func TestMeanAndMedian(t *testing.T) {
	if !approx(mean([]float64{2, 4, 6}), 4) {
		t.Fatal("mean wrong")
	}
	if !approx(mean(nil), 0) {
		t.Fatal("mean(nil) should be 0")
	}
	if !approx(median([]float64{3, 1, 2}), 2) {
		t.Fatal("median odd wrong")
	}
	if !approx(median([]float64{1, 2, 3, 4}), 2.5) {
		t.Fatal("median even wrong")
	}
	if !approx(median(nil), 0) {
		t.Fatal("median(nil) should be 0")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run 'TestEMA|TestMeanAndMedian' -v`
Expected: FAIL — `undefined: EMA`.

- [ ] **Step 3: Write minimal implementation**

`internal/metrics/ema.go`:
```go
package metrics

import "sort"

// EMA returns the exponential moving average of values with the given span
// (e.g. 5 or 14). alpha = 2/(span+1); the series is seeded with the first value.
// A span <= 0 disables smoothing and returns a copy of the input.
func EMA(values []float64, span int) []float64 {
	out := make([]float64, len(values))
	if len(values) == 0 {
		return out
	}
	if span <= 0 {
		copy(out, values)
		return out
	}
	alpha := 2.0 / (float64(span) + 1.0)
	out[0] = values[0]
	for i := 1; i < len(values); i++ {
		out[i] = alpha*values[i] + (1-alpha)*out[i-1]
	}
	return out
}

// mean returns the arithmetic mean, or 0 for an empty slice.
func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// median returns the median, or 0 for an empty slice. It sorts a copy.
func median(values []float64) float64 {
	n := len(values)
	if n == 0 {
		return 0
	}
	cp := make([]float64, n)
	copy(cp, values)
	sort.Float64s(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/ -run 'TestEMA|TestMeanAndMedian' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/ema.go internal/metrics/ema_test.go
git commit -m "feat: EMA smoothing helper + mean/median stat math"
```

---

## Task 6: commit_rate metric (time-series)

**Files:**
- Create: `internal/metrics/commit_rate.go`, `internal/metrics/commit_rate_test.go`

`commit_rate` is a **time-series** of commits per day across the window, read from `daily_repo_stats` (O(days)). Days with no aggregate row are emitted as `0` so the series is dense (one point per date in the window) — the frontend renders a continuous line. (Aggregates do not carry bot flags, so `commit_rate` does not vary with `exclude_bots`; this is documented in the Public API surface.)

- [ ] **Step 1: Write the failing test**

`internal/metrics/commit_rate_test.go`:
```go
package metrics

import (
	"context"
	"testing"
)

func TestCommitRateDenseSeries(t *testing.T) {
	src := &fakeSource{daily: []DailyRepoStatsRow{
		{Date: "2026-03-01", Commits: 2},
		{Date: "2026-03-03", Commits: 5},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-03"}
	res, err := commitRate{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindTimeSeries {
		t.Fatalf("kind = %v, want time_series", res.Kind)
	}
	if len(res.Series) != 3 {
		t.Fatalf("series len = %d, want 3 (dense)", len(res.Series))
	}
	want := []Point{{"2026-03-01", 2}, {"2026-03-02", 0}, {"2026-03-03", 5}}
	for i, p := range want {
		if res.Series[i] != p {
			t.Fatalf("point[%d] = %+v, want %+v", i, res.Series[i], p)
		}
	}
}

func TestCommitRateKey(t *testing.T) {
	if commitRate{}.Key() != "commit_rate" {
		t.Fatalf("key = %q", commitRate{}.Key())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run 'TestCommitRate' -v`
Expected: FAIL — `undefined: commitRate`.

- [ ] **Step 3: Write minimal implementation**

`internal/metrics/commit_rate.go`:
```go
package metrics

import "context"

// commitRate is a time-series of commits/day over the window (reads daily aggregates).
type commitRate struct{}

func (commitRate) Key() string { return "commit_rate" }

func (commitRate) Compute(ctx context.Context, src Source, repoID int64, w Window, opts Opts) (Result, error) {
	rows, err := src.DailyRepoStats(ctx, repoID, w.From, w.To)
	if err != nil {
		return Result{}, err
	}
	byDate := make(map[string]int64, len(rows))
	for _, r := range rows {
		byDate[r.Date] = r.Commits
	}
	dates, err := w.Dates()
	if err != nil {
		return Result{}, err
	}
	series := make([]Point, 0, len(dates))
	for _, d := range dates {
		series = append(series, Point{Date: d, Value: float64(byDate[d])})
	}
	return TimeSeries("Commits per day", series), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/ -run 'TestCommitRate' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/commit_rate.go internal/metrics/commit_rate_test.go
git commit -m "feat: commit_rate time-series metric"
```

---

## Task 7: pr_throughput metric (time-series)

**Files:**
- Create: `internal/metrics/pr_throughput.go`, `internal/metrics/pr_throughput_test.go`

`pr_throughput` is a **time-series** of PRs **merged** per day over the window (the headline throughput line), read from `daily_repo_stats.prs_merged`. Dense series (zero-filled), one point per date. (Like `commit_rate`, it reads bot-agnostic aggregates; the Public API surface notes `exclude_bots` does not affect it.)

- [ ] **Step 1: Write the failing test**

`internal/metrics/pr_throughput_test.go`:
```go
package metrics

import (
	"context"
	"testing"
)

func TestPRThroughputMergedPerDay(t *testing.T) {
	src := &fakeSource{daily: []DailyRepoStatsRow{
		{Date: "2026-03-01", PRsMerged: 1},
		{Date: "2026-03-02", PRsMerged: 3},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-02"}
	res, err := prThroughput{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindTimeSeries || len(res.Series) != 2 {
		t.Fatalf("res = %+v", res)
	}
	if res.Series[0].Value != 1 || res.Series[1].Value != 3 {
		t.Fatalf("values = %v", res.Series)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run 'TestPRThroughput' -v`
Expected: FAIL — `undefined: prThroughput`.

- [ ] **Step 3: Write minimal implementation**

`internal/metrics/pr_throughput.go`:
```go
package metrics

import "context"

// prThroughput is a time-series of PRs merged/day over the window.
type prThroughput struct{}

func (prThroughput) Key() string { return "pr_throughput" }

func (prThroughput) Compute(ctx context.Context, src Source, repoID int64, w Window, opts Opts) (Result, error) {
	rows, err := src.DailyRepoStats(ctx, repoID, w.From, w.To)
	if err != nil {
		return Result{}, err
	}
	byDate := make(map[string]int64, len(rows))
	for _, r := range rows {
		byDate[r.Date] = r.PRsMerged
	}
	dates, err := w.Dates()
	if err != nil {
		return Result{}, err
	}
	series := make([]Point, 0, len(dates))
	for _, d := range dates {
		series = append(series, Point{Date: d, Value: float64(byDate[d])})
	}
	return TimeSeries("PRs merged per day", series), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/ -run 'TestPRThroughput' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/pr_throughput.go internal/metrics/pr_throughput_test.go
git commit -m "feat: pr_throughput time-series metric"
```

---

## Task 8: time_to_merge metric (scalar)

**Files:**
- Create: `internal/metrics/time_to_merge.go`, `internal/metrics/time_to_merge_test.go`

`time_to_merge` is a **scalar** (mean and median hours from PR creation to merge) over PRs merged in the window, read from the event table (`MergedPRDurations`) so `exclude_bots` applies. The Result is a scalar carrying the **median** hours as the headline `value`, `unit:"hours"`, `count` = number of merged PRs sampled, and the **mean** stashed in the label for context. (Median is the headline because merge time is right-skewed.)

- [ ] **Step 1: Write the failing test**

`internal/metrics/time_to_merge_test.go`:
```go
package metrics

import (
	"context"
	"testing"
)

func TestTimeToMergeMedianHours(t *testing.T) {
	src := &fakeSource{mergedPRs: []prRow{
		// 12h, 24h, 36h durations.
		{row: PRDurationRow{Number: 1, CreatedAt: mustTime("2026-03-01T00:00:00Z"), MergedAt: mustTime("2026-03-01T12:00:00Z")}},
		{row: PRDurationRow{Number: 2, CreatedAt: mustTime("2026-03-02T00:00:00Z"), MergedAt: mustTime("2026-03-03T00:00:00Z")}},
		{row: PRDurationRow{Number: 3, CreatedAt: mustTime("2026-03-04T00:00:00Z"), MergedAt: mustTime("2026-03-05T12:00:00Z")}, isBot: true},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-31"}

	res, err := timeToMerge{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindScalar {
		t.Fatalf("kind = %v", res.Kind)
	}
	// All three: durations 12,24,36 → median 24h.
	if res.Value == nil || *res.Value != 24 {
		t.Fatalf("median = %v, want 24", res.Value)
	}
	if res.Count == nil || *res.Count != 3 {
		t.Fatalf("count = %v, want 3", res.Count)
	}
	if res.Unit != "hours" {
		t.Fatalf("unit = %q", res.Unit)
	}

	// exclude_bots drops PR3 → durations 12,24 → median 18h, count 2.
	res2, _ := timeToMerge{}.Compute(context.Background(), src, 1, w, Opts{ExcludeBots: true})
	if res2.Value == nil || *res2.Value != 18 || *res2.Count != 2 {
		t.Fatalf("excl bots: value=%v count=%v", res2.Value, res2.Count)
	}
}

func TestTimeToMergeEmpty(t *testing.T) {
	res, err := timeToMerge{}.Compute(context.Background(), &fakeSource{}, 1, Window{From: "2026-03-01", To: "2026-03-31"}, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Value == nil || *res.Value != 0 || *res.Count != 0 {
		t.Fatalf("empty time_to_merge = %+v", res)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run 'TestTimeToMerge' -v`
Expected: FAIL — `undefined: timeToMerge`.

- [ ] **Step 3: Write minimal implementation**

`internal/metrics/time_to_merge.go`:
```go
package metrics

import (
	"context"
	"fmt"
)

// timeToMerge is a scalar: median (headline) and mean hours from PR creation to
// merge, over PRs merged in the window. Reads the event table so exclude_bots applies.
type timeToMerge struct{}

func (timeToMerge) Key() string { return "time_to_merge" }

func (timeToMerge) Compute(ctx context.Context, src Source, repoID int64, w Window, opts Opts) (Result, error) {
	rows, err := src.MergedPRDurations(ctx, repoID, w.From, w.To, opts.ExcludeBots)
	if err != nil {
		return Result{}, err
	}
	hours := make([]float64, 0, len(rows))
	for _, r := range rows {
		hours = append(hours, r.MergedAt.Sub(r.CreatedAt).Hours())
	}
	label := fmt.Sprintf("Time to merge (mean %.1fh)", mean(hours))
	return Scalar(label, median(hours), "hours", int64(len(hours))), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/ -run 'TestTimeToMerge' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/time_to_merge.go internal/metrics/time_to_merge_test.go
git commit -m "feat: time_to_merge scalar metric (median/mean hours)"
```

---

## Task 9: review_latency metric (scalar)

**Files:**
- Create: `internal/metrics/review_latency.go`, `internal/metrics/review_latency_test.go`

`review_latency` is a **scalar**: median (headline) and mean hours from PR creation to its **first review** (`first_review_at`), over PRs whose first review landed in the window. Reads the event table (`ReviewLatencies`) so `exclude_bots` applies; PRs without a first review are excluded by the store query.

- [ ] **Step 1: Write the failing test**

`internal/metrics/review_latency_test.go`:
```go
package metrics

import (
	"context"
	"testing"
)

func TestReviewLatencyMedianHours(t *testing.T) {
	src := &fakeSource{reviews: []reviewRow{
		// 2h and 6h → median 4h.
		{row: ReviewLatencyRow{Number: 1, CreatedAt: mustTime("2026-03-01T00:00:00Z"), FirstReviewAt: mustTime("2026-03-01T02:00:00Z")}},
		{row: ReviewLatencyRow{Number: 2, CreatedAt: mustTime("2026-03-02T00:00:00Z"), FirstReviewAt: mustTime("2026-03-02T06:00:00Z")}},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-31"}
	res, err := reviewLatency{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindScalar || res.Value == nil || *res.Value != 4 || *res.Count != 2 || res.Unit != "hours" {
		t.Fatalf("review latency = %+v", res)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run 'TestReviewLatency' -v`
Expected: FAIL — `undefined: reviewLatency`.

- [ ] **Step 3: Write minimal implementation**

`internal/metrics/review_latency.go`:
```go
package metrics

import (
	"context"
	"fmt"
)

// reviewLatency is a scalar: median (headline) and mean hours from PR creation to
// first review. Reads the event table so exclude_bots applies.
type reviewLatency struct{}

func (reviewLatency) Key() string { return "review_latency" }

func (reviewLatency) Compute(ctx context.Context, src Source, repoID int64, w Window, opts Opts) (Result, error) {
	rows, err := src.ReviewLatencies(ctx, repoID, w.From, w.To, opts.ExcludeBots)
	if err != nil {
		return Result{}, err
	}
	hours := make([]float64, 0, len(rows))
	for _, r := range rows {
		hours = append(hours, r.FirstReviewAt.Sub(r.CreatedAt).Hours())
	}
	label := fmt.Sprintf("Review latency (mean %.1fh)", mean(hours))
	return Scalar(label, median(hours), "hours", int64(len(hours))), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/ -run 'TestReviewLatency' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/review_latency.go internal/metrics/review_latency_test.go
git commit -m "feat: review_latency scalar metric (median/mean hours)"
```

---

## Task 10: issue_lifetime metric (scalar)

**Files:**
- Create: `internal/metrics/issue_lifetime.go`, `internal/metrics/issue_lifetime_test.go`

`issue_lifetime` is a **scalar**: median (headline) and mean hours from issue creation to close, over issues closed in the window. Reads the event table (`ClosedIssueLifetimes`) so `exclude_bots` applies.

- [ ] **Step 1: Write the failing test**

`internal/metrics/issue_lifetime_test.go`:
```go
package metrics

import (
	"context"
	"testing"
)

func TestIssueLifetimeMedianHours(t *testing.T) {
	src := &fakeSource{closedIssues: []issueRow{
		// 24h, 48h, 72h → median 48h.
		{row: IssueLifetimeRow{Number: 1, CreatedAt: mustTime("2026-03-01T00:00:00Z"), ClosedAt: mustTime("2026-03-02T00:00:00Z")}},
		{row: IssueLifetimeRow{Number: 2, CreatedAt: mustTime("2026-03-01T00:00:00Z"), ClosedAt: mustTime("2026-03-03T00:00:00Z")}},
		{row: IssueLifetimeRow{Number: 3, CreatedAt: mustTime("2026-03-01T00:00:00Z"), ClosedAt: mustTime("2026-03-04T00:00:00Z")}},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-31"}
	res, err := issueLifetime{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindScalar || res.Value == nil || *res.Value != 48 || *res.Count != 3 {
		t.Fatalf("issue lifetime = %+v", res)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run 'TestIssueLifetime' -v`
Expected: FAIL — `undefined: issueLifetime`.

- [ ] **Step 3: Write minimal implementation**

`internal/metrics/issue_lifetime.go`:
```go
package metrics

import (
	"context"
	"fmt"
)

// issueLifetime is a scalar: median (headline) and mean hours from issue creation
// to close. Reads the event table so exclude_bots applies.
type issueLifetime struct{}

func (issueLifetime) Key() string { return "issue_lifetime" }

func (issueLifetime) Compute(ctx context.Context, src Source, repoID int64, w Window, opts Opts) (Result, error) {
	rows, err := src.ClosedIssueLifetimes(ctx, repoID, w.From, w.To, opts.ExcludeBots)
	if err != nil {
		return Result{}, err
	}
	hours := make([]float64, 0, len(rows))
	for _, r := range rows {
		hours = append(hours, r.ClosedAt.Sub(r.CreatedAt).Hours())
	}
	label := fmt.Sprintf("Issue lifetime (mean %.1fh)", mean(hours))
	return Scalar(label, median(hours), "hours", int64(len(hours))), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/ -run 'TestIssueLifetime' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/issue_lifetime.go internal/metrics/issue_lifetime_test.go
git commit -m "feat: issue_lifetime scalar metric (median/mean hours)"
```

---

## Task 11: open_issue_age metric (distribution/buckets)

**Files:**
- Create: `internal/metrics/open_issue_age.go`, `internal/metrics/open_issue_age_test.go`

`open_issue_age` is a **distribution/buckets** Result: of all issues open **as of the window's end** (`w.ToTime()`), how many fall into each age bucket — `<24h`, `<7d`, `<30d`, `<90d`, `<180d`, `older` (spec §7). Age is `asOf - created_at`. Reads `OpenIssuesAsOf` so `exclude_bots` applies. Every bucket is always present (count 0 if empty) and emitted in fixed order so the frontend bar chart is stable.

- [ ] **Step 1: Write the failing test**

`internal/metrics/open_issue_age_test.go`:
```go
package metrics

import (
	"context"
	"testing"
)

func TestOpenIssueAgeBuckets(t *testing.T) {
	// Window ends 2026-03-31; ToTime() = 2026-04-01T00:00:00Z is the asOf cutoff.
	src := &fakeSource{openIssues: []openRow{
		{row: OpenIssueRow{Number: 1, CreatedAt: mustTime("2026-03-31T06:00:00Z")}},  // ~18h → <24h
		{row: OpenIssueRow{Number: 2, CreatedAt: mustTime("2026-03-28T00:00:00Z")}},  // 4d → <7d
		{row: OpenIssueRow{Number: 3, CreatedAt: mustTime("2026-03-10T00:00:00Z")}},  // 22d → <30d
		{row: OpenIssueRow{Number: 4, CreatedAt: mustTime("2026-02-01T00:00:00Z")}},  // 59d → <90d
		{row: OpenIssueRow{Number: 5, CreatedAt: mustTime("2025-12-01T00:00:00Z")}},  // 121d → <180d
		{row: OpenIssueRow{Number: 6, CreatedAt: mustTime("2025-01-01T00:00:00Z")}, isBot: true}, // >180d → older (bot)
	}}
	w := Window{From: "2026-03-01", To: "2026-03-31"}

	res, err := openIssueAge{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindBuckets {
		t.Fatalf("kind = %v", res.Kind)
	}
	want := []Bucket{
		{"<24h", 1}, {"<7d", 1}, {"<30d", 1}, {"<90d", 1}, {"<180d", 1}, {"older", 1},
	}
	if len(res.Buckets) != len(want) {
		t.Fatalf("buckets = %+v", res.Buckets)
	}
	for i, b := range want {
		if res.Buckets[i] != b {
			t.Fatalf("bucket[%d] = %+v, want %+v", i, res.Buckets[i], b)
		}
	}

	// exclude_bots drops issue 6 → older bucket becomes 0 (but still present).
	res2, _ := openIssueAge{}.Compute(context.Background(), src, 1, w, Opts{ExcludeBots: true})
	if res2.Buckets[5] != (Bucket{"older", 0}) {
		t.Fatalf("excl bots older bucket = %+v, want 0", res2.Buckets[5])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run 'TestOpenIssueAge' -v`
Expected: FAIL — `undefined: openIssueAge`.

- [ ] **Step 3: Write minimal implementation**

`internal/metrics/open_issue_age.go`:
```go
package metrics

import (
	"context"
	"time"
)

// openIssueAge is a distribution: open issues (as of window end) bucketed by age.
type openIssueAge struct{}

func (openIssueAge) Key() string { return "open_issue_age" }

// ageBucketBounds are the upper bounds (exclusive) of each labeled bucket; the
// final "older" bucket catches everything past the last bound.
var ageBucketBounds = []struct {
	label string
	limit time.Duration
}{
	{"<24h", 24 * time.Hour},
	{"<7d", 7 * 24 * time.Hour},
	{"<30d", 30 * 24 * time.Hour},
	{"<90d", 90 * 24 * time.Hour},
	{"<180d", 180 * 24 * time.Hour},
}

func (openIssueAge) Compute(ctx context.Context, src Source, repoID int64, w Window, opts Opts) (Result, error) {
	asOf, err := w.ToTime()
	if err != nil {
		return Result{}, err
	}
	rows, err := src.OpenIssuesAsOf(ctx, repoID, asOf, opts.ExcludeBots)
	if err != nil {
		return Result{}, err
	}
	counts := make([]int64, len(ageBucketBounds)+1) // +1 for "older"
	for _, r := range rows {
		age := asOf.Sub(r.CreatedAt)
		placed := false
		for i, b := range ageBucketBounds {
			if age < b.limit {
				counts[i]++
				placed = true
				break
			}
		}
		if !placed {
			counts[len(counts)-1]++
		}
	}
	buckets := make([]Bucket, 0, len(counts))
	for i, b := range ageBucketBounds {
		buckets = append(buckets, Bucket{Label: b.label, Count: counts[i]})
	}
	buckets = append(buckets, Bucket{Label: "older", Count: counts[len(counts)-1]})
	return Buckets("Open issue age", buckets), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/ -run 'TestOpenIssueAge' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/open_issue_age.go internal/metrics/open_issue_age_test.go
git commit -m "feat: open_issue_age distribution metric (age buckets)"
```

---

## Task 12: code_churn metric (time-series)

**Files:**
- Create: `internal/metrics/code_churn.go`, `internal/metrics/code_churn_test.go`

`code_churn` is a **time-series** of churn (additions + deletions) per day over the window, read from `daily_repo_stats`. Dense, zero-filled.

- [ ] **Step 1: Write the failing test**

`internal/metrics/code_churn_test.go`:
```go
package metrics

import (
	"context"
	"testing"
)

func TestCodeChurnSeries(t *testing.T) {
	src := &fakeSource{daily: []DailyRepoStatsRow{
		{Date: "2026-03-01", Additions: 10, Deletions: 2},
		{Date: "2026-03-02", Additions: 3, Deletions: 1},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-02"}
	res, err := codeChurn{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindTimeSeries || len(res.Series) != 2 {
		t.Fatalf("res = %+v", res)
	}
	if res.Series[0].Value != 12 || res.Series[1].Value != 4 {
		t.Fatalf("churn values = %v", res.Series)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run 'TestCodeChurn' -v`
Expected: FAIL — `undefined: codeChurn`.

- [ ] **Step 3: Write minimal implementation**

`internal/metrics/code_churn.go`:
```go
package metrics

import "context"

// codeChurn is a time-series of churn (additions + deletions)/day over the window.
type codeChurn struct{}

func (codeChurn) Key() string { return "code_churn" }

func (codeChurn) Compute(ctx context.Context, src Source, repoID int64, w Window, opts Opts) (Result, error) {
	rows, err := src.DailyRepoStats(ctx, repoID, w.From, w.To)
	if err != nil {
		return Result{}, err
	}
	byDate := make(map[string]int64, len(rows))
	for _, r := range rows {
		byDate[r.Date] = r.Additions + r.Deletions
	}
	dates, err := w.Dates()
	if err != nil {
		return Result{}, err
	}
	series := make([]Point, 0, len(dates))
	for _, d := range dates {
		series = append(series, Point{Date: d, Value: float64(byDate[d])})
	}
	return TimeSeries("Code churn per day", series), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/ -run 'TestCodeChurn' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/code_churn.go internal/metrics/code_churn_test.go
git commit -m "feat: code_churn time-series metric"
```

---

## Task 13: comment_volume metric (time-series)

**Files:**
- Create: `internal/metrics/comment_volume.go`, `internal/metrics/comment_volume_test.go`

`comment_volume` is a **time-series** of comments per day over the window, read from `daily_repo_stats.comments`. Dense, zero-filled.

- [ ] **Step 1: Write the failing test**

`internal/metrics/comment_volume_test.go`:
```go
package metrics

import (
	"context"
	"testing"
)

func TestCommentVolumeSeries(t *testing.T) {
	src := &fakeSource{daily: []DailyRepoStatsRow{
		{Date: "2026-03-01", Comments: 7},
		{Date: "2026-03-03", Comments: 2},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-03"}
	res, err := commentVolume{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindTimeSeries || len(res.Series) != 3 {
		t.Fatalf("res = %+v", res)
	}
	if res.Series[0].Value != 7 || res.Series[1].Value != 0 || res.Series[2].Value != 2 {
		t.Fatalf("comment values = %v", res.Series)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run 'TestCommentVolume' -v`
Expected: FAIL — `undefined: commentVolume`.

- [ ] **Step 3: Write minimal implementation**

`internal/metrics/comment_volume.go`:
```go
package metrics

import "context"

// commentVolume is a time-series of comments/day over the window.
type commentVolume struct{}

func (commentVolume) Key() string { return "comment_volume" }

func (commentVolume) Compute(ctx context.Context, src Source, repoID int64, w Window, opts Opts) (Result, error) {
	rows, err := src.DailyRepoStats(ctx, repoID, w.From, w.To)
	if err != nil {
		return Result{}, err
	}
	byDate := make(map[string]int64, len(rows))
	for _, r := range rows {
		byDate[r.Date] = r.Comments
	}
	dates, err := w.Dates()
	if err != nil {
		return Result{}, err
	}
	series := make([]Point, 0, len(dates))
	for _, d := range dates {
		series = append(series, Point{Date: d, Value: float64(byDate[d])})
	}
	return TimeSeries("Comments per day", series), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/ -run 'TestCommentVolume' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/comment_volume.go internal/metrics/comment_volume_test.go
git commit -m "feat: comment_volume time-series metric"
```

---

## Task 14: contributor_leaderboard metric (leaderboard)

**Files:**
- Create: `internal/metrics/contributor_leaderboard.go`, `internal/metrics/contributor_leaderboard_test.go`

`contributor_leaderboard` is a **leaderboard** Result: per-contributor commits/additions/deletions summed over the window from `daily_contributor_stats`, ranked by commits descending (ties broken by login for determinism). Reads the contributor aggregate (which has no bot flag), so `exclude_bots` filters by `IsBot(login)` heuristic at the metric layer — but since aggregates are login-keyed and bots can be detected from the login, the metric drops logins where the login looks like a bot (ends in `[bot]`). This keeps the leaderboard honest without an extra store column.

> Bot detection at this layer mirrors `githubapi.IsBot` (login ends in `[bot]`). To avoid `metrics` importing `githubapi` (a fetch package — a layering smell), the metric uses a tiny local `looksLikeBot(login)` with the same `[bot]`-suffix rule. The duration/open-issue metrics already filter via the store's `is_bot` column; only the contributor leaderboard, which reads the login-keyed aggregate, needs this login heuristic.

- [ ] **Step 1: Write the failing test**

`internal/metrics/contributor_leaderboard_test.go`:
```go
package metrics

import (
	"context"
	"testing"
)

func TestContributorLeaderboardRanking(t *testing.T) {
	src := &fakeSource{contrib: []DailyContribRow{
		{Date: "2026-03-01", Login: "neo", Commits: 3, Additions: 30, Deletions: 5},
		{Date: "2026-03-02", Login: "neo", Commits: 2, Additions: 10, Deletions: 1},
		{Date: "2026-03-01", Login: "trinity", Commits: 4, Additions: 8, Deletions: 0},
		{Date: "2026-03-01", Login: "dependabot[bot]", Commits: 9, Additions: 100, Deletions: 0},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-31"}

	res, err := contributorLeaderboard{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindLeaderboard {
		t.Fatalf("kind = %v", res.Kind)
	}
	// Incl bots: dependabot (9) > neo (5) > trinity (4).
	if len(res.Rows) != 3 {
		t.Fatalf("rows = %+v", res.Rows)
	}
	if res.Rows[0].Login != "dependabot[bot]" || res.Rows[0].Commits != 9 {
		t.Fatalf("row0 = %+v", res.Rows[0])
	}
	if res.Rows[1].Login != "neo" || res.Rows[1].Commits != 5 || res.Rows[1].Additions != 40 || res.Rows[1].Deletions != 6 {
		t.Fatalf("row1 = %+v", res.Rows[1])
	}

	// exclude_bots drops dependabot → neo (5) > trinity (4).
	res2, _ := contributorLeaderboard{}.Compute(context.Background(), src, 1, w, Opts{ExcludeBots: true})
	if len(res2.Rows) != 2 || res2.Rows[0].Login != "neo" {
		t.Fatalf("excl bots rows = %+v", res2.Rows)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run 'TestContributorLeaderboard' -v`
Expected: FAIL — `undefined: contributorLeaderboard`.

- [ ] **Step 3: Write minimal implementation**

`internal/metrics/contributor_leaderboard.go`:
```go
package metrics

import (
	"context"
	"sort"
	"strings"
)

// contributorLeaderboard ranks contributors by commits (then additions, then
// login) over the window, from the login-keyed daily contributor aggregate.
type contributorLeaderboard struct{}

func (contributorLeaderboard) Key() string { return "contributor_leaderboard" }

// looksLikeBot mirrors githubapi.IsBot's suffix rule without importing the fetch
// package (keeps the metrics layer free of githubapi).
func looksLikeBot(login string) bool {
	return strings.HasSuffix(login, "[bot]")
}

func (contributorLeaderboard) Compute(ctx context.Context, src Source, repoID int64, w Window, opts Opts) (Result, error) {
	rows, err := src.DailyContributorStats(ctx, repoID, w.From, w.To)
	if err != nil {
		return Result{}, err
	}
	agg := make(map[string]*LeaderRow)
	order := make([]string, 0)
	for _, r := range rows {
		if opts.ExcludeBots && looksLikeBot(r.Login) {
			continue
		}
		lr, ok := agg[r.Login]
		if !ok {
			lr = &LeaderRow{Login: r.Login}
			agg[r.Login] = lr
			order = append(order, r.Login)
		}
		lr.Commits += r.Commits
		lr.Additions += r.Additions
		lr.Deletions += r.Deletions
	}
	out := make([]LeaderRow, 0, len(order))
	for _, login := range order {
		out = append(out, *agg[login])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Commits != out[j].Commits {
			return out[i].Commits > out[j].Commits
		}
		if out[i].Additions != out[j].Additions {
			return out[i].Additions > out[j].Additions
		}
		return out[i].Login < out[j].Login
	})
	return Leaderboard("Contributor leaderboard", out), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/ -run 'TestContributorLeaderboard' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/contributor_leaderboard.go internal/metrics/contributor_leaderboard_test.go
git commit -m "feat: contributor_leaderboard leaderboard metric"
```

---

## Task 15: DefaultRegistry — register every shipped metric

**Files:**
- Create: `internal/metrics/default.go`, `internal/metrics/default_test.go`

`DefaultRegistry()` builds a `Registry` with every Extended metric registered. This is the single seam the API layer calls — one new file + one `Register` line is all it takes to ship a new stat. The test asserts the full key set and computes the whole registry against a real-ish fake to prove no metric panics.

- [ ] **Step 1: Write the failing test**

`internal/metrics/default_test.go`:
```go
package metrics

import (
	"context"
	"testing"
)

func TestDefaultRegistryKeys(t *testing.T) {
	reg := DefaultRegistry()
	keys := reg.Keys()
	want := []string{
		"code_churn", "comment_volume", "commit_rate", "contributor_leaderboard",
		"issue_lifetime", "open_issue_age", "pr_throughput", "review_latency", "time_to_merge",
	}
	if len(keys) != len(want) {
		t.Fatalf("keys = %v (%d), want %d", keys, len(keys), len(want))
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys[%d] = %q, want %q", i, keys[i], want[i])
		}
	}
}

func TestDefaultRegistryComputeAll(t *testing.T) {
	reg := DefaultRegistry()
	src := &fakeSource{
		daily: []DailyRepoStatsRow{{Date: "2026-03-01", Commits: 1, Additions: 2, Deletions: 1, PRsMerged: 1, Comments: 3}},
		contrib: []DailyContribRow{{Date: "2026-03-01", Login: "neo", Commits: 1, Additions: 2, Deletions: 1}},
		mergedPRs: []prRow{{row: PRDurationRow{Number: 1, CreatedAt: mustTime("2026-03-01T00:00:00Z"), MergedAt: mustTime("2026-03-01T06:00:00Z")}}},
		reviews: []reviewRow{{row: ReviewLatencyRow{Number: 1, CreatedAt: mustTime("2026-03-01T00:00:00Z"), FirstReviewAt: mustTime("2026-03-01T02:00:00Z")}}},
		closedIssues: []issueRow{{row: IssueLifetimeRow{Number: 1, CreatedAt: mustTime("2026-03-01T00:00:00Z"), ClosedAt: mustTime("2026-03-02T00:00:00Z")}}},
		openIssues: []openRow{{row: OpenIssueRow{Number: 2, CreatedAt: mustTime("2026-03-15T00:00:00Z")}}},
	}
	w := Window{From: "2026-03-01", To: "2026-03-31"}
	out, err := reg.Compute(context.Background(), src, 1, nil, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 9 {
		t.Fatalf("computed %d metrics, want 9", len(out))
	}
	// Spot-check a representative of each kind.
	if out["commit_rate"].Kind != KindTimeSeries {
		t.Fatalf("commit_rate kind = %v", out["commit_rate"].Kind)
	}
	if out["time_to_merge"].Kind != KindScalar {
		t.Fatalf("time_to_merge kind = %v", out["time_to_merge"].Kind)
	}
	if out["open_issue_age"].Kind != KindBuckets {
		t.Fatalf("open_issue_age kind = %v", out["open_issue_age"].Kind)
	}
	if out["contributor_leaderboard"].Kind != KindLeaderboard {
		t.Fatalf("contributor_leaderboard kind = %v", out["contributor_leaderboard"].Kind)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/ -run 'TestDefaultRegistry' -v`
Expected: FAIL — `undefined: DefaultRegistry`.

- [ ] **Step 3: Write minimal implementation**

`internal/metrics/default.go`:
```go
package metrics

// DefaultRegistry returns a Registry with every shipped Extended metric (spec §7).
// Adding a stat is one new file + one Register line here.
func DefaultRegistry() *Registry {
	reg := NewRegistry()
	reg.Register(commitRate{})
	reg.Register(prThroughput{})
	reg.Register(timeToMerge{})
	reg.Register(reviewLatency{})
	reg.Register(issueLifetime{})
	reg.Register(openIssueAge{})
	reg.Register(codeChurn{})
	reg.Register(commentVolume{})
	reg.Register(contributorLeaderboard{})
	return reg
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/ -run 'TestDefaultRegistry' -v && go test ./internal/metrics/ -v`
Expected: PASS (the whole `metrics` package is green).

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/default.go internal/metrics/default_test.go
git commit -m "feat: DefaultRegistry wiring every Extended metric"
```

---

## Task 16: HTTP metrics endpoint — GET /api/repos/{id}/metrics

**Files:**
- Create: `internal/api/metrics.go`, `internal/api/metrics_test.go`
- Modify: `internal/api/server.go`

This adds `GET /api/repos/{id}/metrics?keys=<csv>&window=30d&exclude_bots=true`, auth-gated and **authorized via `IsTracked`** (404 when the caller does not track the repo). The handler builds a `metrics.Window` (injected `now` via a server field so tests are deterministic), parses `keys`/`exclude_bots`, runs the registry's `Compute`, and returns `{ "<key>": <Result>, ... }`. The registry is built once in `NewServer` (it only needs the `Source`, which the store already is). `NewServer`'s **signature is unchanged**.

- [ ] **Step 1: Add the registry + injectable clock to the Server and mount the route**

In `internal/api/server.go`, add fields to the `Server` struct and initialize them in `NewServer`, then mount the metrics route. Apply these three edits:

Add to the `Server` struct (alongside the existing M3 fields):
```go
	registry *metrics.Registry
	now      func() time.Time
```

In `NewServer`, after `s := &Server{...}`, initialize the new fields (keep the existing assignment, then add):
```go
	s.registry = metrics.DefaultRegistry()
	if s.now == nil {
		s.now = time.Now
	}
```

Add the route inside the auth-gated `pr` group (next to the M3 repo routes):
```go
		pr.Get("/repos/{id}/metrics", s.repoMetrics)
```

Update the import block to add `time` and the `metrics` package:
```go
import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github-stats/internal/auth"
	"github-stats/internal/config"
	"github-stats/internal/crypto"
	"github-stats/internal/metrics"
	"github-stats/internal/store"
	"github-stats/internal/sync"
	"github-stats/web"
)
```

> The M3 `server.go` already imports `encoding/json`, `net/http`, chi, `auth`, `config`, `crypto`, `store`, `sync`, and `web`. This step adds only `time` and `metrics`. `s.now` is settable by tests (same package) before/after `NewServer` by assigning `srv.now = func() time.Time { ... }`; production leaves it as `time.Now`.

- [ ] **Step 2: Write the failing test**

`internal/api/metrics_test.go`:
```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github-stats/internal/store"
)

// seedMetricsRepo creates a tracked repo for user 1 with a small event set and
// recomputed aggregates, and pins the server clock for deterministic windows.
func seedMetricsRepo(t *testing.T, srv *Server, st *store.Store) int64 {
	t.Helper()
	ctx := context.Background()
	uid, _ := st.UpsertUser(ctx, &store.User{GitHubID: 1, Login: "neo"})
	if uid != 1 {
		t.Fatalf("expected user id 1, got %d", uid)
	}
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 10, FullName: "a/b", DefaultBranch: "main"})
	if err := st.TrackRepo(ctx, uid, repoID); err != nil {
		t.Fatal(err)
	}

	merged := tparse("2026-03-02T12:00:00Z") // created 03-02T00 → 12h
	if err := st.UpsertCommits(ctx, repoID, []store.Commit{
		{SHA: "c1", AuthorLogin: "neo", CommittedAt: tparse("2026-03-01T08:00:00Z"), Additions: 10, Deletions: 2},
		{SHA: "c2", AuthorLogin: "trinity", CommittedAt: tparse("2026-03-02T09:00:00Z"), Additions: 5, Deletions: 1},
		{SHA: "c3", AuthorLogin: "dependabot[bot]", CommittedAt: tparse("2026-03-02T10:00:00Z"), Additions: 3, Deletions: 0, IsBot: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertPullRequests(ctx, repoID, []store.PullRequest{
		{Number: 1, AuthorLogin: "neo", State: "MERGED", CreatedAt: tparse("2026-03-02T00:00:00Z"), MergedAt: &merged, CommentsCount: 1, Title: "pr"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.RecomputeDailyStats(ctx, repoID, "2026-03-01", "2026-03-02"); err != nil {
		t.Fatal(err)
	}
	// Pin the clock so the 30d window covers the fixture.
	srv.now = func() time.Time { return tparse("2026-03-15T00:00:00Z") }
	return repoID
}

func tparse(s string) time.Time {
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return v
}

func authedGet(t *testing.T, srv *Server, st *store.Store, path string) *httptest.ResponseRecorder {
	t.Helper()
	sess, _ := st.CreateSession(context.Background(), 1, time.Hour)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(&http.Cookie{Name: "gs_session", Value: sess.ID})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	return rec
}

func TestRepoMetricsReturnsRequestedKeys(t *testing.T) {
	srv, st := testServer(t)
	repoID := seedMetricsRepo(t, srv, st)

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/metrics?keys=commit_rate,time_to_merge&window=30d")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("keys returned = %v", out)
	}
	if out["commit_rate"]["kind"] != "time_series" {
		t.Fatalf("commit_rate kind = %v", out["commit_rate"]["kind"])
	}
	if out["time_to_merge"]["kind"] != "scalar" {
		t.Fatalf("time_to_merge kind = %v", out["time_to_merge"]["kind"])
	}
	if out["time_to_merge"]["value"].(float64) != 12 {
		t.Fatalf("time_to_merge value = %v, want 12", out["time_to_merge"]["value"])
	}
}

func TestRepoMetricsExcludeBots(t *testing.T) {
	srv, st := testServer(t)
	repoID := seedMetricsRepo(t, srv, st)

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/metrics?keys=contributor_leaderboard&window=30d&exclude_bots=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out map[string]struct {
		Kind string `json:"kind"`
		Rows []struct {
			Login string `json:"login"`
		} `json:"rows"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	for _, row := range out["contributor_leaderboard"].Rows {
		if row.Login == "dependabot[bot]" {
			t.Fatalf("exclude_bots leaked a bot: %v", out["contributor_leaderboard"].Rows)
		}
	}
}

func TestRepoMetricsAllKeysWhenOmitted(t *testing.T) {
	srv, st := testServer(t)
	repoID := seedMetricsRepo(t, srv, st)

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/metrics?window=30d")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 9 {
		t.Fatalf("default keys = %d, want 9", len(out))
	}
}

func TestRepoMetricsUnknownKey400(t *testing.T) {
	srv, st := testServer(t)
	repoID := seedMetricsRepo(t, srv, st)

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/metrics?keys=bogus")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRepoMetricsUntracked404(t *testing.T) {
	srv, st := testServer(t)
	_ = seedMetricsRepo(t, srv, st) // user 1 tracks repo 1
	ctx := context.Background()
	// A repo the user does NOT track.
	other, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 99, FullName: "x/y", DefaultBranch: "main"})

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(other, 10)+"/metrics?keys=commit_rate")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestRepoMetricsUnauthorized401(t *testing.T) {
	srv, st := testServer(t)
	repoID := seedMetricsRepo(t, srv, st)

	// No session cookie.
	req := httptest.NewRequest(http.MethodGet, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/metrics", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
```

- [ ] **Step 2b: Run test to verify it fails**

Run: `go test ./internal/api/ -run 'TestRepoMetrics' -v`
Expected: FAIL — `undefined: (*Server).repoMetrics` (or a compile error until Step 3 lands).

- [ ] **Step 3: Write the handler**

`internal/api/metrics.go`:
```go
package api

import (
	"net/http"
	"strings"

	"github-stats/internal/auth"
	"github-stats/internal/metrics"
)

// requireTracked resolves the {id} URL param, confirms the caller tracks the repo,
// and returns (userID, repoID, ok). It writes the appropriate error response and
// returns ok=false on any failure (401 unauthenticated, 400 bad id, 404 untracked).
func (s *Server) requireTracked(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return 0, 0, false
	}
	repoID, err := repoIDParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return 0, 0, false
	}
	tracked, err := s.store.IsTracked(r.Context(), u.ID, repoID)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return 0, 0, false
	}
	if !tracked {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return 0, 0, false
	}
	return u.ID, repoID, true
}

// parseKeys splits a comma-separated keys parameter, trimming blanks. Empty → nil
// (registry computes all keys).
func parseKeys(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// repoMetrics handles GET /api/repos/{id}/metrics?keys=&window=&exclude_bots=.
func (s *Server) repoMetrics(w http.ResponseWriter, r *http.Request) {
	_, repoID, ok := s.requireTracked(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	keys := parseKeys(q.Get("keys"))
	opts := metrics.Opts{ExcludeBots: q.Get("exclude_bots") == "true"}

	win, err := metrics.ParseWindow(r.Context(), q.Get("window"), repoID, s.store, s.now)
	if err != nil {
		http.Error(w, "bad window: "+err.Error(), http.StatusBadRequest)
		return
	}
	out, err := s.registry.Compute(r.Context(), s.store, repoID, keys, win, opts)
	if err != nil {
		// Unknown metric key → 400; anything else is a 500.
		if strings.HasPrefix(err.Error(), "unknown metric") {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "compute failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/ -run 'TestRepoMetrics' -v`
Expected: PASS (all six `TestRepoMetrics*` cases).

- [ ] **Step 5: Commit**

```bash
git add internal/api/server.go internal/api/metrics.go internal/api/metrics_test.go
git commit -m "feat: GET /api/repos/{id}/metrics endpoint (registry-backed)"
```

---

## Task 17: Overview bundle — GET /api/repos/{id}

**Files:**
- Modify: `internal/api/metrics.go`, `internal/api/server.go`
- Add tests to: `internal/api/metrics_test.go`

`GET /api/repos/{id}` returns the **overview bundle** for the repo card / details panel (spec §8): repo metadata plus the headline numbers — open issues, open PRs, contributors (excl. bots), commit/issue/PR rates (per-day averages over the window), release count, and last refresh. It composes store read methods + repo metadata; the window defaults to 30d (the card's at-a-glance horizon) but honors `?window=` and `?exclude_bots=`.

> This route reuses the `{id}` path. To avoid clashing with M3's `GET /api/repos` (the list) and the `/repos/{id}/...` sub-routes, it mounts as `pr.Get("/repos/{id}", s.repoOverview)` — chi distinguishes the exact `{id}` segment from the list path and the deeper `/{id}/metrics` etc.

- [ ] **Step 1: Mount the route**

In `internal/api/server.go`, add inside the auth-gated `pr` group:
```go
		pr.Get("/repos/{id}", s.repoOverview)
```

- [ ] **Step 2: Write the failing test (append to `metrics_test.go`)**

```go
func TestRepoOverviewBundle(t *testing.T) {
	srv, st := testServer(t)
	repoID := seedMetricsRepo(t, srv, st)

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(repoID, 10)+"?window=30d&exclude_bots=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var ov struct {
		ID            int64   `json:"id"`
		FullName      string  `json:"full_name"`
		DefaultBranch string  `json:"default_branch"`
		OpenIssues    int64   `json:"open_issues"`
		OpenPRs       int64   `json:"open_prs"`
		Contributors  int64   `json:"contributors"`
		CommitRate    float64 `json:"commit_rate"`
		IssueRate     float64 `json:"issue_rate"`
		PRRate        float64 `json:"pr_rate"`
		Releases      int64   `json:"releases"`
		SyncStatus    string  `json:"sync_status"`
		LastSyncedAt  *string `json:"last_synced_at"`
		WindowFrom    string  `json:"window_from"`
		WindowTo      string  `json:"window_to"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ov); err != nil {
		t.Fatal(err)
	}
	if ov.ID != repoID || ov.FullName != "a/b" {
		t.Fatalf("overview meta = %+v", ov)
	}
	// As of window end (2026-03-15), no open issues/PRs in the fixture.
	if ov.OpenIssues != 0 || ov.OpenPRs != 0 {
		t.Fatalf("open counts: issues=%d prs=%d", ov.OpenIssues, ov.OpenPRs)
	}
	// Contributors excl bots in window: neo + trinity = 2 (dependabot excluded).
	if ov.Contributors != 2 {
		t.Fatalf("contributors = %d, want 2", ov.Contributors)
	}
	// commit_rate = 3 commits / 30 days window. window 02-13..03-15 inclusive = 31 days.
	if ov.CommitRate <= 0 {
		t.Fatalf("commit_rate = %v, want > 0", ov.CommitRate)
	}
	if ov.WindowTo != "2026-03-15" {
		t.Fatalf("window_to = %q, want 2026-03-15", ov.WindowTo)
	}
}

func TestRepoOverviewUntracked404(t *testing.T) {
	srv, st := testServer(t)
	_ = seedMetricsRepo(t, srv, st)
	ctx := context.Background()
	other, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 77, FullName: "p/q", DefaultBranch: "main"})

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(other, 10))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/api/ -run 'TestRepoOverview' -v`
Expected: FAIL — `undefined: (*Server).repoOverview`.

- [ ] **Step 4: Write the handler (append to `metrics.go`)**

Add these imports to `internal/api/metrics.go`'s import block (it currently imports `net/http`, `strings`, `auth`, `metrics`):
```go
	"context"

	"github-stats/internal/store"
```
so the full block becomes:
```go
import (
	"context"
	"net/http"
	"strings"

	"github-stats/internal/auth"
	"github-stats/internal/metrics"
	"github-stats/internal/store"
)
```

Then append:
```go
// overviewJSON is the repo-card / details bundle (spec §8). M5 renders these.
type overviewJSON struct {
	ID            int64   `json:"id"`
	FullName      string  `json:"full_name"`
	IsPrivate     bool    `json:"is_private"`
	DefaultBranch string  `json:"default_branch"`
	Description   string  `json:"description"`
	Stargazers    int64   `json:"stargazers"`
	Forks         int64   `json:"forks"`
	OpenIssues    int64   `json:"open_issues"`
	OpenPRs       int64   `json:"open_prs"`
	Contributors  int64   `json:"contributors"`
	CommitRate    float64 `json:"commit_rate"` // commits/day over the window
	IssueRate     float64 `json:"issue_rate"`  // issues opened/day over the window
	PRRate        float64 `json:"pr_rate"`     // PRs opened/day over the window
	Releases      int64   `json:"releases"`
	SyncStatus    string  `json:"sync_status"`
	LastSyncedAt  *string `json:"last_synced_at"`
	WindowFrom    string  `json:"window_from"`
	WindowTo      string  `json:"window_to"`
}

// repoOverview handles GET /api/repos/{id}: the repo metadata + headline numbers.
func (s *Server) repoOverview(w http.ResponseWriter, r *http.Request) {
	_, repoID, ok := s.requireTracked(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	q := r.URL.Query()
	excludeBots := q.Get("exclude_bots") == "true"

	win, err := metrics.ParseWindow(ctx, q.Get("window"), repoID, s.store, s.now)
	if err != nil {
		http.Error(w, "bad window: "+err.Error(), http.StatusBadRequest)
		return
	}
	repo, err := s.store.GetRepo(ctx, repoID)
	if err != nil {
		http.Error(w, "repo lookup failed", http.StatusInternalServerError)
		return
	}
	asOf, err := win.ToTime()
	if err != nil {
		http.Error(w, "bad window", http.StatusInternalServerError)
		return
	}

	ov, err := s.buildOverview(ctx, repo, repoID, win, asOf, excludeBots)
	if err != nil {
		http.Error(w, "overview failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ov)
}

// buildOverview composes the overview bundle from store reads + the window.
func (s *Server) buildOverview(ctx context.Context, repo *store.Repo, repoID int64, win metrics.Window, asOf timeTime, excludeBots bool) (overviewJSON, error) {
	openIssues, err := s.store.CountOpenIssues(ctx, repoID, asOf, excludeBots)
	if err != nil {
		return overviewJSON{}, err
	}
	openPRs, err := s.store.CountOpenPRs(ctx, repoID, asOf, excludeBots)
	if err != nil {
		return overviewJSON{}, err
	}
	contributors, err := s.store.CountContributors(ctx, repoID, win.From, win.To, excludeBots)
	if err != nil {
		return overviewJSON{}, err
	}
	releases, err := s.store.CountReleases(ctx, repoID)
	if err != nil {
		return overviewJSON{}, err
	}
	daily, err := s.store.DailyRepoStats(ctx, repoID, win.From, win.To)
	if err != nil {
		return overviewJSON{}, err
	}
	dates, err := win.Dates()
	if err != nil {
		return overviewJSON{}, err
	}
	days := float64(len(dates))
	if days == 0 {
		days = 1
	}
	var commits, issuesOpened, prsOpened int64
	for _, d := range daily {
		commits += d.Commits
		issuesOpened += d.IssuesOpened
		prsOpened += d.PRsOpened
	}

	ov := overviewJSON{
		ID:            repoID,
		FullName:      repo.FullName,
		IsPrivate:     repo.IsPrivate,
		DefaultBranch: repo.DefaultBranch,
		Description:   repo.Description,
		Stargazers:    repo.Stargazers,
		Forks:         repo.Forks,
		OpenIssues:    openIssues,
		OpenPRs:       openPRs,
		Contributors:  contributors,
		CommitRate:    float64(commits) / days,
		IssueRate:     float64(issuesOpened) / days,
		PRRate:        float64(prsOpened) / days,
		Releases:      releases,
		WindowFrom:    win.From,
		WindowTo:      win.To,
	}
	if ss, err := s.store.GetSyncState(ctx, repoID); err == nil && ss != nil {
		ov.SyncStatus = ss.Status
		if ss.LastBackfillAt != nil {
			formatted := ss.LastBackfillAt.UTC().Format("2006-01-02T15:04:05Z07:00")
			ov.LastSyncedAt = &formatted
		}
	}
	return ov, nil
}
```

> `buildOverview` takes `asOf timeTime` — define the alias `type timeTime = time.Time` once, OR simply use `time.Time` directly and add `"time"` to the import block. The cleaner choice: add `"time"` to the import block and change the parameter type to `time.Time`. Use that. (The alias note is here only to flag that `metrics.Window.ToTime()` returns `time.Time`.)

So the final import block for `metrics.go` is:
```go
import (
	"context"
	"net/http"
	"strings"
	"time"

	"github-stats/internal/auth"
	"github-stats/internal/metrics"
	"github-stats/internal/store"
)
```
and the helper signature is:
```go
func (s *Server) buildOverview(ctx context.Context, repo *store.Repo, repoID int64, win metrics.Window, asOf time.Time, excludeBots bool) (overviewJSON, error) {
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/api/ -run 'TestRepoOverview' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/server.go internal/api/metrics.go internal/api/metrics_test.go
git commit -m "feat: GET /api/repos/{id} overview bundle endpoint"
```

---

## Task 18: Latest-items lists — GET /api/repos/{id}/latest/{kind}

**Files:**
- Modify: `internal/api/metrics.go`, `internal/api/server.go`
- Add tests to: `internal/api/metrics_test.go`

`GET /api/repos/{id}/latest/{commits|prs|issues}?limit=` returns the newest items for the repo-detail "latest" lists (spec §8). Auth-gated + `IsTracked`. `limit` defaults to 20, capped at 100. Unknown `{kind}` → 404. Reads `LatestCommits`/`LatestPRs`/`LatestIssues`.

- [ ] **Step 1: Mount the route**

In `internal/api/server.go`, add inside the auth-gated `pr` group:
```go
		pr.Get("/repos/{id}/latest/{kind}", s.repoLatest)
```

- [ ] **Step 2: Write the failing test (append to `metrics_test.go`)**

```go
func TestRepoLatestCommits(t *testing.T) {
	srv, st := testServer(t)
	repoID := seedMetricsRepo(t, srv, st)

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/latest/commits?limit=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("commits = %d, want 2", len(out))
	}
	// Newest first: c3 (03-02T10) then c2 (03-02T09).
	if out[0]["sha"] != "c3" {
		t.Fatalf("first sha = %v, want c3", out[0]["sha"])
	}
}

func TestRepoLatestPRsAndIssues(t *testing.T) {
	srv, st := testServer(t)
	repoID := seedMetricsRepo(t, srv, st)

	recPRs := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/latest/prs")
	if recPRs.Code != http.StatusOK {
		t.Fatalf("prs status = %d", recPRs.Code)
	}
	var prs []map[string]any
	json.Unmarshal(recPRs.Body.Bytes(), &prs)
	if len(prs) != 1 || prs[0]["number"].(float64) != 1 || prs[0]["state"] != "MERGED" {
		t.Fatalf("prs = %v", prs)
	}

	recIss := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/latest/issues")
	if recIss.Code != http.StatusOK {
		t.Fatalf("issues status = %d", recIss.Code)
	}
	var iss []map[string]any
	json.Unmarshal(recIss.Body.Bytes(), &iss)
	if len(iss) != 0 {
		t.Fatalf("issues = %v, want 0 (fixture has none)", iss)
	}
}

func TestRepoLatestUnknownKind404(t *testing.T) {
	srv, st := testServer(t)
	repoID := seedMetricsRepo(t, srv, st)

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/latest/bogus")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestRepoLatestUntracked404(t *testing.T) {
	srv, st := testServer(t)
	_ = seedMetricsRepo(t, srv, st)
	ctx := context.Background()
	other, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 88, FullName: "m/n", DefaultBranch: "main"})

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(other, 10)+"/latest/commits")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/api/ -run 'TestRepoLatest' -v`
Expected: FAIL — `undefined: (*Server).repoLatest`.

- [ ] **Step 4: Write the handler (append to `metrics.go`)**

This adds `strconv` and `chi` to the import block. Update the import block to:
```go
import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github-stats/internal/auth"
	"github-stats/internal/metrics"
	"github-stats/internal/store"
)
```

Then append:
```go
// commitJSON / prJSON / issueJSON are the wire shapes for the latest-items lists.
type commitJSON struct {
	SHA          string `json:"sha"`
	AuthorLogin  string `json:"author_login"`
	CommittedAt  string `json:"committed_at"`
	Additions    int64  `json:"additions"`
	Deletions    int64  `json:"deletions"`
	IsBot        bool   `json:"is_bot"`
	MsgFirstLine string `json:"msg_first_line"`
}

type prJSON struct {
	Number        int64   `json:"number"`
	AuthorLogin   string  `json:"author_login"`
	State         string  `json:"state"`
	CreatedAt     string  `json:"created_at"`
	MergedAt      *string `json:"merged_at"`
	ClosedAt      *string `json:"closed_at"`
	CommentsCount int64   `json:"comments_count"`
	IsBot         bool    `json:"is_bot"`
	Title         string  `json:"title"`
}

type issueJSON struct {
	Number        int64   `json:"number"`
	AuthorLogin   string  `json:"author_login"`
	State         string  `json:"state"`
	CreatedAt     string  `json:"created_at"`
	ClosedAt      *string `json:"closed_at"`
	CommentsCount int64   `json:"comments_count"`
	IsBot         bool    `json:"is_bot"`
	Title         string  `json:"title"`
}

const isoLayout = "2006-01-02T15:04:05Z07:00"

func fmtTime(t time.Time) string { return t.UTC().Format(isoLayout) }

func fmtTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(isoLayout)
	return &s
}

// parseLimit reads ?limit= (default 20, min 1, max 100).
func parseLimit(raw string) int {
	const def, max = 20, 100
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

// repoLatest handles GET /api/repos/{id}/latest/{commits|prs|issues}?limit=.
func (s *Server) repoLatest(w http.ResponseWriter, r *http.Request) {
	_, repoID, ok := s.requireTracked(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	limit := parseLimit(r.URL.Query().Get("limit"))

	switch chi.URLParam(r, "kind") {
	case "commits":
		rows, err := s.store.LatestCommits(ctx, repoID, limit)
		if err != nil {
			http.Error(w, "load failed", http.StatusInternalServerError)
			return
		}
		out := make([]commitJSON, 0, len(rows))
		for _, c := range rows {
			out = append(out, commitJSON{
				SHA: c.SHA, AuthorLogin: c.AuthorLogin, CommittedAt: fmtTime(c.CommittedAt),
				Additions: c.Additions, Deletions: c.Deletions, IsBot: c.IsBot, MsgFirstLine: c.MsgFirstLine,
			})
		}
		writeJSON(w, http.StatusOK, out)
	case "prs":
		rows, err := s.store.LatestPRs(ctx, repoID, limit)
		if err != nil {
			http.Error(w, "load failed", http.StatusInternalServerError)
			return
		}
		out := make([]prJSON, 0, len(rows))
		for _, p := range rows {
			out = append(out, prJSON{
				Number: p.Number, AuthorLogin: p.AuthorLogin, State: p.State, CreatedAt: fmtTime(p.CreatedAt),
				MergedAt: fmtTimePtr(p.MergedAt), ClosedAt: fmtTimePtr(p.ClosedAt),
				CommentsCount: p.CommentsCount, IsBot: p.IsBot, Title: p.Title,
			})
		}
		writeJSON(w, http.StatusOK, out)
	case "issues":
		rows, err := s.store.LatestIssues(ctx, repoID, limit)
		if err != nil {
			http.Error(w, "load failed", http.StatusInternalServerError)
			return
		}
		out := make([]issueJSON, 0, len(rows))
		for _, is := range rows {
			out = append(out, issueJSON{
				Number: is.Number, AuthorLogin: is.AuthorLogin, State: is.State, CreatedAt: fmtTime(is.CreatedAt),
				ClosedAt: fmtTimePtr(is.ClosedAt), CommentsCount: is.CommentsCount, IsBot: is.IsBot, Title: is.Title,
			})
		}
		writeJSON(w, http.StatusOK, out)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown kind"})
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/api/ -run 'TestRepoLatest' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/server.go internal/api/metrics.go internal/api/metrics_test.go
git commit -m "feat: GET /api/repos/{id}/latest/{commits|prs|issues} endpoint"
```

---

## Task 19: Full-suite verification

**Files:**
- None (verification only)

- [ ] **Step 1: Run the entire test suite**

Run: `go build ./... && go test ./...`
Expected: build succeeds; every package PASSES — `config`, `crypto`, `store` (incl. new `reads_test.go`), `auth`, `githubapi`, `backfill`, `sync`, `metrics` (all metric units + registry + window + result + ema + default), and `api` (M1 `/api/me` + SPA fallback, M3 repo endpoints, and the new M4 metrics/overview/latest endpoints).

- [ ] **Step 2: Confirm no metric touches GitHub or HTTP (layering check)**

Run: `grep -rn "net/http\|githubapi" internal/metrics/`
Expected: **no matches** — the `metrics` package imports only `context`, `time`, `sort`, `math`/`strings`/`fmt`, `encoding/json` (none in compute paths), and `github-stats/internal/store` (for the row-type aliases in `source.go`). This proves the spec §4 boundary: metrics read only through the `Source` port.

- [ ] **Step 3: Confirm NewServer signature is unchanged from M3**

Run: `grep -n "func NewServer" internal/api/server.go`
Expected: `func NewServer(cfg config.Config, st *store.Store, authSvc *auth.Service, engine *sync.Engine, cipher *crypto.Cipher) *Server` — identical to M3. The registry is built internally; no caller (`main.go`, M1/M3 tests) changes.

- [ ] **Step 4: Commit (if any incidental fixes were needed)**

```bash
git add -A
git commit -m "test: full-suite green for M4 metrics registry + endpoints"
```

---

## Out of scope (later milestones)

M4 deliberately excludes the following; they belong to later plans and are **not** to be built here:

- **The React dashboard** that renders these metrics — overview cards, repo-detail charts (uPlot time-series, the buckets bar, the leaderboard table), the window selector, the exclude-bots toggle, the `/owner/repo` URL shortcut, and live SSE sync status — **M5**. M4 ships only the Go metrics engine + JSON endpoints the M5 frontend consumes; the "one renderer per Result kind" mapping is defined here (the `kind` tag) but built in M5.
- **Collections** (`collections` / `collection_repos` tables and `GET/POST /api/collections`, `DELETE /api/collections/{id}`), **save/load** of dashboards, the **optional PAT** credential, refined **bot-detection** lists, **rate-limit UX**, and **self-host hardening docs** — **M6**.
- No new sync/collector behavior: M4 reads the `daily_*` aggregates and event tables that M2 materialized and M3 keeps fresh. It adds **no** write paths, **no** GitHub calls, and **no** changes to the sync engine.

---

## Self-Review notes

- **Spec coverage:** every M4 bullet maps to a task. (1) `Metric` interface + `Registry` (`Register`/`Get`/`Keys`/`Compute`) → Task 4; narrow read-only `Source` port satisfied by `*store.Store` → Task 2 (+ compile-time `var _ Source = (*store.Store)(nil)` in Task 4); `Window` parsing (`30d|90d|6m|1y|all`, injected `now`, `"all"` from earliest) + `Opts{ExcludeBots}` → Task 2; the ~4 `Result` shapes (time-series/scalar/buckets/leaderboard) with a `kind` tag → Task 3; the Extended metric set each in its own file — `commit_rate`(6), `pr_throughput`(7), `time_to_merge`(8), `review_latency`(9), `issue_lifetime`(10), `open_issue_age`(11), `code_churn`(12), `comment_volume`(13), `contributor_leaderboard`(14) — plus the `ema` helper (5d/14d) → Task 5, and `DefaultRegistry` → Task 15. (2) store read methods backing the `Source` (`DailyRepoStats`, `DailyContributorStats`, `MergedPRDurations`, `ReviewLatencies`, `OpenIssuesAsOf`, `ClosedIssueLifetimes`, `LatestCommits/LatestPRs/LatestIssues`, `EarliestEventDate`, and the overview counts) → Task 1. (3) HTTP endpoints `GET /api/repos/{id}/metrics` → Task 16, `GET /api/repos/{id}` overview bundle → Task 17, `GET /api/repos/{id}/latest/{kind}` → Task 18 — all auth-gated and authorized via `IsTracked`.
- **No placeholders:** every code/test step contains complete, compilable Go and complete SQL. All nine metric files, the registry, the window parser, the result constructors, the EMA/mean/median math, the store read methods (with full parameterized SQL), and the three handlers are written out in full.
- **`NewServer` signature unchanged:** M4 builds the `Registry` inside `NewServer` from the already-injected `*store.Store` (which is the `Source`). The only `server.go` edits are two new struct fields (`registry`, `now`), their initialization, three route mounts, and two added imports (`time`, `metrics`). No caller changes — `main.go` and the M1/M3 test helpers stay as-is (Task 19 Step 3 asserts this).
- **Type consistency with M1/M2/M3:** reuses `store.Store/Repo/Commit/PullRequest/Issue/SyncState`, `store.GetRepo/GetSyncState/IsTracked/UpsertCommits/UpsertPullRequests/UpsertIssues/RecomputeDailyStats`, `auth.UserFromContext`, M3's `writeJSON`/`repoIDParam`/`Server.Router()`, and the `gs_session` cookie. New types introduced early (`Source`, `Window`, `Opts`, `Result`/`ResultKind`/`Point`/`Bucket`/`LeaderRow`, `Metric`, `Registry`, and the `store.*Row` read structs) are used unchanged downstream. The `metrics.DailyRepoStatsRow` etc. are **type aliases** for the `store.*Row` types, so `*store.Store` satisfies `metrics.Source` with zero adapter code.
- **Determinism (no flaky sleeps, no wall clock):** every metric unit test builds a hand-crafted `fakeSource` with exact RFC3339 timestamps and asserts exact values; the clock is injected via `ParseWindow(..., now)` and the `Server.now` field (pinned in the API tests' `seedMetricsRepo`). EMA and rate math are tested against known inputs/outputs (the span-5 EMA sequence is computed by hand in the test comment). No test calls `time.Now()` in an assertion path.
- **Layering enforced (spec §4):** the `metrics` package imports only the stdlib plus `github-stats/internal/store` (for the row-type aliases) — never `net/http` or `githubapi`. Task 19 Step 2 greps to prove it. The bot filter is applied at the store SQL layer (`is_bot` column) for duration/open-issue metrics; the contributor leaderboard, reading the login-keyed aggregate, uses a local `looksLikeBot` suffix check rather than importing `githubapi` (documented in Task 14).
- **HTTP correctness & authorization:** all three read endpoints are mounted inside M3's auth-gated `/api` group (so unknown `/api/*` still gets M3's JSON-404), and each calls `requireTracked` which returns 401 (no session), 400 (bad id), or 404 (untracked) before touching the store reads — tested explicitly per endpoint. `exclude_bots=true` and `window=` are honored and tested. The `{id}` overview route coexists with M3's `GET /api/repos` list and the deeper `/{id}/metrics|latest|refresh|sync/stream` routes (chi routes the exact segment).
- **Result shapes are JSON-marshal-ready:** `Result` uses `omitempty` so only the populated payload field appears, and each constructor normalizes `nil` slices to `[]` so the frontend never sees `null` where it expects an array. Each metric's natural shape is documented key→kind in the Public API surface below.

---

## What M5 will add (next plan)

- **React dashboard** (Vite SPA, embedded via `embed.FS`) that consumes these endpoints: an **Overview** page of repo cards (each card hydrated from `GET /api/repos/{id}` — open issues/PRs, contributors, rates, releases, last refresh) and a **Repo detail** page.
- **One renderer per Result `kind`**: a uPlot line chart for `time_series`, a stat tile for `scalar` (value + unit + count), a bar chart for `buckets` (open-issue-age), and a table for `leaderboard` (contributors) — driven by the `kind` tag M4 stamps on every `Result`.
- **Window selector** (30d/90d/6m/1y/all) and **exclude-bots toggle** wired to the `window`/`exclude_bots` query params; the **latest lists** (Commits/PRs/Issues sections) from `GET /api/repos/{id}/latest/{kind}`.
- The **`/owner/repo` URL shortcut** to a repo-detail view, and live **SSE sync status** (reusing M3's `GET /api/repos/{id}/sync/stream`).

---

## Public API surface M4 exposes

For the M5 plan to build the frontend against precisely. All under module `github-stats`.

**`internal/store` (package `store`) — new read methods (join existing `Store{DB *sql.DB}`):**
```go
type DailyRepoStatsRow struct { Date string; Commits, Additions, Deletions, PRsOpened, PRsMerged, PRsClosed, IssuesOpened, IssuesClosed, Comments, Releases, ActiveContrib int64 }
type DailyContribRow   struct { Date, Login string; Commits, Additions, Deletions int64 }
type PRDurationRow     struct { Number int64; CreatedAt, MergedAt time.Time }
type ReviewLatencyRow  struct { Number int64; CreatedAt, FirstReviewAt time.Time }
type IssueLifetimeRow  struct { Number int64; CreatedAt, ClosedAt time.Time }
type OpenIssueRow      struct { Number int64; CreatedAt time.Time }

func (s *Store) DailyRepoStats(ctx, repoID int64, fromDate, toDate string) ([]DailyRepoStatsRow, error)
func (s *Store) DailyContributorStats(ctx, repoID int64, fromDate, toDate string) ([]DailyContribRow, error)
func (s *Store) MergedPRDurations(ctx, repoID int64, fromDate, toDate string, excludeBots bool) ([]PRDurationRow, error)
func (s *Store) ReviewLatencies(ctx, repoID int64, fromDate, toDate string, excludeBots bool) ([]ReviewLatencyRow, error)
func (s *Store) ClosedIssueLifetimes(ctx, repoID int64, fromDate, toDate string, excludeBots bool) ([]IssueLifetimeRow, error)
func (s *Store) OpenIssuesAsOf(ctx, repoID int64, asOf time.Time, excludeBots bool) ([]OpenIssueRow, error)
func (s *Store) LatestCommits(ctx, repoID int64, limit int) ([]Commit, error)
func (s *Store) LatestPRs(ctx, repoID int64, limit int) ([]PullRequest, error)
func (s *Store) LatestIssues(ctx, repoID int64, limit int) ([]Issue, error)
func (s *Store) EarliestEventDate(ctx, repoID int64) (string, error) // 'YYYY-MM-DD'; ErrNotFound when no events
func (s *Store) CountOpenIssues(ctx, repoID int64, asOf time.Time, excludeBots bool) (int64, error)
func (s *Store) CountOpenPRs(ctx, repoID int64, asOf time.Time, excludeBots bool) (int64, error)
func (s *Store) CountContributors(ctx, repoID int64, fromDate, toDate string, excludeBots bool) (int64, error)
func (s *Store) CountReleases(ctx, repoID int64) (int64, error)
```

**`internal/metrics` (package `metrics`) — the modular statistics generator:**
```go
type Source interface { /* the 7 read methods above minus the Count*/Latest* helpers: DailyRepoStats, DailyContributorStats, MergedPRDurations, ReviewLatencies, ClosedIssueLifetimes, OpenIssuesAsOf, EarliestEventDate */ }
type Opts struct { ExcludeBots bool }

type Window struct { From, To string } // inclusive UTC 'YYYY-MM-DD'
func ParseWindow(ctx, spec string, repoID int64, src EarliestSource, now func() time.Time) (Window, error) // ""→30d; 30d|90d|6m|1y|all
func (w Window) Dates() ([]string, error)
func (w Window) ToTime() (time.Time, error)

type ResultKind string // "time_series" | "scalar" | "buckets" | "leaderboard"
type Point     struct { Date string `json:"date"`; Value float64 `json:"value"` }
type Bucket    struct { Label string `json:"label"`; Count int64 `json:"count"` }
type LeaderRow struct { Login string `json:"login"`; Commits, Additions, Deletions int64 }
type Result struct {
	Kind    ResultKind  `json:"kind"`
	Label   string      `json:"label,omitempty"`
	Series  []Point     `json:"series,omitempty"`   // time_series
	Value   *float64    `json:"value,omitempty"`    // scalar
	Unit    string      `json:"unit,omitempty"`     // scalar
	Count   *int64      `json:"count,omitempty"`    // scalar (sample size)
	Buckets []Bucket    `json:"buckets,omitempty"`  // buckets
	Rows    []LeaderRow `json:"rows,omitempty"`     // leaderboard
}
func TimeSeries(label string, series []Point) Result
func Scalar(label string, value float64, unit string, count int64) Result
func Buckets(label string, buckets []Bucket) Result
func Leaderboard(label string, rows []LeaderRow) Result

type Metric interface { Key() string; Compute(ctx, src Source, repoID int64, w Window, opts Opts) (Result, error) }
type Registry struct { /* unexported */ }
func NewRegistry() *Registry
func (r *Registry) Register(m Metric)
func (r *Registry) Get(key string) (Metric, bool)
func (r *Registry) Keys() []string // sorted
func (r *Registry) Compute(ctx, src Source, repoID int64, keys []string, w Window, opts Opts) (map[string]Result, error) // empty keys → all
func DefaultRegistry() *Registry // every shipped metric

func EMA(values []float64, span int) []float64 // 2/(span+1) smoothing, seeded with values[0]
```

**Metric key → Result kind (what the M5 frontend renders per key):**
| key | Result kind | reads | exclude_bots? | notes |
|-----|-------------|-------|---------------|-------|
| `commit_rate` | `time_series` | `daily_repo_stats.commits` | no (aggregate) | commits/day, dense (zero-filled) |
| `pr_throughput` | `time_series` | `daily_repo_stats.prs_merged` | no (aggregate) | PRs merged/day, dense |
| `code_churn` | `time_series` | `daily_repo_stats.additions+deletions` | no (aggregate) | churn/day, dense |
| `comment_volume` | `time_series` | `daily_repo_stats.comments` | no (aggregate) | comments/day, dense |
| `time_to_merge` | `scalar` | `pull_requests` (event) | **yes** | median hours (headline `value`), `unit:"hours"`, `count`=PRs; mean in `label` |
| `review_latency` | `scalar` | `pull_requests.first_review_at` (event) | **yes** | median hours to first review |
| `issue_lifetime` | `scalar` | `issues` (event) | **yes** | median hours create→close |
| `open_issue_age` | `buckets` | `issues` open as-of window end (event) | **yes** | buckets `<24h`,`<7d`,`<30d`,`<90d`,`<180d`,`older` (always all 6, ordered) |
| `contributor_leaderboard` | `leaderboard` | `daily_contributor_stats` | **yes** (`[bot]`-suffix) | rows ranked by commits desc, ties by additions then login |

**`internal/api` (package `api`) — new read endpoints (auth-gated under `/api`, authorized via `IsTracked`; `NewServer` signature UNCHANGED from M3):**
```go
func NewServer(cfg config.Config, st *store.Store, authSvc *auth.Service, engine *sync.Engine, cipher *crypto.Cipher) *Server // unchanged

// GET /api/repos/{id}/metrics?keys=<csv>&window=30d&exclude_bots=true
//   -> 200 { "<key>": <Result>, ... }   (keys omitted → all 9; unknown key → 400; untracked → 404; no session → 401)
//
// GET /api/repos/{id}?window=30d&exclude_bots=true   (overview bundle; untracked → 404)
//   -> 200 {
//        "id":int, "full_name":str, "is_private":bool, "default_branch":str, "description":str,
//        "stargazers":int, "forks":int,
//        "open_issues":int, "open_prs":int, "contributors":int,
//        "commit_rate":float, "issue_rate":float, "pr_rate":float,  (per-day averages over the window)
//        "releases":int, "sync_status":str, "last_synced_at":str|null,
//        "window_from":"YYYY-MM-DD", "window_to":"YYYY-MM-DD"
//      }
//
// GET /api/repos/{id}/latest/{commits|prs|issues}?limit=20   (default 20, max 100; unknown kind → 404; untracked → 404)
//   -> 200 [ {commit|pr|issue JSON} ]
//      commit: { "sha","author_login","committed_at","additions","deletions","is_bot","msg_first_line" }
//      pr:     { "number","author_login","state","created_at","merged_at"|null,"closed_at"|null,"comments_count","is_bot","title" }
//      issue:  { "number","author_login","state","created_at","closed_at"|null,"comments_count","is_bot","title" }
```
