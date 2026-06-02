package metrics

import (
	"context"
	"testing"
)

func TestReviewLatencyMedianHours(t *testing.T) {
	src := &fakeSource{reviews: []reviewRow{
		// 2h and 6h → median 4h.
		{row: ReviewLatencyRow{Number: 1, CreatedAt: mustTime("2026-03-01T00:00:00Z"), FirstReviewAt: mustTime("2026-03-01T02:00:00Z")}},
		{row: ReviewLatencyRow{Number: 2, CreatedAt: mustTime("2026-03-02T00:00:00Z"), FirstReviewAt: mustTime("2026-03-02T06:00:00Z")}},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-31"}
	res, err := reviewLatency{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindScalar || res.Value == nil || *res.Value != 4 || *res.Count != 2 || res.Unit != "hours" {
		t.Fatalf("review latency = %+v", res)
	}
}
