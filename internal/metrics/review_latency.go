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
