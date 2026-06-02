package metrics

import (
	"context"
	"testing"
)

func TestDefaultRegistryKeys(t *testing.T) {
	reg := DefaultRegistry()
	keys := reg.Keys()
	want := []string{
		"code_churn", "comment_volume", "commit_rate", "contributor_leaderboard",
		"issue_lifetime", "open_issue_age", "pr_throughput", "review_latency", "time_to_merge",
	}
	if len(keys) != len(want) {
		t.Fatalf("keys = %v (%d), want %d", keys, len(keys), len(want))
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys[%d] = %q, want %q", i, keys[i], want[i])
		}
	}
}

func TestDefaultRegistryComputeAll(t *testing.T) {
	reg := DefaultRegistry()
	src := &fakeSource{
		daily: []DailyRepoStatsRow{{Date: "2026-03-01", Commits: 1, Additions: 2, Deletions: 1, PRsMerged: 1, Comments: 3}},
		contrib: []DailyContribRow{{Date: "2026-03-01", Login: "neo", Commits: 1, Additions: 2, Deletions: 1}},
		mergedPRs: []prRow{{row: PRDurationRow{Number: 1, CreatedAt: mustTime("2026-03-01T00:00:00Z"), MergedAt: mustTime("2026-03-01T06:00:00Z")}}},
		reviews: []reviewRow{{row: ReviewLatencyRow{Number: 1, CreatedAt: mustTime("2026-03-01T00:00:00Z"), FirstReviewAt: mustTime("2026-03-01T02:00:00Z")}}},
		closedIssues: []issueRow{{row: IssueLifetimeRow{Number: 1, CreatedAt: mustTime("2026-03-01T00:00:00Z"), ClosedAt: mustTime("2026-03-02T00:00:00Z")}}},
		openIssues: []openRow{{row: OpenIssueRow{Number: 2, CreatedAt: mustTime("2026-03-15T00:00:00Z")}}},
	}
	w := Window{From: "2026-03-01", To: "2026-03-31"}
	out, err := reg.Compute(context.Background(), src, 1, nil, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 9 {
		t.Fatalf("computed %d metrics, want 9", len(out))
	}
	// Spot-check a representative of each kind.
	if out["commit_rate"].Kind != KindTimeSeries {
		t.Fatalf("commit_rate kind = %v", out["commit_rate"].Kind)
	}
	if out["time_to_merge"].Kind != KindScalar {
		t.Fatalf("time_to_merge kind = %v", out["time_to_merge"].Kind)
	}
	if out["open_issue_age"].Kind != KindBuckets {
		t.Fatalf("open_issue_age kind = %v", out["open_issue_age"].Kind)
	}
	if out["contributor_leaderboard"].Kind != KindLeaderboard {
		t.Fatalf("contributor_leaderboard kind = %v", out["contributor_leaderboard"].Kind)
	}
}
