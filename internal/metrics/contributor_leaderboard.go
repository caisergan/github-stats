package metrics

import (
	"context"
	"sort"

	"github-stats/internal/botident"
)

// contributorLeaderboard ranks contributors by commits (then additions, then
// login) over the window, from the login-keyed daily contributor aggregate.
type contributorLeaderboard struct{}

func (contributorLeaderboard) Key() string { return "contributor_leaderboard" }

func (contributorLeaderboard) Compute(ctx context.Context, src Source, repoID int64, w Window, opts Opts) (Result, error) {
	rows, err := src.DailyContributorStats(ctx, repoID, w.From, w.To)
	if err != nil {
		return Result{}, err
	}
	agg := make(map[string]*LeaderRow)
	order := make([]string, 0)
	for _, r := range rows {
		if opts.ExcludeBots && botident.IsBot(r.Login) {
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
