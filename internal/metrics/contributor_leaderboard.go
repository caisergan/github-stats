package metrics

import (
	"context"
	"sort"
	"strings"
)

// contributorLeaderboard ranks contributors by commits (then additions, then
// login) over the window, from the login-keyed daily contributor aggregate.
type contributorLeaderboard struct{}

func (contributorLeaderboard) Key() string { return "contributor_leaderboard" }

// looksLikeBot mirrors githubapi.IsBot's suffix rule without importing the fetch
// package (keeps the metrics layer free of githubapi).
func looksLikeBot(login string) bool {
	return strings.HasSuffix(login, "[bot]")
}

func (contributorLeaderboard) Compute(ctx context.Context, src Source, repoID int64, w Window, opts Opts) (Result, error) {
	rows, err := src.DailyContributorStats(ctx, repoID, w.From, w.To)
	if err != nil {
		return Result{}, err
	}
	agg := make(map[string]*LeaderRow)
	order := make([]string, 0)
	for _, r := range rows {
		if opts.ExcludeBots && looksLikeBot(r.Login) {
			continue
		}
		lr, ok := agg[r.Login]
		if !ok {
			lr = &LeaderRow{Login: r.Login}
			agg[r.Login] = lr
			order = append(order, r.Login)
		}
		lr.Commits += r.Commits
		lr.Additions += r.Additions
		lr.Deletions += r.Deletions
	}
	out := make([]LeaderRow, 0, len(order))
	for _, login := range order {
		out = append(out, *agg[login])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Commits != out[j].Commits {
			return out[i].Commits > out[j].Commits
		}
		if out[i].Additions != out[j].Additions {
			return out[i].Additions > out[j].Additions
		}
		return out[i].Login < out[j].Login
	})
	return Leaderboard("Contributor leaderboard", out), nil
}
