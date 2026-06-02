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
