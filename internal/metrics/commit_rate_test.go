package metrics

import (
	"context"
	"testing"
)

func TestCommitRateDenseSeries(t *testing.T) {
	src := &fakeSource{daily: []DailyRepoStatsRow{
		{Date: "2026-03-01", Commits: 2},
		{Date: "2026-03-03", Commits: 5},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-03"}
	res, err := commitRate{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindTimeSeries {
		t.Fatalf("kind = %v, want time_series", res.Kind)
	}
	if len(res.Series) != 3 {
		t.Fatalf("series len = %d, want 3 (dense)", len(res.Series))
	}
	want := []Point{{"2026-03-01", 2}, {"2026-03-02", 0}, {"2026-03-03", 5}}
	for i, p := range want {
		if res.Series[i] != p {
			t.Fatalf("point[%d] = %+v, want %+v", i, res.Series[i], p)
		}
	}
}

func TestCommitRateKey(t *testing.T) {
	m := commitRate{}
	if m.Key() != "commit_rate" {
		t.Fatalf("key = %q", m.Key())
	}
}
