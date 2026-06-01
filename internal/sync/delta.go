package sync

import (
	"context"
	"strings"
	"time"

	"github-stats/internal/githubapi"
	"github-stats/internal/store"
)

// overlapWindow is subtracted from the last-sync cutoff so edits to items that
// changed shortly before the previous sync are re-pulled (spec §6).
const overlapWindow = 24 * time.Hour

// freshLookback bounds the first delta of a repo that has no recorded
// LastCommitAt yet (full history is the backfill job's responsibility).
const freshLookback = 14 * 24 * time.Hour

// RunDelta performs an incremental sync of repoID: commits since the last
// recorded commit time, and PRs/issues updated since that time minus an overlap
// window. It upserts events, recomputes daily aggregates for the touched span,
// and advances sync_state. `now` is injected for deterministic tests.
func RunDelta(ctx context.Context, st *store.Store, client *githubapi.Client, repoID int64, now func() time.Time) error {
	repo, err := st.GetRepo(ctx, repoID)
	if err != nil {
		return err
	}
	owner, name := splitFullName(repo.FullName)

	ss, err := st.GetSyncState(ctx, repoID)
	if err != nil {
		return err
	}

	var since time.Time
	if ss.LastCommitAt != nil {
		since = *ss.LastCommitAt
	} else {
		since = now().Add(-freshLookback)
	}
	cutoff := since.Add(-overlapWindow)

	span := &dateSpan{}
	newest := since

	// --- Commits since ---
	after := ""
	for {
		page, err := client.FetchCommitsSince(ctx, owner, name, repo.DefaultBranch, since, after)
		if err != nil {
			return err
		}
		if err := st.UpsertCommits(ctx, repoID, page.Commits); err != nil {
			return err
		}
		for _, c := range page.Commits {
			span.add(c.CommittedAt)
			if c.CommittedAt.After(newest) {
				newest = c.CommittedAt
			}
		}
		if !page.HasNextPage {
			break
		}
		after = page.EndCursor
	}

	// --- Pull requests updated (stop at cutoff) ---
	after = ""
prLoop:
	for {
		page, err := client.FetchPullRequestsUpdated(ctx, owner, name, after)
		if err != nil {
			return err
		}
		var batch []store.PullRequest
		for _, up := range page.PRs {
			if up.UpdatedAt.Before(cutoff) {
				if len(batch) > 0 {
					if err := st.UpsertPullRequests(ctx, repoID, batch); err != nil {
						return err
					}
					addPRSpan(span, batch)
				}
				break prLoop
			}
			batch = append(batch, up.PullRequest)
		}
		if err := st.UpsertPullRequests(ctx, repoID, batch); err != nil {
			return err
		}
		addPRSpan(span, batch)
		if !page.HasNextPage {
			break
		}
		after = page.EndCursor
	}

	// --- Issues updated (stop at cutoff) ---
	after = ""
issueLoop:
	for {
		page, err := client.FetchIssuesUpdated(ctx, owner, name, after)
		if err != nil {
			return err
		}
		var batch []store.Issue
		for _, ui := range page.Issues {
			if ui.UpdatedAt.Before(cutoff) {
				if len(batch) > 0 {
					if err := st.UpsertIssues(ctx, repoID, batch); err != nil {
						return err
					}
					addIssueSpan(span, batch)
				}
				break issueLoop
			}
			batch = append(batch, ui.Issue)
		}
		if err := st.UpsertIssues(ctx, repoID, batch); err != nil {
			return err
		}
		addIssueSpan(span, batch)
		if !page.HasNextPage {
			break
		}
		after = page.EndCursor
	}

	// Recompute aggregates over the touched span.
	if !span.min.IsZero() {
		from := span.min.UTC().Format("2006-01-02")
		to := span.max.UTC().Format("2006-01-02")
		if err := st.RecomputeDailyStats(ctx, repoID, from, to); err != nil {
			return err
		}
	}

	// Advance sync state.
	ss.LastCommitAt = &newest
	ss.Status = "complete"
	return st.UpsertSyncState(ctx, ss)
}

func addPRSpan(span *dateSpan, prs []store.PullRequest) {
	for _, p := range prs {
		span.add(p.CreatedAt)
		if p.MergedAt != nil {
			span.add(*p.MergedAt)
		}
		if p.ClosedAt != nil {
			span.add(*p.ClosedAt)
		}
	}
}

func addIssueSpan(span *dateSpan, issues []store.Issue) {
	for _, is := range issues {
		span.add(is.CreatedAt)
		if is.ClosedAt != nil {
			span.add(*is.ClosedAt)
		}
	}
}

// dateSpan tracks the min/max event dates touched during a delta so the
// aggregate recompute covers exactly the affected range.
type dateSpan struct {
	min, max time.Time
}

func (d *dateSpan) add(t time.Time) {
	if t.IsZero() {
		return
	}
	if d.min.IsZero() || t.Before(d.min) {
		d.min = t
	}
	if d.max.IsZero() || t.After(d.max) {
		d.max = t
	}
}

// splitFullName splits "owner/name" into its parts.
func splitFullName(fullName string) (owner, name string) {
	if i := strings.IndexByte(fullName, '/'); i >= 0 {
		return fullName[:i], fullName[i+1:]
	}
	return fullName, ""
}
