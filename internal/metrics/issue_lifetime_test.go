package metrics

import (
	"context"
	"testing"
)

func TestIssueLifetimeMedianHours(t *testing.T) {
	src := &fakeSource{closedIssues: []issueRow{
		// 24h, 48h, 72h → median 48h.
		{row: IssueLifetimeRow{Number: 1, CreatedAt: mustTime("2026-03-01T00:00:00Z"), ClosedAt: mustTime("2026-03-02T00:00:00Z")}},
		{row: IssueLifetimeRow{Number: 2, CreatedAt: mustTime("2026-03-01T00:00:00Z"), ClosedAt: mustTime("2026-03-03T00:00:00Z")}},
		{row: IssueLifetimeRow{Number: 3, CreatedAt: mustTime("2026-03-01T00:00:00Z"), ClosedAt: mustTime("2026-03-04T00:00:00Z")}},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-31"}
	res, err := issueLifetime{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindScalar || res.Value == nil || *res.Value != 48 || *res.Count != 3 {
		t.Fatalf("issue lifetime = %+v", res)
	}
}
