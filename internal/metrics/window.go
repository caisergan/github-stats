package metrics

import (
	"context"
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// Window is a concrete inclusive UTC date range [From, To] (each 'YYYY-MM-DD').
type Window struct {
	From string
	To   string
}

// EarliestSource is the slice of Source that ParseWindow needs for "all".
type EarliestSource interface {
	EarliestEventDate(ctx context.Context, repoID int64) (string, error)
}

// ParseWindow turns a window spec ("30d"|"90d"|"6m"|"1y"|"all"|"") into a concrete
// [From, To] range anchored at now() (UTC). An empty spec defaults to "30d".
// "all" spans from the repo's earliest event date (via src) to today; when the
// repo has no events it collapses to today/today.
func ParseWindow(ctx context.Context, spec string, repoID int64, src EarliestSource, now func() time.Time) (Window, error) {
	if now == nil {
		now = time.Now
	}
	today := now().UTC()
	to := today.Format(dateLayout)

	if spec == "" {
		spec = "30d"
	}
	if spec == "all" {
		from := to
		if src != nil {
			earliest, err := src.EarliestEventDate(ctx, repoID)
			if err == nil && earliest != "" {
				from = earliest
			}
		}
		return Window{From: from, To: to}, nil
	}

	var from time.Time
	switch spec {
	case "30d":
		from = today.AddDate(0, 0, -30)
	case "90d":
		from = today.AddDate(0, 0, -90)
	case "6m":
		from = today.AddDate(0, -6, 0)
	case "1y":
		from = today.AddDate(-1, 0, 0)
	default:
		return Window{}, fmt.Errorf("unknown window spec %q", spec)
	}
	return Window{From: from.Format(dateLayout), To: to}, nil
}

// Dates returns every date in [From, To] inclusive as 'YYYY-MM-DD' strings.
func (w Window) Dates() ([]string, error) {
	from, err := time.Parse(dateLayout, w.From)
	if err != nil {
		return nil, err
	}
	to, err := time.Parse(dateLayout, w.To)
	if err != nil {
		return nil, err
	}
	var out []string
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format(dateLayout))
	}
	return out, nil
}

// ToTime returns w.To parsed as the end-of-day instant (To 23:59:59.999... is
// approximated as the next midnight) for "as of" queries. Callers that need an
// inclusive cutoff use the returned time directly.
func (w Window) ToTime() (time.Time, error) {
	to, err := time.Parse(dateLayout, w.To)
	if err != nil {
		return time.Time{}, err
	}
	return to.AddDate(0, 0, 1), nil
}
