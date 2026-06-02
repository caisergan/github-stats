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
