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
