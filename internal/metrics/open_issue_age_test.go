package metrics

import (
	"context"
	"testing"
)

func TestOpenIssueAgeBuckets(t *testing.T) {
	// Window ends 2026-03-31; ToTime() = 2026-04-01T00:00:00Z is the asOf cutoff.
	src := &fakeSource{openIssues: []openRow{
		{row: OpenIssueRow{Number: 1, CreatedAt: mustTime("2026-03-31T06:00:00Z")}},  // ~18h → <24h
		{row: OpenIssueRow{Number: 2, CreatedAt: mustTime("2026-03-28T00:00:00Z")}},  // 4d → <7d
		{row: OpenIssueRow{Number: 3, CreatedAt: mustTime("2026-03-10T00:00:00Z")}},  // 22d → <30d
		{row: OpenIssueRow{Number: 4, CreatedAt: mustTime("2026-02-01T00:00:00Z")}},  // 59d → <90d
		{row: OpenIssueRow{Number: 5, CreatedAt: mustTime("2025-12-01T00:00:00Z")}},  // 121d → <180d
		{row: OpenIssueRow{Number: 6, CreatedAt: mustTime("2025-01-01T00:00:00Z")}, isBot: true}, // >180d → older (bot)
	}}
	w := Window{From: "2026-03-01", To: "2026-03-31"}

	res, err := openIssueAge{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindBuckets {
		t.Fatalf("kind = %v", res.Kind)
	}
	want := []Bucket{
		{"<24h", 1}, {"<7d", 1}, {"<30d", 1}, {"<90d", 1}, {"<180d", 1}, {"older", 1},
	}
	if len(res.Buckets) != len(want) {
		t.Fatalf("buckets = %+v", res.Buckets)
	}
	for i, b := range want {
		if res.Buckets[i] != b {
			t.Fatalf("bucket[%d] = %+v, want %+v", i, res.Buckets[i], b)
		}
	}

	// exclude_bots drops issue 6 → older bucket becomes 0 (but still present).
	res2, _ := openIssueAge{}.Compute(context.Background(), src, 1, w, Opts{ExcludeBots: true})
	if res2.Buckets[5] != (Bucket{"older", 0}) {
		t.Fatalf("excl bots older bucket = %+v, want 0", res2.Buckets[5])
	}
}
