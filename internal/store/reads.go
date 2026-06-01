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
			AND substr(merged_at,1,10) >= ? AND substr(merged_at,1,10) <= ?`+botFilter(excludeBots)+`
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
			AND substr(first_review_at,1,10) >= ? AND substr(first_review_at,1,10) <= ?`+botFilter(excludeBots)+`
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
			AND substr(closed_at,1,10) >= ? AND substr(closed_at,1,10) <= ?`+botFilter(excludeBots)+`
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
			SELECT MIN(substr(committed_at,1,10)) AS day FROM commits WHERE repo_id = ?1
			UNION ALL
			SELECT MIN(substr(created_at,1,10)) FROM pull_requests WHERE repo_id = ?1
			UNION ALL
			SELECT MIN(substr(created_at,1,10)) FROM issues WHERE repo_id = ?1
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
		WHERE repo_id = ? AND substr(committed_at,1,10) >= ? AND substr(committed_at,1,10) <= ?`+botFilter(excludeBots),
		repoID, fromDate, toDate).Scan(&n)
	return n, err
}

// CountReleases counts releases for a repo.
func (s *Store) CountReleases(ctx context.Context, repoID int64) (int64, error) {
	var n int64
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM releases WHERE repo_id = ?`, repoID).Scan(&n)
	return n, err
}
