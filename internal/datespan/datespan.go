// Package datespan tracks the min/max event timestamps touched during a sync so
// aggregate recomputation can cover exactly the affected date range. It is
// shared by the backfill and sync (delta) packages, which both feed
// store.RecomputeDailyStats with a from/to day range.
package datespan

import "time"

// Span accumulates the earliest and latest non-zero timestamps it is shown.
// The zero Span is ready to use.
type Span struct {
	min, max time.Time
}

// Add widens the span to include t. Zero times are ignored.
func (s *Span) Add(t time.Time) {
	if t.IsZero() {
		return
	}
	if s.min.IsZero() || t.Before(s.min) {
		s.min = t
	}
	if s.max.IsZero() || t.After(s.max) {
		s.max = t
	}
}

// Empty reports whether the span has seen no (non-zero) timestamps.
func (s *Span) Empty() bool { return s.min.IsZero() }

// Range returns the UTC day bounds ("2006-01-02") of the span. Callers should
// guard with Empty first; on an empty span both values are the zero date.
func (s *Span) Range() (from, to string) {
	const day = "2006-01-02"
	return s.min.UTC().Format(day), s.max.UTC().Format(day)
}
