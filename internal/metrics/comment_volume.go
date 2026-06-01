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
