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
