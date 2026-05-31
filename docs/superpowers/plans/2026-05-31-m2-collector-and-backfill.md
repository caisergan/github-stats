# M2 — Collector & Single-Repo Backfill Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the data-collection foundation for `github-stats` and backfill **one repository end-to-end** into the SQLite store. This delivers: a new `0002` migration with the event + aggregate schema (spec §5), DAOs for those tables including an aggregate **recompute** that rebuilds daily stats from events, a rate-limit-aware `githubapi` package (GraphQL client, ETag conditional REST transport, dual REST/GraphQL budget manager, bot detection), and a `backfill` package whose `Run(...)` pages commits/PRs/issues/releases into the store, saves cursors after each page (resumable), and recomputes aggregates for touched dates. M2 is exercised programmatically by Go tests — **no HTTP route** (that arrives in M3).

**Architecture:** Builds directly on M1 (`docs/superpowers/plans/2026-05-30-m1-skeleton-and-auth.md`). The store keeps **events as the source of truth** and **materialized daily aggregates** recomputed from those events for any date range a sync touches (spec §5, §6, §12). `githubapi` is a self-contained collector: GraphQL replaces dozens of REST calls and lives in its **own 5,000-points/hr bucket**; conditional REST GETs (`If-None-Match` → `304`) **do not count against the rate limit** (spec §3). A `Budget` type tracks the REST and GraphQL buckets **separately** from response metadata. `backfill.Run` orchestrates: it delegates fetching to `githubapi`, writing to `store`, persists cursors to `sync_state` after every page so a rate-limit window or crash can resume, and upserts aggregates as pages land. Boundaries match spec §4: `backfill` orchestrates, `githubapi` fetches, `store` persists; `metrics` (M4) will read only aggregates.

**Tech Stack:** Go 1.25+, `modernc.org/sqlite` (driver `"sqlite"`, WAL, `db.SetMaxOpenConns(1)`), Go stdlib `net/http` + `encoding/json` (no GraphQL library — hand-rolled client), `net/http/httptest` for fake GitHub REST + GraphQL servers in tests. No new third-party dependencies are required.

---

## File Structure

```
github-stats/
├── internal/
│   ├── store/
│   │   ├── migrations/0002_collector.sql   # repos, commits, prs, issues, releases, daily_*, sync_state, etags
│   │   ├── repos.go                         # Repo + UpsertRepo/GetRepo/GetRepoByFullName
│   │   ├── repos_test.go
│   │   ├── events.go                        # Commit/PullRequest/Issue/Release + batch upserts
│   │   ├── events_test.go
│   │   ├── syncstate.go                     # SyncState + GetSyncState/UpsertSyncState
│   │   ├── syncstate_test.go
│   │   ├── etagcache.go                     # ETag cache: GetETag/PutETag
│   │   ├── etagcache_test.go
│   │   ├── aggregates.go                    # RecomputeDailyStats(repoID, from, to)
│   │   └── aggregates_test.go
│   ├── githubapi/
│   │   ├── budget.go                        # Budget: dual REST + GraphQL rate-limit tracker
│   │   ├── budget_test.go
│   │   ├── bot.go                           # IsBot(login)
│   │   ├── bot_test.go
│   │   ├── etagtransport.go                 # ETagTransport http.RoundTripper (conditional GETs)
│   │   ├── etagtransport_test.go
│   │   ├── graphql.go                       # Client.graphql() low-level POST + rateLimit decode
│   │   ├── graphql_test.go
│   │   ├── client.go                        # Client struct + NewClient + REST GET helper
│   │   ├── fetch.go                         # typed paged fetchers: repo meta, commits, PRs, issues, releases
│   │   └── fetch_test.go
│   └── backfill/
│       ├── backfill.go                      # Run(ctx, store, client, repoID)
│       └── backfill_test.go
```

> All files under `internal/store/` join the **existing** `package store`; all files under `internal/githubapi/` are `package githubapi`; `internal/backfill/` is `package backfill`. The M1 test helper `openTemp(t)` in `internal/store/store_test.go` is reused by every new store test.

---

## Task 1: 0002 migration — event & aggregate schema

**Files:**
- Create: `internal/store/migrations/0002_collector.sql`

This migration is picked up automatically by the M1 migration runner (it reads `//go:embed migrations/*.sql` and applies files in sorted filename order, tracking applied versions in `schema_migrations`). Column names match spec §5 exactly — M3/M4 depend on them.

- [ ] **Step 1: Write the migration SQL**

`internal/store/migrations/0002_collector.sql`:
```sql
-- Tracked repositories.
CREATE TABLE repos (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    github_id       INTEGER NOT NULL UNIQUE,
    full_name       TEXT    NOT NULL UNIQUE,   -- "owner/name"
    is_private      INTEGER NOT NULL DEFAULT 0,
    default_branch  TEXT    NOT NULL DEFAULT '',
    description     TEXT    NOT NULL DEFAULT '',
    stargazers      INTEGER NOT NULL DEFAULT 0,
    forks           INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Event tables (lean — source of truth).
CREATE TABLE commits (
    repo_id        INTEGER NOT NULL,
    sha            TEXT    NOT NULL,
    author_login   TEXT    NOT NULL DEFAULT '',
    committed_at   TIMESTAMP NOT NULL,
    additions      INTEGER NOT NULL DEFAULT 0,
    deletions      INTEGER NOT NULL DEFAULT 0,
    is_bot         INTEGER NOT NULL DEFAULT 0,
    msg_first_line TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (repo_id, sha),
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);
CREATE INDEX idx_commits_repo_date ON commits(repo_id, committed_at);

CREATE TABLE pull_requests (
    repo_id        INTEGER NOT NULL,
    number         INTEGER NOT NULL,
    author_login   TEXT    NOT NULL DEFAULT '',
    state          TEXT    NOT NULL,           -- 'OPEN' | 'CLOSED' | 'MERGED'
    created_at     TIMESTAMP NOT NULL,
    merged_at      TIMESTAMP,
    closed_at      TIMESTAMP,
    additions      INTEGER NOT NULL DEFAULT 0,
    deletions      INTEGER NOT NULL DEFAULT 0,
    changed_files  INTEGER NOT NULL DEFAULT 0,
    comments_count INTEGER NOT NULL DEFAULT 0,
    first_review_at TIMESTAMP,
    is_bot         INTEGER NOT NULL DEFAULT 0,
    title          TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (repo_id, number),
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);
CREATE INDEX idx_prs_repo_created ON pull_requests(repo_id, created_at);

CREATE TABLE issues (
    repo_id        INTEGER NOT NULL,
    number         INTEGER NOT NULL,
    author_login   TEXT    NOT NULL DEFAULT '',
    state          TEXT    NOT NULL,           -- 'OPEN' | 'CLOSED'
    created_at     TIMESTAMP NOT NULL,
    closed_at      TIMESTAMP,
    comments_count INTEGER NOT NULL DEFAULT 0,
    is_bot         INTEGER NOT NULL DEFAULT 0,
    title          TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (repo_id, number),
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);
CREATE INDEX idx_issues_repo_created ON issues(repo_id, created_at);

CREATE TABLE releases (
    repo_id        INTEGER NOT NULL,
    tag            TEXT    NOT NULL,
    name           TEXT    NOT NULL DEFAULT '',
    published_at   TIMESTAMP,
    author_login   TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (repo_id, tag),
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);

-- Materialized aggregates (charts read these).
CREATE TABLE daily_repo_stats (
    repo_id             INTEGER NOT NULL,
    date                TEXT    NOT NULL,       -- 'YYYY-MM-DD' (UTC)
    commits             INTEGER NOT NULL DEFAULT 0,
    additions           INTEGER NOT NULL DEFAULT 0,
    deletions           INTEGER NOT NULL DEFAULT 0,
    prs_opened          INTEGER NOT NULL DEFAULT 0,
    prs_merged          INTEGER NOT NULL DEFAULT 0,
    prs_closed          INTEGER NOT NULL DEFAULT 0,
    issues_opened       INTEGER NOT NULL DEFAULT 0,
    issues_closed       INTEGER NOT NULL DEFAULT 0,
    comments            INTEGER NOT NULL DEFAULT 0,
    releases            INTEGER NOT NULL DEFAULT 0,
    active_contributors INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (repo_id, date),
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);

CREATE TABLE daily_contributor_stats (
    repo_id    INTEGER NOT NULL,
    date       TEXT    NOT NULL,                -- 'YYYY-MM-DD' (UTC)
    login      TEXT    NOT NULL,
    commits    INTEGER NOT NULL DEFAULT 0,
    additions  INTEGER NOT NULL DEFAULT 0,
    deletions  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (repo_id, date, login),
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);

-- Sync bookkeeping (cursors per repo). sync_jobs is intentionally deferred to M3.
CREATE TABLE sync_state (
    repo_id          INTEGER PRIMARY KEY,
    last_commit_at   TIMESTAMP,
    last_pr_cursor   TEXT NOT NULL DEFAULT '',
    last_issue_cursor TEXT NOT NULL DEFAULT '',
    last_commit_cursor TEXT NOT NULL DEFAULT '',
    last_release_cursor TEXT NOT NULL DEFAULT '',
    last_backfill_at TIMESTAMP,
    status           TEXT NOT NULL DEFAULT '',  -- '' | 'backfilling' | 'complete'
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);

-- ETag cache for conditional REST GETs (304s are free; see spec §3).
CREATE TABLE etags (
    url           TEXT PRIMARY KEY,
    etag          TEXT NOT NULL,
    status        INTEGER NOT NULL,
    body          BLOB NOT NULL,
    last_modified TEXT NOT NULL DEFAULT '',
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

> Note: spec §5 lists `etags (user_id, url, etag, last_modified)`. M2's conditional transport caches the response **body + status** keyed by URL (so a `304` can return the cached payload), and the backfill is per-repo not per-user, so `user_id` is omitted here and the body/status columns are added. M3 may add a `user_id` column if per-user ETag scoping is needed; for M2 the URL already encodes the repo. This deviation is intentional and noted in Self-Review.

- [ ] **Step 2: Verify the migration applies (reuse M1 store tests)**

Run: `go test ./internal/store/ -run TestOpenAppliesMigrations -v`
Expected: PASS — `Open()` now applies `0001` then `0002`; the existing test still finds `users`/`credentials`/`sessions`. (We add a dedicated assertion for the new tables in Task 2's test.)

- [ ] **Step 3: Commit**

```bash
git add internal/store/migrations/0002_collector.sql
git commit -m "feat: 0002 migration for collector event + aggregate schema"
```

---

## Task 2: Repos DAO

**Files:**
- Create: `internal/store/repos.go`, `internal/store/repos_test.go`

- [ ] **Step 1: Write the failing test**

`internal/store/repos_test.go`:
```go
package store

import (
	"context"
	"testing"
)

