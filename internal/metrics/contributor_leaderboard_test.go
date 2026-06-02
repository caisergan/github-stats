package metrics

import (
	"context"
	"testing"
)

// TestContributorLeaderboardExcludesAllowlistBots guards that exclude_bots drops
// bot logins that lack the "[bot]" suffix (e.g. GitHub's "web-flow" merge author,
// or a bare "dependabot"), matching the is_bot flag ingest stamps via botident.
func TestContributorLeaderboardExcludesAllowlistBots(t *testing.T) {
	src := &fakeSource{contrib: []DailyContribRow{
		{Date: "2026-03-01", Login: "neo", Commits: 3, Additions: 30, Deletions: 5},
		{Date: "2026-03-01", Login: "web-flow", Commits: 50, Additions: 0, Deletions: 0},
		{Date: "2026-03-01", Login: "github-actions", Commits: 12, Additions: 4, Deletions: 0},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-31"}

	res, err := contributorLeaderboard{}.Compute(context.Background(), src, 1, w, Opts{ExcludeBots: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0].Login != "neo" {
		t.Fatalf("exclude_bots should drop web-flow and github-actions, got %+v", res.Rows)
	}
}

func TestContributorLeaderboardRanking(t *testing.T) {
	src := &fakeSource{contrib: []DailyContribRow{
		{Date: "2026-03-01", Login: "neo", Commits: 3, Additions: 30, Deletions: 5},
		{Date: "2026-03-02", Login: "neo", Commits: 2, Additions: 10, Deletions: 1},
		{Date: "2026-03-01", Login: "trinity", Commits: 4, Additions: 8, Deletions: 0},
		{Date: "2026-03-01", Login: "dependabot[bot]", Commits: 9, Additions: 100, Deletions: 0},
	}}
	w := Window{From: "2026-03-01", To: "2026-03-31"}

	res, err := contributorLeaderboard{}.Compute(context.Background(), src, 1, w, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindLeaderboard {
		t.Fatalf("kind = %v", res.Kind)
	}
	// Incl bots: dependabot (9) > neo (5) > trinity (4).
	if len(res.Rows) != 3 {
		t.Fatalf("rows = %+v", res.Rows)
	}
	if res.Rows[0].Login != "dependabot[bot]" || res.Rows[0].Commits != 9 {
		t.Fatalf("row0 = %+v", res.Rows[0])
	}
	if res.Rows[1].Login != "neo" || res.Rows[1].Commits != 5 || res.Rows[1].Additions != 40 || res.Rows[1].Deletions != 6 {
		t.Fatalf("row1 = %+v", res.Rows[1])
	}

	// exclude_bots drops dependabot → neo (5) > trinity (4).
	res2, _ := contributorLeaderboard{}.Compute(context.Background(), src, 1, w, Opts{ExcludeBots: true})
	if len(res2.Rows) != 2 || res2.Rows[0].Login != "neo" {
		t.Fatalf("excl bots rows = %+v", res2.Rows)
	}
}
