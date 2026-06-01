package metrics

import (
	"context"
	"testing"
)

func TestCodeChurnSeries(t *testing.T) {
	src := &fakeSource{daily: []DailyRepoStatsRow{
		{Date: "2026-03-01", Additions: 10, Deletions: 2},
		{Date: "2026-03-02", Additions: 3, Deletions: 1},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-02"}
	res, err := codeChurn{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindTimeSeries || len(res.Series) != 2 {
		t.Fatalf("res = %+v", res)
	}
	if res.Series[0].Value != 12 || res.Series[1].Value != 4 {
		t.Fatalf("churn values = %v", res.Series)
	}
}
