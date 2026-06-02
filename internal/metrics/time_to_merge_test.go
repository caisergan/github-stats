package metrics

import (
	"context"
	"testing"
)

func TestTimeToMergeMedianHours(t *testing.T) {
	src := &fakeSource{mergedPRs: []prRow{
		// 12h, 24h, 36h durations.
		{row: PRDurationRow{Number: 1, CreatedAt: mustTime("2026-03-01T00:00:00Z"), MergedAt: mustTime("2026-03-01T12:00:00Z")}},
		{row: PRDurationRow{Number: 2, CreatedAt: mustTime("2026-03-02T00:00:00Z"), MergedAt: mustTime("2026-03-03T00:00:00Z")}},
		{row: PRDurationRow{Number: 3, CreatedAt: mustTime("2026-03-04T00:00:00Z"), MergedAt: mustTime("2026-03-05T12:00:00Z")}, isBot: true},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-31"}

	res, err := timeToMerge{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindScalar {
		t.Fatalf("kind = %v", res.Kind)
	}
	// All three: durations 12,24,36 → median 24h.
	if res.Value == nil || *res.Value != 24 {
		t.Fatalf("median = %v, want 24", res.Value)
	}
	if res.Count == nil || *res.Count != 3 {
		t.Fatalf("count = %v, want 3", res.Count)
	}
	if res.Unit != "hours" {
		t.Fatalf("unit = %q", res.Unit)
	}

	// exclude_bots drops PR3 → durations 12,24 → median 18h, count 2.
	res2, _ := timeToMerge{}.Compute(context.Background(), src, 1, w, Opts{ExcludeBots: true})
	if res2.Value == nil || *res2.Value != 18 || *res2.Count != 2 {
		t.Fatalf("excl bots: value=%v count=%v", res2.Value, res2.Count)
	}
}

func TestTimeToMergeEmpty(t *testing.T) {
	res, err := timeToMerge{}.Compute(context.Background(), &fakeSource{}, 1, Window{From: "2026-03-01", To: "2026-03-31"}, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Value == nil || *res.Value != 0 || *res.Count != 0 {
		t.Fatalf("empty time_to_merge = %+v", res)
	}
}
