package metrics

import (
	"context"
	"testing"
)

func TestCommentVolumeSeries(t *testing.T) {
	src := &fakeSource{daily: []DailyRepoStatsRow{
		{Date: "2026-03-01", Comments: 7},
		{Date: "2026-03-03", Comments: 2},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-03"}
	res, err := commentVolume{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindTimeSeries || len(res.Series) != 3 {
		t.Fatalf("res = %+v", res)
	}
	if res.Series[0].Value != 7 || res.Series[1].Value != 0 || res.Series[2].Value != 2 {
		t.Fatalf("comment values = %v", res.Series)
	}
}
