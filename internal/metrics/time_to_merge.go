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