func TestNewTablesExist(t *testing.T) {
	s := openTemp(t)
	for _, table := range []string{
		"repos", "commits", "pull_requests", "issues", "releases",
		"daily_repo_stats", "daily_contributor_stats", "sync_state", "etags",
	} {
		var name string
		err := s.DB.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestUpsertRepoInsertsThenUpdates(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	id, err := s.UpsertRepo(ctx, &Repo{
		GitHubID: 100, FullName: "octocat/hello", IsPrivate: true,
		DefaultBranch: "main", Description: "first", Stargazers: 3, Forks: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	id2, err := s.UpsertRepo(ctx, &Repo{
		GitHubID: 100, FullName: "octocat/hello", IsPrivate: false,
		DefaultBranch: "trunk", Description: "second", Stargazers: 9, Forks: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id {
		t.Fatalf("upsert created new row: %d != %d", id2, id)
	}

	r, err := s.GetRepo(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if r.DefaultBranch != "trunk" || r.Description != "second" || r.Stargazers != 9 || r.IsPrivate {
		t.Fatalf("update not applied: %+v", r)
	}
}

func TestGetRepoByFullName(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	id, _ := s.UpsertRepo(ctx, &Repo{GitHubID: 7, FullName: "a/b", DefaultBranch: "main"})

	r, err := s.GetRepoByFullName(ctx, "a/b")
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != id {
		t.Fatalf("ID = %d, want %d", r.ID, id)
	}
	if _, err := s.GetRepoByFullName(ctx, "missing/repo"); err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestGetRepoNotFound(t *testing.T) {
	s := openTemp(t)
	if _, err := s.GetRepo(context.Background(), 999); err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestNewTablesExist|TestUpsertRepo|TestGetRepo' -v`
Expected: FAIL — `undefined: Repo`.

- [ ] **Step 3: Write minimal implementation**

`internal/store/repos.go`:
```go
package store

import (
	"context"
	"database/sql"
	"time"
)

// Repo is a tracked GitHub repository.
type Repo struct {
	ID            int64
	GitHubID      int64
	FullName      string // "owner/name"
	IsPrivate     bool
	DefaultBranch string
	Description   string
	Stargazers    int64
	Forks         int64
	CreatedAt     time.Time
}

// UpsertRepo inserts or updates a repo by github_id and returns the local id.
func (s *Store) UpsertRepo(ctx context.Context, r *Repo) (int64, error) {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO repos (github_id, full_name, is_private, default_branch, description, stargazers, forks)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(github_id) DO UPDATE SET
			full_name = excluded.full_name,
			is_private = excluded.is_private,
			default_branch = excluded.default_branch,
			description = excluded.description,
			stargazers = excluded.stargazers,
			forks = excluded.forks`,
		r.GitHubID, r.FullName, boolToInt(r.IsPrivate), r.DefaultBranch,
		r.Description, r.Stargazers, r.Forks,
	)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := s.DB.QueryRowContext(ctx,
		`SELECT id FROM repos WHERE github_id = ?`, r.GitHubID,
	).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// GetRepo returns the repo with the given local id, or ErrNotFound.
func (s *Store) GetRepo(ctx context.Context, id int64) (*Repo, error) {
	return s.scanRepo(s.DB.QueryRowContext(ctx, repoSelect+` WHERE id = ?`, id))
}

// GetRepoByFullName returns the repo with the given "owner/name", or ErrNotFound.
func (s *Store) GetRepoByFullName(ctx context.Context, fullName string) (*Repo, error) {
	return s.scanRepo(s.DB.QueryRowContext(ctx, repoSelect+` WHERE full_name = ?`, fullName))
}

const repoSelect = `SELECT id, github_id, full_name, is_private, default_branch,
	description, stargazers, forks, created_at FROM repos`

func (s *Store) scanRepo(row *sql.Row) (*Repo, error) {
	var r Repo
	var priv int
	err := row.Scan(&r.ID, &r.GitHubID, &r.FullName, &priv, &r.DefaultBranch,
		&r.Description, &r.Stargazers, &r.Forks, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.IsPrivate = priv != 0
	return &r, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run 'TestNewTablesExist|TestUpsertRepo|TestGetRepo' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/repos.go internal/store/repos_test.go
git commit -m "feat: repos DAO with upsert and lookups"
```

---

## Task 3: Event DAOs — batch upserts for commits/PRs/issues/releases

**Files:**
- Create: `internal/store/events.go`, `internal/store/events_test.go`

Each batch upsert runs inside a single transaction with one prepared statement, so a page of up to 100 nodes lands atomically. `ON CONFLICT` makes re-ingesting the same page idempotent (important for resumable backfill).

- [ ] **Step 1: Write the failing test**

`internal/store/events_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestUpsert.*Batch|TestUpsertEmptyBatch' -v`
Expected: FAIL — `undefined: Commit`.

- [ ] **Step 3: Write minimal implementation**

`internal/store/events.go`:
```go
package store

import (
	"context"
	"database/sql"
	"time"
)

// Commit is a single commit event (source of truth).
type Commit struct {
	SHA          string
	AuthorLogin  string
	CommittedAt  time.Time
	Additions    int64
	Deletions    int64
	IsBot        bool
	MsgFirstLine string
}

// PullRequest is a single pull-request event.
type PullRequest struct {
	Number        int64
	AuthorLogin   string
	State         string // "OPEN" | "CLOSED" | "MERGED"
	CreatedAt     time.Time
	MergedAt      *time.Time
	ClosedAt      *time.Time
	Additions     int64
	Deletions     int64
	ChangedFiles  int64
	CommentsCount int64
	FirstReviewAt *time.Time
	IsBot         bool
	Title         string
}

// Issue is a single issue event.
type Issue struct {
	Number        int64
	AuthorLogin   string
	State         string // "OPEN" | "CLOSED"
	CreatedAt     time.Time
	ClosedAt      *time.Time
	CommentsCount int64
	IsBot         bool
	Title         string
}

// Release is a single release event.
type Release struct {
	Tag         string
	Name        string
	PublishedAt *time.Time
	AuthorLogin string
}

// inTx runs fn inside a transaction, committing on success and rolling back on error.
func (s *Store) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// UpsertCommits batch-upserts a page of commits in one transaction.
func (s *Store) UpsertCommits(ctx context.Context, repoID int64, commits []Commit) error {
	if len(commits) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO commits (repo_id, sha, author_login, committed_at, additions, deletions, is_bot, msg_first_line)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(repo_id, sha) DO UPDATE SET
				author_login = excluded.author_login,
				committed_at = excluded.committed_at,
				additions = excluded.additions,
				deletions = excluded.deletions,
				is_bot = excluded.is_bot,
				msg_first_line = excluded.msg_first_line`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, c := range commits {
			if _, err := stmt.ExecContext(ctx,
				repoID, c.SHA, c.AuthorLogin, c.CommittedAt, c.Additions, c.Deletions,
				boolToInt(c.IsBot), c.MsgFirstLine,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

// UpsertPullRequests batch-upserts a page of pull requests in one transaction.
func (s *Store) UpsertPullRequests(ctx context.Context, repoID int64, prs []PullRequest) error {
	if len(prs) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO pull_requests (repo_id, number, author_login, state, created_at, merged_at, closed_at,
				additions, deletions, changed_files, comments_count, first_review_at, is_bot, title)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(repo_id, number) DO UPDATE SET
				author_login = excluded.author_login,
				state = excluded.state,
				created_at = excluded.created_at,
				merged_at = excluded.merged_at,
				closed_at = excluded.closed_at,
				additions = excluded.additions,
				deletions = excluded.deletions,
				changed_files = excluded.changed_files,
				comments_count = excluded.comments_count,
				first_review_at = excluded.first_review_at,
				is_bot = excluded.is_bot,
				title = excluded.title`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, p := range prs {
			if _, err := stmt.ExecContext(ctx,
				repoID, p.Number, p.AuthorLogin, p.State, p.CreatedAt, p.MergedAt, p.ClosedAt,
				p.Additions, p.Deletions, p.ChangedFiles, p.CommentsCount, p.FirstReviewAt,
				boolToInt(p.IsBot), p.Title,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

// UpsertIssues batch-upserts a page of issues in one transaction.
func (s *Store) UpsertIssues(ctx context.Context, repoID int64, issues []Issue) error {
	if len(issues) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO issues (repo_id, number, author_login, state, created_at, closed_at, comments_count, is_bot, title)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(repo_id, number) DO UPDATE SET
				author_login = excluded.author_login,
				state = excluded.state,
				created_at = excluded.created_at,
				closed_at = excluded.closed_at,
				comments_count = excluded.comments_count,
				is_bot = excluded.is_bot,
				title = excluded.title`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, is := range issues {
			if _, err := stmt.ExecContext(ctx,
				repoID, is.Number, is.AuthorLogin, is.State, is.CreatedAt, is.ClosedAt,
				is.CommentsCount, boolToInt(is.IsBot), is.Title,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

// UpsertReleases batch-upserts a page of releases in one transaction.
func (s *Store) UpsertReleases(ctx context.Context, repoID int64, releases []Release) error {
	if len(releases) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO releases (repo_id, tag, name, published_at, author_login)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(repo_id, tag) DO UPDATE SET
				name = excluded.name,
				published_at = excluded.published_at,
				author_login = excluded.author_login`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, rel := range releases {
			if _, err := stmt.ExecContext(ctx,
				repoID, rel.Tag, rel.Name, rel.PublishedAt, rel.AuthorLogin,
			); err != nil {
				return err
			}
		}
		return nil
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run 'TestUpsert.*Batch|TestUpsertEmptyBatch' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/events.go internal/store/events_test.go
git commit -m "feat: batch upsert DAOs for commits/PRs/issues/releases"
```

---

## Task 4: Sync-state DAO

**Files:**
- Create: `internal/store/syncstate.go`, `internal/store/syncstate_test.go`

- [ ] **Step 1: Write the failing test**

`internal/store/syncstate_test.go`:
```go
package store

import (
	"context"
	"testing"
)

func TestGetSyncStateDefaultsWhenAbsent(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)

	st, err := s.GetSyncState(ctx, repoID)
	if err != nil {
		t.Fatalf("GetSyncState should return zero-value state, not error: %v", err)
	}
	if st.RepoID != repoID {
		t.Fatalf("RepoID = %d, want %d", st.RepoID, repoID)
	}
	if st.Status != "" || st.LastPRCursor != "" || st.LastCommitAt != nil {
		t.Fatalf("expected empty state, got %+v", st)
	}
}

func TestUpsertSyncStateRoundTrip(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)

	last := ts("2026-02-01T00:00:00Z")
	in := &SyncState{
		RepoID:            repoID,
		LastCommitAt:      &last,
		LastCommitCursor:  "c1",
		LastPRCursor:      "p1",
		LastIssueCursor:   "i1",
		LastReleaseCursor: "r1",
		Status:            "backfilling",
	}
	if err := s.UpsertSyncState(ctx, in); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSyncState(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastPRCursor != "p1" || got.LastIssueCursor != "i1" ||
		got.LastCommitCursor != "c1" || got.LastReleaseCursor != "r1" ||
		got.Status != "backfilling" || got.LastCommitAt == nil ||
		!got.LastCommitAt.Equal(last) {
		t.Fatalf("round trip mismatch: %+v", got)
	}

	// Upsert again updates in place.
	in.Status = "complete"
	in.LastPRCursor = "p2"
	if err := s.UpsertSyncState(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetSyncState(ctx, repoID)
	if got.Status != "complete" || got.LastPRCursor != "p2" {
		t.Fatalf("update not applied: %+v", got)
	}
	var n int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_state WHERE repo_id = ?`, repoID).Scan(&n)
	if n != 1 {
		t.Fatalf("sync_state rows = %d, want 1", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run 'TestGetSyncState|TestUpsertSyncState' -v`
Expected: FAIL — `undefined: SyncState`.

- [ ] **Step 3: Write minimal implementation**

`internal/store/syncstate.go`:
```go
package store

import (
	"context"
	"database/sql"
	"time"
)

// SyncState holds per-repo backfill/delta cursors and status.
type SyncState struct {
	RepoID            int64
	LastCommitAt      *time.Time
	LastCommitCursor  string
	LastPRCursor      string
	LastIssueCursor   string
	LastReleaseCursor string
	LastBackfillAt    *time.Time
	Status            string // "" | "backfilling" | "complete"
}

// GetSyncState returns the sync state for a repo. When no row exists it returns
// a zero-value state (RepoID set) and a nil error — callers treat absence as "fresh".
func (s *Store) GetSyncState(ctx context.Context, repoID int64) (*SyncState, error) {
	st := &SyncState{RepoID: repoID}
	err := s.DB.QueryRowContext(ctx, `
		SELECT last_commit_at, last_commit_cursor, last_pr_cursor, last_issue_cursor,
			last_release_cursor, last_backfill_at, status
		FROM sync_state WHERE repo_id = ?`, repoID,
	).Scan(&st.LastCommitAt, &st.LastCommitCursor, &st.LastPRCursor, &st.LastIssueCursor,
		&st.LastReleaseCursor, &st.LastBackfillAt, &st.Status)
	if err == sql.ErrNoRows {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	return st, nil
}

// UpsertSyncState inserts or updates the sync state for a repo.
func (s *Store) UpsertSyncState(ctx context.Context, st *SyncState) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO sync_state (repo_id, last_commit_at, last_commit_cursor, last_pr_cursor,
			last_issue_cursor, last_release_cursor, last_backfill_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repo_id) DO UPDATE SET
			last_commit_at = excluded.last_commit_at,
			last_commit_cursor = excluded.last_commit_cursor,
			last_pr_cursor = excluded.last_pr_cursor,
			last_issue_cursor = excluded.last_issue_cursor,
			last_release_cursor = excluded.last_release_cursor,
			last_backfill_at = excluded.last_backfill_at,
			status = excluded.status`,
		st.RepoID, st.LastCommitAt, st.LastCommitCursor, st.LastPRCursor,
		st.LastIssueCursor, st.LastReleaseCursor, st.LastBackfillAt, st.Status,
	)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run 'TestGetSyncState|TestUpsertSyncState' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/syncstate.go internal/store/syncstate_test.go
git commit -m "feat: sync_state DAO with cursor round-trip"
```

---

## Task 5: ETag cache DAO

**Files:**
- Create: `internal/store/etagcache.go`, `internal/store/etagcache_test.go`

- [ ] **Step 1: Write the failing test**

`internal/store/etagcache_test.go`:
```go
package store

import (
	"bytes"
	"context"
	"testing"
)

func TestETagCachePutGet(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	if _, err := s.GetETag(ctx, "https://api/x"); err != ErrNotFound {
		t.Fatalf("absent etag got %v, want ErrNotFound", err)
	}

	if err := s.PutETag(ctx, &ETagEntry{
		URL: "https://api/x", ETag: `W/"abc"`, Status: 200,
		Body: []byte(`{"ok":true}`), LastModified: "Mon, 01 Jan 2026 00:00:00 GMT",
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetETag(ctx, "https://api/x")
	if err != nil {
		t.Fatal(err)
	}
	if got.ETag != `W/"abc"` || got.Status != 200 || !bytes.Equal(got.Body, []byte(`{"ok":true}`)) {
		t.Fatalf("etag round trip mismatch: %+v", got)
	}

	// Put again with new content overwrites by URL.
	if err := s.PutETag(ctx, &ETagEntry{
		URL: "https://api/x", ETag: `W/"def"`, Status: 200, Body: []byte(`{"ok":false}`),
	}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetETag(ctx, "https://api/x")
	if got.ETag != `W/"def"` || !bytes.Equal(got.Body, []byte(`{"ok":false}`)) {
		t.Fatalf("etag not overwritten: %+v", got)
	}

	var n int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM etags`).Scan(&n)
	if n != 1 {
		t.Fatalf("etag rows = %d, want 1", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestETagCache -v`
Expected: FAIL — `undefined: ETagEntry`.

- [ ] **Step 3: Write minimal implementation**

`internal/store/etagcache.go`:
```go
package store

import (
	"context"
	"database/sql"
)

// ETagEntry is a cached conditional-GET response keyed by URL.
type ETagEntry struct {
	URL          string
	ETag         string
	Status       int
	Body         []byte
	LastModified string
}

// GetETag returns the cached entry for a URL, or ErrNotFound.
func (s *Store) GetETag(ctx context.Context, url string) (*ETagEntry, error) {
	var e ETagEntry
	err := s.DB.QueryRowContext(ctx,
		`SELECT url, etag, status, body, last_modified FROM etags WHERE url = ?`, url,
	).Scan(&e.URL, &e.ETag, &e.Status, &e.Body, &e.LastModified)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// PutETag inserts or replaces the cached entry for a URL.
func (s *Store) PutETag(ctx context.Context, e *ETagEntry) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO etags (url, etag, status, body, last_modified, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(url) DO UPDATE SET
			etag = excluded.etag,
			status = excluded.status,
			body = excluded.body,
			last_modified = excluded.last_modified,
			updated_at = CURRENT_TIMESTAMP`,
		e.URL, e.ETag, e.Status, e.Body, e.LastModified,
	)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestETagCache -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/etagcache.go internal/store/etagcache_test.go
git commit -m "feat: etag cache DAO for conditional GETs"
```

---

## Task 6: RecomputeDailyStats — rebuild aggregates from events

**Files:**
- Create: `internal/store/aggregates.go`, `internal/store/aggregates_test.go`

Events are the source of truth; `RecomputeDailyStats` deletes the aggregate rows in the `[fromDate, toDate]` window and rebuilds them by grouping events on their UTC date. Dates are inclusive `'YYYY-MM-DD'` strings. The whole window rebuild runs in one transaction so charts never see a partial state. A PR/issue contributes to `prs_opened`/`issues_opened` on its `created_at` date, to `prs_merged` on `merged_at`, `prs_closed`/`issues_closed` on `closed_at` — but only when that date falls inside the window (an event opened before the window but closed inside it still gets its close counted; its open does not). `active_contributors` and `comments` come from commits and (PR+issue) comment counts respectively, attributed to the event's relevant date.

- [ ] **Step 1: Write the failing test**

`internal/store/aggregates_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestRecompute -v`
Expected: FAIL — `undefined: (*Store).RecomputeDailyStats`.

- [ ] **Step 3: Write minimal implementation**

`internal/store/aggregates.go`:
```go
package store

import (
	"context"
	"database/sql"
)

// RecomputeDailyStats rebuilds daily_repo_stats and daily_contributor_stats for
// the inclusive UTC date window [fromDate, toDate] (each "YYYY-MM-DD") from the
// event tables, which are the source of truth. It deletes existing aggregate
// rows in the window first, so the operation is idempotent and corrects drift
// from late edits (a PR merging, an issue closing) — see spec §5/§6/§12.
//
// Each metric is attributed to the UTC date of the relevant event timestamp:
// commits → committed_at; prs_opened/issues_opened → created_at;
// prs_merged → merged_at; prs_closed/issues_closed → closed_at;
// comments → PR/issue created_at; releases → published_at.
func (s *Store) RecomputeDailyStats(ctx context.Context, repoID int64, fromDate, toDate string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		// Clear the window so stale rows for now-empty days disappear.
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM daily_repo_stats WHERE repo_id = ? AND date >= ? AND date <= ?`,
			repoID, fromDate, toDate); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM daily_contributor_stats WHERE repo_id = ? AND date >= ? AND date <= ?`,
			repoID, fromDate, toDate); err != nil {
			return err
		}

		// Build per-date repo aggregates by unioning each metric source, then
		// summing into daily_repo_stats. date() in SQLite yields 'YYYY-MM-DD' (UTC).
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO daily_repo_stats (
				repo_id, date, commits, additions, deletions,
				prs_opened, prs_merged, prs_closed,
				issues_opened, issues_closed, comments, releases, active_contributors)
			SELECT ?1, day,
				SUM(commits), SUM(additions), SUM(deletions),
				SUM(prs_opened), SUM(prs_merged), SUM(prs_closed),
				SUM(issues_opened), SUM(issues_closed), SUM(comments), SUM(releases),
				MAX(active_contributors)
			FROM (
				-- Commits and per-day distinct contributor count.
				SELECT date(committed_at) AS day,
					COUNT(*) AS commits, SUM(additions) AS additions, SUM(deletions) AS deletions,
					0 AS prs_opened, 0 AS prs_merged, 0 AS prs_closed,
					0 AS issues_opened, 0 AS issues_closed, 0 AS comments, 0 AS releases,
					COUNT(DISTINCT author_login) AS active_contributors
				FROM commits WHERE repo_id = ?1 GROUP BY day
				UNION ALL
				SELECT date(created_at) AS day,
					0,0,0, COUNT(*), 0, 0, 0,0,0,0, 0
				FROM pull_requests WHERE repo_id = ?1 GROUP BY day
				UNION ALL
				SELECT date(merged_at) AS day,
					0,0,0, 0, COUNT(*), 0, 0,0,0,0, 0
				FROM pull_requests WHERE repo_id = ?1 AND merged_at IS NOT NULL GROUP BY day
				UNION ALL
				SELECT date(closed_at) AS day,
					0,0,0, 0, 0, COUNT(*), 0,0,0,0, 0
				FROM pull_requests WHERE repo_id = ?1 AND closed_at IS NOT NULL GROUP BY day
				UNION ALL
				SELECT date(created_at) AS day,
					0,0,0, 0,0,0, COUNT(*), 0, 0,0, 0
				FROM issues WHERE repo_id = ?1 GROUP BY day
				UNION ALL
				SELECT date(closed_at) AS day,
					0,0,0, 0,0,0, 0, COUNT(*), 0,0, 0
				FROM issues WHERE repo_id = ?1 AND closed_at IS NOT NULL GROUP BY day
				UNION ALL
				SELECT date(created_at) AS day,
					0,0,0, 0,0,0, 0,0, SUM(comments_count), 0, 0
				FROM pull_requests WHERE repo_id = ?1 GROUP BY day
				UNION ALL
				SELECT date(created_at) AS day,
					0,0,0, 0,0,0, 0,0, SUM(comments_count), 0, 0
				FROM issues WHERE repo_id = ?1 GROUP BY day
				UNION ALL
				SELECT date(published_at) AS day,
					0,0,0, 0,0,0, 0,0,0, COUNT(*), 0
				FROM releases WHERE repo_id = ?1 AND published_at IS NOT NULL GROUP BY day
			)
			WHERE day IS NOT NULL AND day >= ?2 AND day <= ?3
			GROUP BY day`,
			repoID, fromDate, toDate); err != nil {
			return err
		}

		// Per-contributor commit aggregates.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO daily_contributor_stats (repo_id, date, login, commits, additions, deletions)
			SELECT ?1, date(committed_at) AS day, author_login,
				COUNT(*), SUM(additions), SUM(deletions)
			FROM commits
			WHERE repo_id = ?1 AND date(committed_at) >= ?2 AND date(committed_at) <= ?3
			GROUP BY day, author_login`,
			repoID, fromDate, toDate); err != nil {
			return err
		}
		return nil
	})
}
```

> Note on `active_contributors`: each per-day source row sets it to either the real distinct-contributor count (commits source) or `0` (all other sources). The outer `MAX(active_contributors)` over a `day` group therefore picks the commits-derived count, which is correct because only commits carry a contributor.
>
> Note on placeholders: both INSERTs use **only** SQLite numbered placeholders `?1/?2/?3` (never mixed with anonymous `?`, which SQLite numbers implicitly and would make binding ambiguous). `?1` (repoID) is reused across every `repo_id = ?1`, so each statement binds exactly three args in order: `repoID, fromDate, toDate`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestRecompute -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Run the whole store package**

Run: `go test ./internal/store/ -v`
Expected: PASS — M1 tests plus all new DAO tests.

- [ ] **Step 6: Commit**

```bash
git add internal/store/aggregates.go internal/store/aggregates_test.go
git commit -m "feat: RecomputeDailyStats rebuilds aggregates from events"
```

---

## Task 7: Bot detection

**Files:**
- Create: `internal/githubapi/bot.go`, `internal/githubapi/bot_test.go`

- [ ] **Step 1: Write the failing test**

`internal/githubapi/bot_test.go`:
```go
package githubapi

import "testing"

func TestIsBot(t *testing.T) {
	cases := []struct {
		login string
		want  bool
	}{
		{"dependabot[bot]", true},
		{"renovate[bot]", true},
		{"github-actions[bot]", true},
		{"dependabot", true},   // known list, no suffix
		{"renovate", true},     // known list
		{"web-flow", true},     // GitHub's merge-commit author
		{"octocat", false},
		{"", false},
		{"Dependabot[Bot]", true}, // case-insensitive suffix
	}
	for _, c := range cases {
		if got := IsBot(c.login); got != c.want {
			t.Errorf("IsBot(%q) = %v, want %v", c.login, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/githubapi/ -run TestIsBot -v`
Expected: FAIL — `undefined: IsBot`.

- [ ] **Step 3: Write minimal implementation**

`internal/githubapi/bot.go`:
```go
package githubapi

import "strings"

// knownBots is a small allowlist of bot logins that do not carry the "[bot]"
// suffix in every context (e.g. the GraphQL author login can drop it).
var knownBots = map[string]bool{
	"dependabot":    true,
	"renovate":      true,
	"renovate-bot":  true,
	"github-actions": true,
	"web-flow":      true, // GitHub's synthetic merge-commit author
	"imgbot":        true,
	"codecov":       true,
	"mergify":       true,
}

// IsBot reports whether a login belongs to a bot. True when the login ends in
// "[bot]" (case-insensitive) or matches the known-bot allowlist. Used to set the
// is_bot flag at ingest so the dashboard's exclude-bots toggle works (spec §7).
func IsBot(login string) bool {
	if login == "" {
		return false
	}
	if strings.HasSuffix(strings.ToLower(login), "[bot]") {
		return true
	}
	return knownBots[strings.ToLower(login)]
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/githubapi/ -run TestIsBot -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/githubapi/bot.go internal/githubapi/bot_test.go
git commit -m "feat: githubapi bot detection"
```

---

## Task 8: Dual rate-limit budget manager

**Files:**
- Create: `internal/githubapi/budget.go`, `internal/githubapi/budget_test.go`

`Budget` tracks REST (`X-RateLimit-Remaining`/`Reset`) and GraphQL (points `remaining`/`resetAt`) **separately** (spec §3: they are distinct buckets). It is updated from REST response headers and from the GraphQL `rateLimit` field, and reports remaining + reset for either bucket. It also computes a backoff duration for secondary-limit responses (`403`/`429` + optional `Retry-After`). The type holds no clock — callers pass `now` where a comparison is needed, keeping tests deterministic.

- [ ] **Step 1: Write the failing test**

`internal/githubapi/budget_test.go`:
```go
package githubapi

import (
	"net/http"
	"testing"
	"time"
)

func TestBudgetUpdateFromRESTHeaders(t *testing.T) {
	b := NewBudget()
	reset := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "4321")
	h.Set("X-RateLimit-Reset", "1775044800") // 2026-04-01T12:00:00Z unix
	b.UpdateFromRESTHeaders(h)

	rem, gotReset := b.REST()
	if rem != 4321 {
		t.Fatalf("REST remaining = %d, want 4321", rem)
	}
	if !gotReset.Equal(reset) {
		t.Fatalf("REST reset = %v, want %v", gotReset, reset)
	}
}

func TestBudgetUpdateFromGraphQL(t *testing.T) {
	b := NewBudget()
	resetAt := "2026-04-01T13:00:00Z"
	b.UpdateFromGraphQL(RateLimit{Cost: 1, Remaining: 4990, ResetAt: resetAt})

	rem, reset := b.GraphQL()
	if rem != 4990 {
		t.Fatalf("GraphQL remaining = %d, want 4990", rem)
	}
	want, _ := time.Parse(time.RFC3339, resetAt)
	if !reset.Equal(want) {
		t.Fatalf("GraphQL reset = %v, want %v", reset, want)
	}
}

func TestBudgetExhaustion(t *testing.T) {
	b := NewBudget()
	b.UpdateFromGraphQL(RateLimit{Remaining: 0, ResetAt: "2026-04-01T13:00:00Z"})
	if !b.GraphQLExhausted() {
		t.Fatal("expected GraphQL exhausted at 0 remaining")
	}
	b.UpdateFromGraphQL(RateLimit{Remaining: 10, ResetAt: "2026-04-01T13:00:00Z"})
	if b.GraphQLExhausted() {
		t.Fatal("expected not exhausted at 10 remaining")
	}
}

func TestBackoffForSecondaryLimit(t *testing.T) {
	b := NewBudget()
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	// Retry-After in seconds.
	h := http.Header{}
	h.Set("Retry-After", "30")
	d := b.BackoffFor(http.StatusForbidden, h, now)
	if d != 30*time.Second {
		t.Fatalf("Retry-After seconds backoff = %v, want 30s", d)
	}

	// Retry-After as HTTP date.
	h2 := http.Header{}
	h2.Set("Retry-After", now.Add(45*time.Second).UTC().Format(http.TimeFormat))
	d2 := b.BackoffFor(http.StatusTooManyRequests, h2, now)
	if d2 < 44*time.Second || d2 > 46*time.Second {
		t.Fatalf("Retry-After date backoff = %v, want ~45s", d2)
	}

	// 429 with no Retry-After falls back to a default minimum.
	d3 := b.BackoffFor(http.StatusTooManyRequests, http.Header{}, now)
	if d3 <= 0 {
		t.Fatalf("default backoff = %v, want > 0", d3)
	}

	// A normal 200 yields no backoff.
	if d4 := b.BackoffFor(http.StatusOK, http.Header{}, now); d4 != 0 {
		t.Fatalf("200 backoff = %v, want 0", d4)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/githubapi/ -run 'TestBudget|TestBackoff' -v`
Expected: FAIL — `undefined: NewBudget`.

- [ ] **Step 3: Write minimal implementation**

`internal/githubapi/budget.go`:
```go
package githubapi

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimit mirrors the GraphQL `rateLimit { cost remaining resetAt }` field.
type RateLimit struct {
	Cost      int    `json:"cost"`
	Remaining int    `json:"remaining"`
	ResetAt   string `json:"resetAt"` // RFC3339
}

// defaultBackoff is the minimum wait applied to a secondary-limit response that
// carries no Retry-After header.
const defaultBackoff = 60 * time.Second

// Budget tracks the REST and GraphQL rate-limit buckets separately. GitHub's
// REST (5,000 req/hr) and GraphQL (5,000 points/hr) limits are distinct pools
// (spec §3), so each is tracked on its own. Safe for concurrent use.
type Budget struct {
	mu            sync.Mutex
	restRemaining int
	restReset     time.Time
	gqlRemaining  int
	gqlReset      time.Time
}

// NewBudget returns a Budget with optimistic full buckets.
func NewBudget() *Budget {
	return &Budget{restRemaining: 5000, gqlRemaining: 5000}
}

// UpdateFromRESTHeaders ingests X-RateLimit-* headers from a REST response.
func (b *Budget) UpdateFromRESTHeaders(h http.Header) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if v := h.Get("X-RateLimit-Remaining"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			b.restRemaining = n
		}
	}
	if v := h.Get("X-RateLimit-Reset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			b.restReset = time.Unix(n, 0).UTC()
		}
	}
}

// UpdateFromGraphQL ingests a GraphQL rateLimit object.
func (b *Budget) UpdateFromGraphQL(rl RateLimit) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gqlRemaining = rl.Remaining
	if rl.ResetAt != "" {
		if t, err := time.Parse(time.RFC3339, rl.ResetAt); err == nil {
			b.gqlReset = t.UTC()
		}
	}
}

// REST returns the REST bucket's remaining count and reset time.
func (b *Budget) REST() (remaining int, reset time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.restRemaining, b.restReset
}

// GraphQL returns the GraphQL bucket's remaining points and reset time.
func (b *Budget) GraphQL() (remaining int, reset time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.gqlRemaining, b.gqlReset
}

// GraphQLExhausted reports whether the GraphQL bucket is empty.
func (b *Budget) GraphQLExhausted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.gqlRemaining <= 0
}

// RESTExhausted reports whether the REST bucket is empty.
func (b *Budget) RESTExhausted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.restRemaining <= 0
}

// BackoffFor returns how long to wait before retrying after a response. It
// returns 0 for non-limit statuses. For 403/429 it honours Retry-After (seconds
// or HTTP-date) and otherwise falls back to defaultBackoff.
func (b *Budget) BackoffFor(status int, h http.Header, now time.Time) time.Duration {
	if status != http.StatusForbidden && status != http.StatusTooManyRequests {
		return 0
	}
	if ra := h.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			return time.Duration(secs) * time.Second
		}
		if t, err := http.ParseTime(ra); err == nil {
			if d := t.Sub(now); d > 0 {
				return d
			}
		}
	}
	return defaultBackoff
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/githubapi/ -run 'TestBudget|TestBackoff' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/githubapi/budget.go internal/githubapi/budget_test.go
git commit -m "feat: dual REST/GraphQL rate-limit budget manager"
```

---

## Task 9: ETag conditional REST transport

**Files:**
- Create: `internal/githubapi/etagtransport.go`, `internal/githubapi/etagtransport_test.go`

`ETagTransport` is an `http.RoundTripper` wrapping a base transport. For a GET it looks up a cached `(etag, body, status)` by URL, attaches `If-None-Match`, and on a `304` synthesizes a `200` response from the cached body (a `304` does **not** count against the rate limit — spec §3). On a `200` with an `ETag` header it caches the body. Non-GET requests pass straight through.

- [ ] **Step 1: Write the failing test**

`internal/githubapi/etagtransport_test.go`:
```go
package githubapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

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

func TestETagTransportCachesAndRevalidates(t *testing.T) {
	st := openTestStore(t)

	var hits int32
	var sawINM atomic.Value
	sawINM.Store("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if inm := r.Header.Get("If-None-Match"); inm != "" {
			sawINM.Store(inm)
			if inm == `W/"v1"` {
				w.WriteHeader(http.StatusNotModified) // 304 — free, no body
				return
			}
		}
		w.Header().Set("ETag", `W/"v1"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"repo"}`))
	}))
	defer srv.Close()

	rt := &ETagTransport{Store: st, Base: http.DefaultTransport}
	client := &http.Client{Transport: rt}

	// First call: miss → 200, body cached.
	req1, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/repos/a/b", nil)
	resp1, err := client.Do(req1)
	if err != nil {
		t.Fatal(err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	if resp1.StatusCode != 200 || string(body1) != `{"name":"repo"}` {
		t.Fatalf("first call: status=%d body=%s", resp1.StatusCode, body1)
	}

	// Second call: cached etag sent; server returns 304; transport serves cached body as 200.
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/repos/a/b", nil)
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("second call status = %d, want 200 (served from cache)", resp2.StatusCode)
	}
	if string(body2) != `{"name":"repo"}` {
		t.Fatalf("second call body = %s, want cached body", body2)
	}
	if sawINM.Load().(string) != `W/"v1"` {
		t.Fatalf("If-None-Match sent = %q, want W/\"v1\"", sawINM.Load())
	}
	if hits != 2 {
		t.Fatalf("server hits = %d, want 2", hits)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/githubapi/ -run TestETagTransport -v`
Expected: FAIL — `undefined: ETagTransport`.

- [ ] **Step 3: Write minimal implementation**

`internal/githubapi/etagtransport.go`:
```go
package githubapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github-stats/internal/store"
)

// ETagTransport is an http.RoundTripper that performs conditional REST GETs
// using cached ETags. On a 304 it serves the cached body as a 200, so callers
// never see a 304 and the request does not count against the rate limit
// (spec §3). Non-GET requests are passed straight through to Base.
type ETagTransport struct {
	Store *store.Store
	Base  http.RoundTripper
}

func (t *ETagTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

// RoundTrip implements http.RoundTripper.
func (t *ETagTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return t.base().RoundTrip(req)
	}
	ctx := req.Context()
	url := req.URL.String()

	cached, err := t.Store.GetETag(ctx, url)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if cached != nil {
		req = req.Clone(ctx)
		req.Header.Set("If-None-Match", cached.ETag)
	}

	resp, err := t.base().RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotModified && cached != nil {
		resp.Body.Close()
		return t.responseFromCache(req, cached), nil
	}

	if resp.StatusCode == http.StatusOK {
		if etag := resp.Header.Get("ETag"); etag != "" {
			body, rerr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if rerr != nil {
				return nil, rerr
			}
			_ = t.Store.PutETag(ctx, &store.ETagEntry{
				URL:          url,
				ETag:         etag,
				Status:       http.StatusOK,
				Body:         body,
				LastModified: resp.Header.Get("Last-Modified"),
			})
			resp.Body = io.NopCloser(bytes.NewReader(body))
			resp.ContentLength = int64(len(body))
		}
	}
	return resp, nil
}

func (t *ETagTransport) responseFromCache(req *http.Request, cached *store.ETagEntry) *http.Response {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("ETag", cached.ETag)
	return &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        h,
		Body:          io.NopCloser(bytes.NewReader(cached.Body)),
		ContentLength: int64(len(cached.Body)),
		Request:       req,
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/githubapi/ -run TestETagTransport -v`
Expected: PASS.

- [ ] **Step 5: Run `go mod tidy` (githubapi now imports the store package) and verify build**

Run: `go mod tidy && go build ./...`
Expected: no module changes beyond what already exists; build succeeds.

- [ ] **Step 6: Commit**

```bash
git add internal/githubapi/etagtransport.go internal/githubapi/etagtransport_test.go go.mod go.sum
git commit -m "feat: etag conditional REST transport (304s are free)"
```

---

## Task 10: GraphQL client core

**Files:**
- Create: `internal/githubapi/client.go`, `internal/githubapi/graphql.go`, `internal/githubapi/graphql_test.go`

The low-level `graphql` method POSTs `{query, variables}` to `GraphQLURL` with the bearer token, decodes into the caller's typed `data` target, surfaces GraphQL `errors`, and updates the `Budget` from any `rateLimit` block present in the response. `Client` also exposes a REST GET helper (used by the repo-meta fallback and releases fallback) that routes through the ETag transport.

- [ ] **Step 1: Write the failing test**

`internal/githubapi/graphql_test.go`:
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
)

func newTestClient(t *testing.T, gqlURL, restURL string) *Client {
	t.Helper()
	st := openTestStore(t)
	return NewClient(Options{
		Token:       "gho_test",
		GraphQLURL:  gqlURL,
		RESTBaseURL: restURL,
		Store:       st,
		HTTP:        &http.Client{},
	})
}

func TestGraphQLDecodesDataAndRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gho_test" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "rateLimit") {
			t.Errorf("query missing rateLimit: %s", req.Query)
		}
		if req.Variables["owner"] != "octocat" {
			t.Errorf("variables = %v", req.Variables)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"thing": {"name": "hello"},
				"rateLimit": {"cost": 1, "remaining": 4998, "resetAt": "2026-04-01T13:00:00Z"}
			}
		}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, "http://unused")

	var data struct {
		Thing struct {
			Name string `json:"name"`
		} `json:"thing"`
		RateLimit RateLimit `json:"rateLimit"`
	}
	err := c.graphql(context.Background(),
		`query($owner:String!){ thing rateLimit { cost remaining resetAt } }`,
		map[string]any{"owner": "octocat"}, &data)
	if err != nil {
		t.Fatal(err)
	}
	if data.Thing.Name != "hello" {
		t.Fatalf("decoded name = %q", data.Thing.Name)
	}
	rem, _ := c.Budget.GraphQL()
	if rem != 4998 {
		t.Fatalf("budget not updated from rateLimit: remaining=%d", rem)
	}
}

func TestGraphQLSurfacesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"Could not resolve to a Repository"}]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, "http://unused")
	var data struct{}
	err := c.graphql(context.Background(), `query{x}`, nil, &data)
	if err == nil || !strings.Contains(err.Error(), "Could not resolve") {
		t.Fatalf("expected GraphQL error surfaced, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/githubapi/ -run TestGraphQL -v`
Expected: FAIL — `undefined: NewClient`.

- [ ] **Step 3: Write minimal implementation**

`internal/githubapi/client.go`:
```go
package githubapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github-stats/internal/store"
)

// Options configures a Client. URLs are injectable so tests can point at
// httptest servers (spec/design contract).
type Options struct {
	Token       string
	GraphQLURL  string // e.g. https://api.github.com/graphql
	RESTBaseURL string // e.g. https://api.github.com
	Store       *store.Store
	HTTP        *http.Client // optional; one is built (with ETag transport) if nil
}

// Client is a rate-limit-aware GitHub API client (GraphQL + conditional REST).
type Client struct {
	token       string
	graphqlURL  string
	restBaseURL string
	http        *http.Client
	Budget      *Budget
}

// NewClient builds a Client. If Options.HTTP is nil, an http.Client whose
// transport is an ETagTransport (over the store) is created so REST GETs are
// conditional by default.
func NewClient(o Options) *Client {
	httpClient := o.HTTP
	if httpClient == nil {
		httpClient = &http.Client{
			Transport: &ETagTransport{Store: o.Store, Base: http.DefaultTransport},
		}
	}
	return &Client{
		token:       o.Token,
		graphqlURL:  o.GraphQLURL,
		restBaseURL: strings.TrimRight(o.RESTBaseURL, "/"),
		http:        httpClient,
		Budget:      NewBudget(),
	}
}

// restGET performs a GET against the REST base URL and returns the body bytes.
// Routed through the client's transport (ETag-conditional when wired that way).
func (c *Client) restGET(ctx context.Context, path string) ([]byte, int, error) {
	url := c.restBaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	c.Budget.UpdateFromRESTHeaders(resp.Header)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode != http.StatusOK {
		return body, resp.StatusCode, fmt.Errorf("REST GET %s: status %d", path, resp.StatusCode)
	}
	return body, resp.StatusCode, nil
}
```

`internal/githubapi/graphql.go`:
```go
package githubapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// graphqlRequest is the POST payload shape.
type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// graphqlError is one entry in a GraphQL "errors" array.
type graphqlError struct {
	Message string `json:"message"`
}

// graphql POSTs a query and decodes the "data" field into target. It surfaces
// any GraphQL errors and updates the Budget from a rateLimit block if the
// decoded data contains one (target may embed `RateLimit` under "rateLimit").
func (c *Client) graphql(ctx context.Context, query string, vars map[string]any, target any) error {
	payload, err := json.Marshal(graphqlRequest{Query: query, Variables: vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.graphqlURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("graphql: status %d", resp.StatusCode)
	}

	// Decode errors and raw data first, then unmarshal data into target.
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []graphqlError  `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 {
		msgs := make([]string, len(envelope.Errors))
		for i, e := range envelope.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("graphql errors: %s", strings.Join(msgs, "; "))
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return fmt.Errorf("graphql: empty data")
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return err
	}

	// Opportunistically update the budget if the data carries a rateLimit block.
	var rl struct {
		RateLimit RateLimit `json:"rateLimit"`
	}
	if err := json.Unmarshal(envelope.Data, &rl); err == nil && rl.RateLimit.ResetAt != "" {
		c.Budget.UpdateFromGraphQL(rl.RateLimit)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/githubapi/ -run TestGraphQL -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/githubapi/client.go internal/githubapi/graphql.go internal/githubapi/graphql_test.go
git commit -m "feat: graphql client core with rateLimit + error handling"
```

---

## Task 11: Typed paged fetchers (repo meta, commits, PRs, issues, releases)

**Files:**
- Create: `internal/githubapi/fetch.go`, `internal/githubapi/fetch_test.go`

Each fetcher issues one GraphQL query and returns parsed `store`-ready nodes plus an `endCursor` + `hasNextPage`. Timestamps are parsed from GitHub's RFC3339 strings; `is_bot` is set via `IsBot`. The full GraphQL query strings are written out below — no abbreviation.

> Design note on commit history: spec §6 phrases this as `defaultBranchRef.target ... history`. This plan instead selects `ref(qualifiedName:$branch).target` so the fetcher is parameterized by the `branch` argument (the backfill passes the repo's resolved default branch, and M3's delta sync can pass any branch). The two are equivalent when `$branch` is the default branch. The decode struct and all test fixtures use the `ref.target.history` path accordingly.

- [ ] **Step 1: Write the failing test**

`internal/githubapi/fetch_test.go`:
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
)

// gqlResponder routes by which top-level field the query selects.
func gqlResponder(t *testing.T, byField map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		for field, resp := range byField {
			if strings.Contains(req.Query, field) {
				_, _ = w.Write([]byte(resp))
				return
			}
		}
		t.Errorf("no canned response for query: %s", req.Query)
		w.WriteHeader(500)
	}
}

func TestFetchRepoMeta(t *testing.T) {
	srv := httptest.NewServer(gqlResponder(t, map[string]string{
		"databaseId": `{"data":{"repository":{
			"databaseId": 123, "nameWithOwner":"octocat/hello", "isPrivate":true,
			"description":"hi", "stargazerCount":7, "forkCount":2,
			"defaultBranchRef":{"name":"main"},
			"rateLimit":{"cost":1,"remaining":4999,"resetAt":"2026-04-01T13:00:00Z"}
		}}}`,
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	r, err := c.FetchRepoMeta(context.Background(), "octocat", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if r.GitHubID != 123 || r.FullName != "octocat/hello" || !r.IsPrivate ||
		r.DefaultBranch != "main" || r.Stargazers != 7 || r.Forks != 2 {
		t.Fatalf("repo meta = %+v", r)
	}
}

func TestFetchCommitsPage(t *testing.T) {
	srv := httptest.NewServer(gqlResponder(t, map[string]string{
		"history": `{"data":{"repository":{"ref":{"target":{"history":{
			"pageInfo":{"endCursor":"CUR1","hasNextPage":true},
			"nodes":[
				{"oid":"sha1","additions":10,"deletions":2,"committedDate":"2026-03-01T08:00:00Z",
				 "messageHeadline":"first","author":{"user":{"login":"neo"}}},
				{"oid":"sha2","additions":0,"deletions":0,"committedDate":"2026-03-01T09:00:00Z",
				 "messageHeadline":"bot bump","author":{"user":{"login":"dependabot[bot]"}}}
			]
		}}}},"rateLimit":{"cost":1,"remaining":4998,"resetAt":"2026-04-01T13:00:00Z"}}}`,
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	page, err := c.FetchCommits(context.Background(), "octocat", "hello", "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Commits) != 2 || page.EndCursor != "CUR1" || !page.HasNextPage {
		t.Fatalf("commits page = %+v", page)
	}
	if page.Commits[0].SHA != "sha1" || page.Commits[0].AuthorLogin != "neo" ||
		page.Commits[0].Additions != 10 || page.Commits[0].MsgFirstLine != "first" {
		t.Fatalf("commit[0] = %+v", page.Commits[0])
	}
	if !page.Commits[1].IsBot {
		t.Fatalf("commit[1] should be flagged bot: %+v", page.Commits[1])
	}
}

func TestFetchPullRequestsPage(t *testing.T) {
	srv := httptest.NewServer(gqlResponder(t, map[string]string{
		"pullRequests": `{"data":{"repository":{"pullRequests":{
			"pageInfo":{"endCursor":"PR1","hasNextPage":false},
			"nodes":[
				{"number":1,"state":"MERGED","title":"add x","createdAt":"2026-03-01T07:00:00Z",
				 "mergedAt":"2026-03-01T18:00:00Z","closedAt":"2026-03-01T18:00:00Z",
				 "additions":10,"deletions":2,"changedFiles":3,
				 "author":{"login":"neo"},"comments":{"totalCount":4},
				 "reviews":{"nodes":[{"submittedAt":"2026-03-01T12:00:00Z"}]}},
				{"number":2,"state":"OPEN","title":"bump","createdAt":"2026-03-02T07:00:00Z",
				 "mergedAt":null,"closedAt":null,"additions":1,"deletions":1,"changedFiles":1,
				 "author":{"login":"dependabot[bot]"},"comments":{"totalCount":0},
				 "reviews":{"nodes":[]}}
			]
		}},"rateLimit":{"cost":1,"remaining":4997,"resetAt":"2026-04-01T13:00:00Z"}}}`,
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	page, err := c.FetchPullRequests(context.Background(), "octocat", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.PRs) != 2 || page.HasNextPage {
		t.Fatalf("PR page = %+v", page)
	}
	pr := page.PRs[0]
	if pr.Number != 1 || pr.State != "MERGED" || pr.CommentsCount != 4 ||
		pr.MergedAt == nil || pr.FirstReviewAt == nil || pr.ChangedFiles != 3 {
		t.Fatalf("PR[0] = %+v", pr)
	}
	if page.PRs[1].MergedAt != nil || !page.PRs[1].IsBot {
		t.Fatalf("PR[1] = %+v", page.PRs[1])
	}
}

func TestFetchIssuesPage(t *testing.T) {
	srv := httptest.NewServer(gqlResponder(t, map[string]string{
		"issues": `{"data":{"repository":{"issues":{
			"pageInfo":{"endCursor":"IS1","hasNextPage":false},
			"nodes":[
				{"number":1,"state":"CLOSED","title":"bug","createdAt":"2026-03-01T06:00:00Z",
				 "closedAt":"2026-03-02T11:00:00Z","author":{"login":"neo"},"comments":{"totalCount":4}}
			]
		}},"rateLimit":{"cost":1,"remaining":4996,"resetAt":"2026-04-01T13:00:00Z"}}}`,
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	page, err := c.FetchIssues(context.Background(), "octocat", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Issues) != 1 || page.HasNextPage {
		t.Fatalf("issue page = %+v", page)
	}
	is := page.Issues[0]
	if is.Number != 1 || is.State != "CLOSED" || is.CommentsCount != 4 || is.ClosedAt == nil {
		t.Fatalf("issue[0] = %+v", is)
	}
}

func TestFetchReleasesPage(t *testing.T) {
	srv := httptest.NewServer(gqlResponder(t, map[string]string{
		"releases": `{"data":{"repository":{"releases":{
			"pageInfo":{"endCursor":"RE1","hasNextPage":false},
			"nodes":[
				{"tagName":"v1.0.0","name":"First","publishedAt":"2026-03-01T12:00:00Z","author":{"login":"neo"}},
				{"tagName":"v1.1.0","name":"Second","publishedAt":null,"author":null}
			]
		}},"rateLimit":{"cost":1,"remaining":4995,"resetAt":"2026-04-01T13:00:00Z"}}}`,
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	page, err := c.FetchReleases(context.Background(), "octocat", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Releases) != 2 || page.HasNextPage {
		t.Fatalf("release page = %+v", page)
	}
	if page.Releases[0].Tag != "v1.0.0" || page.Releases[0].PublishedAt == nil ||
		page.Releases[0].AuthorLogin != "neo" {
		t.Fatalf("release[0] = %+v", page.Releases[0])
	}
	if page.Releases[1].PublishedAt != nil || page.Releases[1].AuthorLogin != "" {
		t.Fatalf("release[1] = %+v", page.Releases[1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/githubapi/ -run TestFetch -v`
Expected: FAIL — `undefined: (*Client).FetchRepoMeta`.

- [ ] **Step 3: Write minimal implementation**

`internal/githubapi/fetch.go`:
```go
package githubapi

import (
	"context"
	"time"

	"github-stats/internal/store"
)

const pageSize = 100

// parseTime parses an RFC3339 timestamp; the zero value on empty/invalid input.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// parseTimePtr returns a *time.Time, nil for empty/invalid input.
func parseTimePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

// --- Repo meta -------------------------------------------------------------

const repoMetaQuery = `
query($owner:String!, $name:String!) {
  repository(owner:$owner, name:$name) {
    databaseId
    nameWithOwner
    isPrivate
    description
    stargazerCount
    forkCount
    defaultBranchRef { name }
  }
  rateLimit { cost remaining resetAt }
}`

// FetchRepoMeta returns repository metadata as a store.Repo (ID unset; the
// caller upserts to obtain the local id).
func (c *Client) FetchRepoMeta(ctx context.Context, owner, name string) (*store.Repo, error) {
	var data struct {
		Repository struct {
			DatabaseID     int64  `json:"databaseId"`
			NameWithOwner  string `json:"nameWithOwner"`
			IsPrivate      bool   `json:"isPrivate"`
			Description    string `json:"description"`
			StargazerCount int64  `json:"stargazerCount"`
			ForkCount      int64  `json:"forkCount"`
			DefaultBranch  struct {
				Name string `json:"name"`
			} `json:"defaultBranchRef"`
		} `json:"repository"`
		RateLimit RateLimit `json:"rateLimit"`
	}
	if err := c.graphql(ctx, repoMetaQuery,
		map[string]any{"owner": owner, "name": name}, &data); err != nil {
		return nil, err
	}
	r := &store.Repo{
		GitHubID:      data.Repository.DatabaseID,
		FullName:      data.Repository.NameWithOwner,
		IsPrivate:     data.Repository.IsPrivate,
		DefaultBranch: data.Repository.DefaultBranch.Name,
		Description:   data.Repository.Description,
		Stargazers:    data.Repository.StargazerCount,
		Forks:         data.Repository.ForkCount,
	}
	return r, nil
}

// --- Commits ---------------------------------------------------------------

// CommitPage is one page of commit history.
type CommitPage struct {
	Commits     []store.Commit
	EndCursor   string
	HasNextPage bool
}

const commitsQuery = `
query($owner:String!, $name:String!, $branch:String!, $after:String) {
  repository(owner:$owner, name:$name) {
    ref(qualifiedName:$branch) {
      target {
        ... on Commit {
          history(first:100, after:$after) {
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

func (c *Client) FetchCommits(ctx context.Context, owner, name, branch, after string) (*CommitPage, error) {
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
	vars := map[string]any{"owner": owner, "name": name, "branch": "refs/heads/" + branch}
	if after != "" {
		vars["after"] = after
	}
	if err := c.graphql(ctx, commitsQuery, vars, &data); err != nil {
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

// --- Pull requests ---------------------------------------------------------

// PRPage is one page of pull requests.
type PRPage struct {
	PRs         []store.PullRequest
	EndCursor   string
	HasNextPage bool
}

const pullRequestsQuery = `
query($owner:String!, $name:String!, $after:String) {
  repository(owner:$owner, name:$name) {
    pullRequests(first:100, after:$after, orderBy:{field:CREATED_AT, direction:ASC}) {
      pageInfo { endCursor hasNextPage }
      nodes {
        number
        state
        title
        createdAt
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

func (c *Client) FetchPullRequests(ctx context.Context, owner, name, after string) (*PRPage, error) {
	var data struct {
		Repository struct {
			PullRequests struct {
				PageInfo pageInfo `json:"pageInfo"`
				Nodes    []struct {
					Number       int64  `json:"number"`
					State        string `json:"state"`
					Title        string `json:"title"`
					CreatedAt    string `json:"createdAt"`
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
	if err := c.graphql(ctx, pullRequestsQuery, vars, &data); err != nil {
		return nil, err
	}
	prs := data.Repository.PullRequests
	page := &PRPage{EndCursor: prs.PageInfo.EndCursor, HasNextPage: prs.PageInfo.HasNextPage}
	for _, n := range prs.Nodes {
		var firstReview *time.Time
		if len(n.Reviews.Nodes) > 0 {
			firstReview = parseTimePtr(n.Reviews.Nodes[0].SubmittedAt)
		}
		login := n.Author.Login
		page.PRs = append(page.PRs, store.PullRequest{
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
		})
	}
	return page, nil
}

// --- Issues ----------------------------------------------------------------

// IssuePage is one page of issues.
type IssuePage struct {
	Issues      []store.Issue
	EndCursor   string
	HasNextPage bool
}

const issuesQuery = `
query($owner:String!, $name:String!, $after:String) {
  repository(owner:$owner, name:$name) {
    issues(first:100, after:$after, orderBy:{field:CREATED_AT, direction:ASC}) {
      pageInfo { endCursor hasNextPage }
      nodes {
        number
        state
        title
        createdAt
        closedAt
        author { login }
        comments { totalCount }
      }
    }
  }
  rateLimit { cost remaining resetAt }
}`

func (c *Client) FetchIssues(ctx context.Context, owner, name, after string) (*IssuePage, error) {
	var data struct {
		Repository struct {
			Issues struct {
				PageInfo pageInfo `json:"pageInfo"`
				Nodes    []struct {
					Number    int64  `json:"number"`
					State     string `json:"state"`
					Title     string `json:"title"`
					CreatedAt string `json:"createdAt"`
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
	if err := c.graphql(ctx, issuesQuery, vars, &data); err != nil {
		return nil, err
	}
	iss := data.Repository.Issues
	page := &IssuePage{EndCursor: iss.PageInfo.EndCursor, HasNextPage: iss.PageInfo.HasNextPage}
	for _, n := range iss.Nodes {
		login := n.Author.Login
		page.Issues = append(page.Issues, store.Issue{
			Number:        n.Number,
			AuthorLogin:   login,
			State:         n.State,
			CreatedAt:     parseTime(n.CreatedAt),
			ClosedAt:      parseTimePtr(n.ClosedAt),
			CommentsCount: n.Comments.TotalCount,
			IsBot:         IsBot(login),
			Title:         n.Title,
		})
	}
	return page, nil
}

// --- Releases --------------------------------------------------------------

// ReleasePage is one page of releases.
type ReleasePage struct {
	Releases    []store.Release
	EndCursor   string
	HasNextPage bool
}

const releasesQuery = `
query($owner:String!, $name:String!, $after:String) {
  repository(owner:$owner, name:$name) {
    releases(first:100, after:$after, orderBy:{field:CREATED_AT, direction:ASC}) {
      pageInfo { endCursor hasNextPage }
      nodes {
        tagName
        name
        publishedAt
        author { login }
      }
    }
  }
  rateLimit { cost remaining resetAt }
}`

func (c *Client) FetchReleases(ctx context.Context, owner, name, after string) (*ReleasePage, error) {
	var data struct {
		Repository struct {
			Releases struct {
				PageInfo pageInfo `json:"pageInfo"`
				Nodes    []struct {
					TagName     string `json:"tagName"`
					Name        string `json:"name"`
					PublishedAt string `json:"publishedAt"`
					Author      *struct {
						Login string `json:"login"`
					} `json:"author"`
				} `json:"nodes"`
			} `json:"releases"`
		} `json:"repository"`
		RateLimit RateLimit `json:"rateLimit"`
	}
	vars := map[string]any{"owner": owner, "name": name}
	if after != "" {
		vars["after"] = after
	}
	if err := c.graphql(ctx, releasesQuery, vars, &data); err != nil {
		return nil, err
	}
	rel := data.Repository.Releases
	page := &ReleasePage{EndCursor: rel.PageInfo.EndCursor, HasNextPage: rel.PageInfo.HasNextPage}
	for _, n := range rel.Nodes {
		login := ""
		if n.Author != nil {
			login = n.Author.Login
		}
		page.Releases = append(page.Releases, store.Release{
			Tag:         n.TagName,
			Name:        n.Name,
			PublishedAt: parseTimePtr(n.PublishedAt),
			AuthorLogin: login,
		})
	}
	return page, nil
}

// pageInfo is the GraphQL pagination cursor block.
type pageInfo struct {
	EndCursor   string `json:"endCursor"`
	HasNextPage bool   `json:"hasNextPage"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/githubapi/ -run TestFetch -v`
Expected: PASS (all five fetcher tests).

- [ ] **Step 5: Run the whole githubapi package**

Run: `go test ./internal/githubapi/ -v`
Expected: PASS — bot, budget, etag transport, graphql, fetchers.

- [ ] **Step 6: Commit**

```bash
git add internal/githubapi/fetch.go internal/githubapi/fetch_test.go
git commit -m "feat: typed cursor-paged fetchers for repo/commits/PRs/issues/releases"
```

---

## Task 12: Backfill orchestrator — resumable single-repo backfill

**Files:**
- Create: `internal/backfill/backfill.go`, `internal/backfill/backfill_test.go`

`Run(ctx, store, client, repoID)` refreshes repo meta, then pages each stream (commits, PRs, issues, releases) to completion. After **each page** it upserts the batch and persists the page cursor to `sync_state` (so a crash or rate-limit window resumes from the saved cursor, not page 0). It tracks the min/max event dates touched and, at the end, calls `RecomputeDailyStats` over that span, then marks `sync_state.status = "complete"` with `last_backfill_at`. The test drives `Run` against a fake GraphQL server returning two commit pages and one page each of PRs/issues/releases, then asserts events + aggregates + sync state.

- [ ] **Step 1: Write the failing test**

`internal/backfill/backfill_test.go`:
```go
package backfill

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// fakeGraphQL answers each query type, paging commits across two responses.
func fakeGraphQL(t *testing.T) http.HandlerFunc {
	var commitCalls int
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		w.Header().Set("Content-Type", "application/json")
		const rl = `"rateLimit":{"cost":1,"remaining":4990,"resetAt":"2026-04-01T13:00:00Z"}`
		switch {
		case strings.Contains(req.Query, "databaseId"):
			w.Write([]byte(`{"data":{"repository":{"databaseId":555,"nameWithOwner":"octocat/hello",
				"isPrivate":false,"description":"hi","stargazerCount":1,"forkCount":0,
				"defaultBranchRef":{"name":"main"}},` + rl + `}}`))
		case strings.Contains(req.Query, "history"):
			commitCalls++
			if commitCalls == 1 {
				w.Write([]byte(`{"data":{"repository":{"ref":{"target":{"history":{
					"pageInfo":{"endCursor":"C1","hasNextPage":true},
					"nodes":[{"oid":"sha1","additions":10,"deletions":2,
						"committedDate":"2026-03-01T08:00:00Z","messageHeadline":"first",
						"author":{"user":{"login":"neo"}}}]}}}}},` + rl + `}}`))
			} else {
				w.Write([]byte(`{"data":{"repository":{"ref":{"target":{"history":{
					"pageInfo":{"endCursor":"C2","hasNextPage":false},
					"nodes":[{"oid":"sha2","additions":3,"deletions":0,
						"committedDate":"2026-03-02T09:00:00Z","messageHeadline":"second",
						"author":{"user":{"login":"trinity"}}}]}}}}},` + rl + `}}`))
			}
		case strings.Contains(req.Query, "pullRequests"):
			w.Write([]byte(`{"data":{"repository":{"pullRequests":{
				"pageInfo":{"endCursor":"P1","hasNextPage":false},
				"nodes":[{"number":1,"state":"MERGED","title":"add","createdAt":"2026-03-01T07:00:00Z",
					"mergedAt":"2026-03-01T18:00:00Z","closedAt":"2026-03-01T18:00:00Z",
					"additions":10,"deletions":2,"changedFiles":3,"author":{"login":"neo"},
					"comments":{"totalCount":2},"reviews":{"nodes":[{"submittedAt":"2026-03-01T12:00:00Z"}]}}]
			}},` + rl + `}}`))
		case strings.Contains(req.Query, "issues"):
			w.Write([]byte(`{"data":{"repository":{"issues":{
				"pageInfo":{"endCursor":"I1","hasNextPage":false},
				"nodes":[{"number":1,"state":"OPEN","title":"bug","createdAt":"2026-03-02T06:00:00Z",
					"closedAt":null,"author":{"login":"trinity"},"comments":{"totalCount":1}}]
			}},` + rl + `}}`))
		case strings.Contains(req.Query, "releases"):
			w.Write([]byte(`{"data":{"repository":{"releases":{
				"pageInfo":{"endCursor":"R1","hasNextPage":false},
				"nodes":[{"tagName":"v1.0.0","name":"First","publishedAt":"2026-03-01T12:00:00Z",
					"author":{"login":"neo"}}]
			}},` + rl + `}}`))
		default:
			t.Errorf("unexpected query: %s", req.Query)
			w.WriteHeader(500)
		}
	}
}

func TestRunBackfillEndToEnd(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	srv := httptest.NewServer(fakeGraphQL(t))
	defer srv.Close()

	client := githubapi.NewClient(githubapi.Options{
		Token:       "gho_test",
		GraphQLURL:  srv.URL,
		RESTBaseURL: srv.URL,
		Store:       st,
		HTTP:        &http.Client{},
	})

	repoID, err := st.UpsertRepo(ctx, &store.Repo{
		GitHubID: 555, FullName: "octocat/hello", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := Run(ctx, st, client, repoID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Events: two commits (both pages), one PR, one issue, one release.
	assertCount(t, st, "SELECT COUNT(*) FROM commits WHERE repo_id=?", repoID, 2)
	assertCount(t, st, "SELECT COUNT(*) FROM pull_requests WHERE repo_id=?", repoID, 1)
	assertCount(t, st, "SELECT COUNT(*) FROM issues WHERE repo_id=?", repoID, 1)
	assertCount(t, st, "SELECT COUNT(*) FROM releases WHERE repo_id=?", repoID, 1)

	// Aggregates were recomputed across the touched span (2026-03-01..2026-03-02).
	var commitsDay1 int
	st.DB.QueryRowContext(ctx,
		`SELECT commits FROM daily_repo_stats WHERE repo_id=? AND date='2026-03-01'`, repoID,
	).Scan(&commitsDay1)
	if commitsDay1 != 1 {
		t.Fatalf("day1 commits aggregate = %d, want 1", commitsDay1)
	}
	var prsMergedDay1 int
	st.DB.QueryRowContext(ctx,
		`SELECT prs_merged FROM daily_repo_stats WHERE repo_id=? AND date='2026-03-01'`, repoID,
	).Scan(&prsMergedDay1)
	if prsMergedDay1 != 1 {
		t.Fatalf("day1 prs_merged = %d, want 1", prsMergedDay1)
	}

	// Sync state marked complete with cursors persisted.
	ss, err := st.GetSyncState(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if ss.Status != "complete" {
		t.Fatalf("status = %q, want complete", ss.Status)
	}
	if ss.LastCommitCursor != "C2" || ss.LastPRCursor != "P1" ||
		ss.LastIssueCursor != "I1" || ss.LastReleaseCursor != "R1" {
		t.Fatalf("cursors not persisted: %+v", ss)
	}
	if ss.LastBackfillAt == nil {
		t.Fatal("last_backfill_at not set")
	}

	// Repo meta refreshed from GraphQL (description + stargazers).
	r, _ := st.GetRepo(ctx, repoID)
	if r.Description != "hi" || r.Stargazers != 1 {
		t.Fatalf("repo meta not refreshed: %+v", r)
	}
}

func assertCount(t *testing.T, st *store.Store, query string, repoID int64, want int) {
	t.Helper()
	var n int
	if err := st.DB.QueryRow(query, repoID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("%s = %d, want %d", query, n, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backfill/ -run TestRunBackfill -v`
Expected: FAIL — `undefined: Run`.

- [ ] **Step 3: Write minimal implementation**

`internal/backfill/backfill.go`:
```go
// Package backfill performs a one-time, resumable, single-repo backfill of
// commits, pull requests, issues and releases into the store, recomputing daily
// aggregates over the touched date span. It orchestrates only: fetching is
// delegated to githubapi and persistence to store (spec §4/§6).
package backfill

import (
	"context"
	"strings"
	"time"

	"github-stats/internal/githubapi"
	"github-stats/internal/store"
)

// dateSpan tracks the min/max event dates touched during a backfill so the final
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

// Run performs the full backfill for repoID. It is resumable: cursors are saved
// to sync_state after every page, so a re-run after interruption continues from
// the last saved cursor rather than re-fetching from the start.
func Run(ctx context.Context, st *store.Store, client *githubapi.Client, repoID int64) error {
	repo, err := st.GetRepo(ctx, repoID)
	if err != nil {
		return err
	}
	owner, name := splitFullName(repo.FullName)

	ss, err := st.GetSyncState(ctx, repoID)
	if err != nil {
		return err
	}
	ss.Status = "backfilling"
	if err := st.UpsertSyncState(ctx, ss); err != nil {
		return err
	}

	// Refresh repo metadata (and pick up the real default branch).
	meta, err := client.FetchRepoMeta(ctx, owner, name)
	if err != nil {
		return err
	}
	meta.ID = repo.ID
	if _, err := st.UpsertRepo(ctx, meta); err != nil {
		return err
	}
	branch := meta.DefaultBranch
	if branch == "" {
		branch = repo.DefaultBranch
	}

	span := &dateSpan{}

	if err := backfillCommits(ctx, st, client, repoID, owner, name, branch, ss, span); err != nil {
		return err
	}
	if err := backfillPRs(ctx, st, client, repoID, owner, name, ss, span); err != nil {
		return err
	}
	if err := backfillIssues(ctx, st, client, repoID, owner, name, ss, span); err != nil {
		return err
	}
	if err := backfillReleases(ctx, st, client, repoID, owner, name, ss, span); err != nil {
		return err
	}

	// Recompute aggregates over the full touched span (no-op if nothing touched).
	if !span.min.IsZero() {
		from := span.min.UTC().Format("2006-01-02")
		to := span.max.UTC().Format("2006-01-02")
		if err := st.RecomputeDailyStats(ctx, repoID, from, to); err != nil {
			return err
		}
	}

	now := time.Now().UTC()
	ss.Status = "complete"
	ss.LastBackfillAt = &now
	return st.UpsertSyncState(ctx, ss)
}

func backfillCommits(ctx context.Context, st *store.Store, client *githubapi.Client,
	repoID int64, owner, name, branch string, ss *store.SyncState, span *dateSpan) error {
	after := ss.LastCommitCursor
	for {
		page, err := client.FetchCommits(ctx, owner, name, branch, after)
		if err != nil {
			return err
		}
		if err := st.UpsertCommits(ctx, repoID, page.Commits); err != nil {
			return err
		}
		for _, c := range page.Commits {
			span.add(c.CommittedAt)
			if ss.LastCommitAt == nil || c.CommittedAt.After(*ss.LastCommitAt) {
				t := c.CommittedAt
				ss.LastCommitAt = &t
			}
		}
		ss.LastCommitCursor = page.EndCursor
		if err := st.UpsertSyncState(ctx, ss); err != nil { // resumable: save cursor each page
			return err
		}
		if !page.HasNextPage {
			return nil
		}
		after = page.EndCursor
	}
}

func backfillPRs(ctx context.Context, st *store.Store, client *githubapi.Client,
	repoID int64, owner, name string, ss *store.SyncState, span *dateSpan) error {
	after := ss.LastPRCursor
	for {
		page, err := client.FetchPullRequests(ctx, owner, name, after)
		if err != nil {
			return err
		}
		if err := st.UpsertPullRequests(ctx, repoID, page.PRs); err != nil {
			return err
		}
		for _, p := range page.PRs {
			span.add(p.CreatedAt)
			if p.MergedAt != nil {
				span.add(*p.MergedAt)
			}
			if p.ClosedAt != nil {
				span.add(*p.ClosedAt)
			}
		}
		ss.LastPRCursor = page.EndCursor
		if err := st.UpsertSyncState(ctx, ss); err != nil {
			return err
		}
		if !page.HasNextPage {
			return nil
		}
		after = page.EndCursor
	}
}

func backfillIssues(ctx context.Context, st *store.Store, client *githubapi.Client,
	repoID int64, owner, name string, ss *store.SyncState, span *dateSpan) error {
	after := ss.LastIssueCursor
	for {
		page, err := client.FetchIssues(ctx, owner, name, after)
		if err != nil {
			return err
		}
		if err := st.UpsertIssues(ctx, repoID, page.Issues); err != nil {
			return err
		}
		for _, is := range page.Issues {
			span.add(is.CreatedAt)
			if is.ClosedAt != nil {
				span.add(*is.ClosedAt)
			}
		}
		ss.LastIssueCursor = page.EndCursor
		if err := st.UpsertSyncState(ctx, ss); err != nil {
			return err
		}
		if !page.HasNextPage {
			return nil
		}
		after = page.EndCursor
	}
}

func backfillReleases(ctx context.Context, st *store.Store, client *githubapi.Client,
	repoID int64, owner, name string, ss *store.SyncState, span *dateSpan) error {
	after := ss.LastReleaseCursor
	for {
		page, err := client.FetchReleases(ctx, owner, name, after)
		if err != nil {
			return err
		}
		if err := st.UpsertReleases(ctx, repoID, page.Releases); err != nil {
			return err
		}
		for _, rel := range page.Releases {
			if rel.PublishedAt != nil {
				span.add(*rel.PublishedAt)
			}
		}
		ss.LastReleaseCursor = page.EndCursor
		if err := st.UpsertSyncState(ctx, ss); err != nil {
			return err
		}
		if !page.HasNextPage {
			return nil
		}
		after = page.EndCursor
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

Run: `go test ./internal/backfill/ -run TestRunBackfill -v`
Expected: PASS.

- [ ] **Step 5: Add a resumability test (saved cursor is honoured)**

Append to `internal/backfill/backfill_test.go`:
```go
func TestRunResumesFromSavedCursor(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	// Server that fails if the commit query arrives WITHOUT a cursor — proving
	// the backfill resumed from the saved cursor instead of restarting at page 0.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.Unmarshal(raw, &req)
		w.Header().Set("Content-Type", "application/json")
		const rl = `"rateLimit":{"cost":1,"remaining":4990,"resetAt":"2026-04-01T13:00:00Z"}`
		switch {
		case strings.Contains(req.Query, "databaseId"):
			w.Write([]byte(`{"data":{"repository":{"databaseId":555,"nameWithOwner":"octocat/hello",
				"isPrivate":false,"description":"hi","stargazerCount":1,"forkCount":0,
				"defaultBranchRef":{"name":"main"}},` + rl + `}}`))
		case strings.Contains(req.Query, "history"):
			if req.Variables["after"] != "SAVED" {
				t.Errorf("expected resume cursor SAVED, got %v", req.Variables["after"])
			}
			w.Write([]byte(`{"data":{"repository":{"ref":{"target":{"history":{
				"pageInfo":{"endCursor":"C9","hasNextPage":false},
				"nodes":[{"oid":"sha9","additions":1,"deletions":0,
					"committedDate":"2026-03-03T09:00:00Z","messageHeadline":"resumed",
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
	}))
	defer srv.Close()

	client := githubapi.NewClient(githubapi.Options{
		Token: "gho_test", GraphQLURL: srv.URL, RESTBaseURL: srv.URL, Store: st, HTTP: &http.Client{},
	})
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 555, FullName: "octocat/hello", DefaultBranch: "main"})

	// Pre-seed a saved commit cursor as if a prior run was interrupted.
	if err := st.UpsertSyncState(ctx, &store.SyncState{RepoID: repoID, LastCommitCursor: "SAVED", Status: "backfilling"}); err != nil {
		t.Fatal(err)
	}

	if err := Run(ctx, st, client, repoID); err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCount(t, st, "SELECT COUNT(*) FROM commits WHERE repo_id=?", repoID, 1)
}
```

- [ ] **Step 6: Run the backfill package**

Run: `go test ./internal/backfill/ -v`
Expected: PASS (both tests).

- [ ] **Step 7: Commit**

```bash
git add internal/backfill/
git commit -m "feat: resumable single-repo backfill orchestrator"
```

---

## Task 13: Full-suite verification

**Files:**
- None (verification only)

- [ ] **Step 1: Run the entire test suite and build**

Run: `go build ./... && go test ./...`
Expected: build succeeds; **all** packages PASS (M1 packages unchanged, M2 store/githubapi/backfill green).

- [ ] **Step 2: Vet for static issues**

Run: `go vet ./...`
Expected: no findings.

- [ ] **Step 3: Commit (only if `go mod tidy` changed anything)**

```bash
go mod tidy
git add go.mod go.sum
git commit -m "chore: tidy modules after M2" || echo "nothing to commit"
```

---

## Out of scope (later milestones)

M2 deliberately excludes the following; they belong to later plans and are **not** to be built here:

- **Scheduler, `sync_jobs` job queue, worker pool, periodic delta sync, "refresh now", HTTP endpoints, and SSE progress** — all **M3**. M2 exposes a programmatic `backfill.Run`, exercised by Go tests, not an HTTP route. (The `sync_jobs` table from spec §5 is intentionally *not* created in `0002`; M3 adds it.)
- **Metrics computation** (`commit_rate`, `time_to_merge`, leaderboards, the `Metric` interface and registry) — **M4**. M2 only materializes the `daily_*` aggregates those metrics will read.
- **Dashboard UI** (React views, uPlot charts, window/exclude-bots toggles, URL shortcut) — **M5**.
- **Collections / `collection_repos` / `repo_tracking`, optional PAT credential, rate-limit UX, self-host hardening docs** — **M6**.

---

## Self-Review notes

- **Spec coverage:** every M2 bullet maps to a task. (1) `0002` migration → Task 1; repos DAO → Task 2; event batch upserts → Task 3; `sync_state` DAO → Task 4; ETag cache DAO → Task 5; `RecomputeDailyStats` → Task 6. (2) `IsBot` → Task 7; dual `Budget` (REST + GraphQL + secondary-limit backoff) → Task 8; ETag conditional REST transport (304 ⇒ cached 200, free per spec §3) → Task 9; GraphQL client + `rateLimit` decode + error surfacing → Task 10; typed cursor-paged fetchers for repo/commits/PRs/issues/releases (with `comments.totalCount`, first review timestamp, additions/deletions/changedFiles/mergedAt/closedAt) → Task 11. (3) resumable `backfill.Run` with per-page cursor saves + aggregate recompute + completion marker → Task 12.
- **No placeholders:** every code/test step contains complete, compilable Go and complete SQL. All five GraphQL query strings (`repoMetaQuery`, `commitsQuery`, `pullRequestsQuery`, `issuesQuery`, `releasesQuery`) are written out in full, as is the recompute SQL.
- **Type consistency:** names introduced early are used unchanged later — `store.Repo/Commit/PullRequest/Issue/Release/SyncState/ETagEntry`; `store.UpsertRepo/GetRepo/GetRepoByFullName/UpsertCommits/UpsertPullRequests/UpsertIssues/UpsertReleases/GetSyncState/UpsertSyncState/GetETag/PutETag/RecomputeDailyStats`; `githubapi.Client/Options/NewClient/Budget/NewBudget/RateLimit/ETagTransport/IsBot`, fetchers `FetchRepoMeta/FetchCommits/FetchPullRequests/FetchIssues/FetchReleases` and page types `CommitPage/PRPage/IssuePage/ReleasePage`; `backfill.Run`. The fetchers return `store.*` structs directly so the backfill upserts them with no translation layer.
- **TDD discipline & determinism:** each unit gets a failing test first (exact command + expected `undefined: …` FAIL), then minimal impl, then PASS. GitHub is faked exclusively via `httptest` (REST and GraphQL) — no real network. Timestamps come from fixture strings (RFC3339), so assertions are exact. `Budget.BackoffFor` takes an explicit `now` rather than calling `time.Now()`, keeping backoff tests deterministic; the only `time.Now()` use is `Run`'s `last_backfill_at`, which the test checks for non-nil only (no loose clock assertion).
- **Reused M1 surface:** `store.Open` + the embedded migration runner pick up `0002` automatically (sorted filename order, tracked in `schema_migrations`); `db.SetMaxOpenConns(1)` + WAL are inherited; the M1 `openTemp(t)` helper is reused in store tests, and a local `openTestStore(t)` mirror is used in `githubapi`/`backfill` tests (different packages can't see the store package's test helper).
- **Intentional schema deviations (flagged):** spec §5's `etags(user_id, url, etag, last_modified)` is realized as `etags(url PK, etag, status, body, last_modified, updated_at)` — body + status are required so a 304 can return the cached payload, and M2's backfill is per-repo (URL already scopes it), so `user_id` is omitted; M3 can add it for per-user scoping. `sync_jobs` (spec §5) is deferred to M3 per the milestone boundary. Both are restated under "Out of scope".
- **Batching & idempotency:** every event upsert uses one transaction + one prepared statement per page (≤100 nodes) with `ON CONFLICT` upserts, so re-ingesting a page (after a resume) never duplicates. `RecomputeDailyStats` deletes-then-rebuilds its window in a transaction, making it idempotent and drift-correcting.

---

## What M3 will add (next plan)

- **`sync_jobs` table** (`0003_*.sql`) + a job-queue DAO (lease/claim/release with `locked_at`, `attempts`, `next_run_at`).
- **Scheduler**: a 1-min ticker enqueuing `delta` jobs for repos past `next_run_at`; adding a repo enqueues a `backfill` job (wrapping `backfill.Run`).
- **Worker pool** (bounded concurrency) leasing jobs, sharing one `githubapi.Budget`; on exhaustion a job saves its cursor (already supported) and reschedules at the bucket reset time.
- **Delta sync**: `orderBy UPDATED_AT DESC` with a stop-at-`last_synced_at` overlap window; commits via `history(since:)`; REST polls (releases/repo meta) reuse the ETag transport (`304` free). Touched dates get `RecomputeDailyStats`.
- **HTTP**: `POST /api/repos` (enqueue backfill), `POST /api/repos/{id}/refresh` (enqueue delta), `GET /api/repos/{id}/sync/stream` (SSE progress).

---

## Public API surface M2 exposes

For the M3 plan to build on precisely. All under module `github-stats`.

**`internal/store` (package `store`) — new in M2 (joins existing `Store{DB *sql.DB}`):**
```go
type Repo struct { ID, GitHubID int64; FullName string; IsPrivate bool; DefaultBranch, Description string; Stargazers, Forks int64; CreatedAt time.Time }
type Commit struct { SHA, AuthorLogin string; CommittedAt time.Time; Additions, Deletions int64; IsBot bool; MsgFirstLine string }
type PullRequest struct { Number int64; AuthorLogin, State string; CreatedAt time.Time; MergedAt, ClosedAt *time.Time; Additions, Deletions, ChangedFiles, CommentsCount int64; FirstReviewAt *time.Time; IsBot bool; Title string }
type Issue struct { Number int64; AuthorLogin, State string; CreatedAt time.Time; ClosedAt *time.Time; CommentsCount int64; IsBot bool; Title string }
type Release struct { Tag, Name string; PublishedAt *time.Time; AuthorLogin string }
type SyncState struct { RepoID int64; LastCommitAt *time.Time; LastCommitCursor, LastPRCursor, LastIssueCursor, LastReleaseCursor string; LastBackfillAt *time.Time; Status string }
type ETagEntry struct { URL, ETag string; Status int; Body []byte; LastModified string }

func (s *Store) UpsertRepo(ctx context.Context, r *Repo) (int64, error)
func (s *Store) GetRepo(ctx context.Context, id int64) (*Repo, error)
func (s *Store) GetRepoByFullName(ctx context.Context, fullName string) (*Repo, error)
func (s *Store) UpsertCommits(ctx context.Context, repoID int64, commits []Commit) error
func (s *Store) UpsertPullRequests(ctx context.Context, repoID int64, prs []PullRequest) error
func (s *Store) UpsertIssues(ctx context.Context, repoID int64, issues []Issue) error
func (s *Store) UpsertReleases(ctx context.Context, repoID int64, releases []Release) error
func (s *Store) GetSyncState(ctx context.Context, repoID int64) (*SyncState, error)
func (s *Store) UpsertSyncState(ctx context.Context, st *SyncState) error
func (s *Store) GetETag(ctx context.Context, url string) (*ETagEntry, error)
func (s *Store) PutETag(ctx context.Context, e *ETagEntry) error
func (s *Store) RecomputeDailyStats(ctx context.Context, repoID int64, fromDate, toDate string) error
```

**`internal/githubapi` (package `githubapi`):**
```go
type Options struct { Token, GraphQLURL, RESTBaseURL string; Store *store.Store; HTTP *http.Client }
type Client struct { /* unexported fields */; Budget *Budget }
func NewClient(o Options) *Client

type RateLimit struct { Cost, Remaining int; ResetAt string }
type Budget struct { /* unexported, concurrency-safe */ }
func NewBudget() *Budget
func (b *Budget) UpdateFromRESTHeaders(h http.Header)
func (b *Budget) UpdateFromGraphQL(rl RateLimit)
func (b *Budget) REST() (remaining int, reset time.Time)
func (b *Budget) GraphQL() (remaining int, reset time.Time)
func (b *Budget) RESTExhausted() bool
func (b *Budget) GraphQLExhausted() bool
func (b *Budget) BackoffFor(status int, h http.Header, now time.Time) time.Duration

type ETagTransport struct { Store *store.Store; Base http.RoundTripper } // implements http.RoundTripper

func IsBot(login string) bool

type CommitPage  struct { Commits  []store.Commit;      EndCursor string; HasNextPage bool }
type PRPage      struct { PRs      []store.PullRequest; EndCursor string; HasNextPage bool }
type IssuePage   struct { Issues   []store.Issue;       EndCursor string; HasNextPage bool }
type ReleasePage struct { Releases []store.Release;     EndCursor string; HasNextPage bool }

func (c *Client) FetchRepoMeta(ctx context.Context, owner, name string) (*store.Repo, error)
func (c *Client) FetchCommits(ctx context.Context, owner, name, branch, after string) (*CommitPage, error)
func (c *Client) FetchPullRequests(ctx context.Context, owner, name, after string) (*PRPage, error)
func (c *Client) FetchIssues(ctx context.Context, owner, name, after string) (*IssuePage, error)
func (c *Client) FetchReleases(ctx context.Context, owner, name, after string) (*ReleasePage, error)
```

**`internal/backfill` (package `backfill`):**
```go
func Run(ctx context.Context, st *store.Store, client *githubapi.Client, repoID int64) error
```
