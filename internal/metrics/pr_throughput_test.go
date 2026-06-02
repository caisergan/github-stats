package metrics

import (
	"context"
	"testing"
)

func TestPRThroughputMergedPerDay(t *testing.T) {
	src := &fakeSource{daily: []DailyRepoStatsRow{
		{Date: "2026-03-01", PRsMerged: 1},
		{Date: "2026-03-02", PRsMerged: 3},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-02"}
	res, err := prThroughput{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindTimeSeries || len(res.Series) != 2 {
		t.Fatalf("res = %+v", res)
	}
	if res.Series[0].Value != 1 || res.Series[1].Value != 3 {
		t.Fatalf("values = %v", res.Series)
	}
}
