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
