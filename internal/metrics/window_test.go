package metrics

import (
	"context"
	"testing"
	"time"
)

func mustTime(s string) time.Time {
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return v
}

func TestParseWindowRelative(t *testing.T) {
	// Anchor on a mid-month date so AddDate month/year math never overflows a
	// short month-end (e.g. AddDate(0,-6,0) on the 31st would normalize forward).
	now := mustTime("2026-03-15T12:00:00Z")
	cases := []struct {
		spec     string
		wantFrom string
		wantTo   string
	}{
		{"30d", "2026-02-13", "2026-03-15"},
		{"90d", "2025-12-15", "2026-03-15"},
		{"6m", "2025-09-15", "2026-03-15"},
		{"1y", "2025-03-15", "2026-03-15"},
	}
	for _, c := range cases {
		w, err := ParseWindow(context.Background(), c.spec, 0, nil, func() time.Time { return now })
		if err != nil {
			t.Fatalf("%s: %v", c.spec, err)
		}
		if w.From != c.wantFrom || w.To != c.wantTo {
			t.Errorf("%s: got [%s,%s], want [%s,%s]", c.spec, w.From, w.To, c.wantFrom, c.wantTo)
		}
	}
}

func TestParseWindowDefaultsTo30d(t *testing.T) {
	now := mustTime("2026-03-15T12:00:00Z")
	w, err := ParseWindow(context.Background(), "", 0, nil, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if w.From != "2026-02-13" || w.To != "2026-03-15" {
		t.Fatalf("default window = [%s,%s], want 30d", w.From, w.To)
	}
}

func TestParseWindowAllUsesEarliest(t *testing.T) {
	now := mustTime("2026-03-31T12:00:00Z")
	src := &windowFakeSource{earliest: "2025-01-15"}
	w, err := ParseWindow(context.Background(), "all", 7, src, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if w.From != "2025-01-15" || w.To != "2026-03-31" {
		t.Fatalf("all window = [%s,%s], want [2025-01-15,2026-03-31]", w.From, w.To)
	}
}

func TestParseWindowAllNoData(t *testing.T) {
	now := mustTime("2026-03-31T12:00:00Z")
	src := &windowFakeSource{earliestErr: errNoData}
	w, err := ParseWindow(context.Background(), "all", 7, src, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	// No data: "all" falls back to a single day (To == From == today).
	if w.From != "2026-03-31" || w.To != "2026-03-31" {
		t.Fatalf("all/no-data window = [%s,%s], want today/today", w.From, w.To)
	}
}

func TestParseWindowRejectsBadSpec(t *testing.T) {
	now := mustTime("2026-03-31T12:00:00Z")
	if _, err := ParseWindow(context.Background(), "7w", 0, nil, func() time.Time { return now }); err == nil {
		t.Fatal("expected error for bad window spec")
	}
}

func TestWindowDays(t *testing.T) {
	w := Window{From: "2026-03-01", To: "2026-03-03"}
	got, err := w.Dates()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-03-01", "2026-03-02", "2026-03-03"}
	if len(got) != len(want) {
		t.Fatalf("dates len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dates[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

// windowFakeSource is a tiny Source used only by the window tests; it implements
// just EarliestEventDate (the rest panic if called).
type windowFakeSource struct {
	earliest    string
	earliestErr error
}

var errNoData = &windowErr{"no data"}

type windowErr struct{ s string }

func (e *windowErr) Error() string { return e.s }

func (f *windowFakeSource) EarliestEventDate(ctx context.Context, repoID int64) (string, error) {
	return f.earliest, f.earliestErr
}
func (f *windowFakeSource) DailyRepoStats(context.Context, int64, string, string) ([]DailyRepoStatsRow, error) {
	panic("unused")
}
func (f *windowFakeSource) DailyContributorStats(context.Context, int64, string, string) ([]DailyContribRow, error) {
	panic("unused")
}
func (f *windowFakeSource) MergedPRDurations(context.Context, int64, string, string, bool) ([]PRDurationRow, error) {
	panic("unused")
}
func (f *windowFakeSource) ReviewLatencies(context.Context, int64, string, string, bool) ([]ReviewLatencyRow, error) {
	panic("unused")
}
func (f *windowFakeSource) ClosedIssueLifetimes(context.Context, int64, string, string, bool) ([]IssueLifetimeRow, error) {
	panic("unused")
}
func (f *windowFakeSource) OpenIssuesAsOf(context.Context, int64, time.Time, bool) ([]OpenIssueRow, error) {
	panic("unused")
}
