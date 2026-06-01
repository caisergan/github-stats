package sync

import (
	"context"
	"strings"
	"time"

	"github-stats/internal/datespan"
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

	nowVal := now()

	// Commits page from the last recorded commit time (commit-cursor semantics).
	var since time.Time
	if ss.LastCommitAt != nil {
		since = *ss.LastCommitAt
	} else {
		since = nowVal.Add(-freshLookback)
	}

	// PR/issue paging stops at a cutoff derived from the freshest *sync* marker
	// (not just the last commit) minus an overlap window. Keying purely off
	// LastCommitAt would re-scan the whole updated-history every run on repos
	// whose commits are infrequent; using the last sync time bounds the rescan.
	lastSync := since
	if ss.LastBackfillAt != nil && ss.LastBackfillAt.After(lastSync) {
		lastSync = *ss.LastBackfillAt
	}
	if ss.LastDeltaAt != nil && ss.LastDeltaAt.After(lastSync) {
		lastSync = *ss.LastDeltaAt
	}
	cutoff := lastSync.Add(-overlapWindow)

	span := &datespan.Span{}
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
			span.Add(c.CommittedAt)
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
	if !span.Empty() {
		from, to := span.Range()
		if err := st.RecomputeDailyStats(ctx, repoID, from, to); err != nil {
			return err
		}
	}

	// Advance sync state. LastDeltaAt records this run so the scheduler can
	// throttle delta cadence off it (LastBackfillAt is stamped only once).
	ss.LastCommitAt = &newest
	ss.LastDeltaAt = &nowVal
	ss.Status = "complete"
	return st.UpsertSyncState(ctx, ss)
}

func addPRSpan(span *datespan.Span, prs []store.PullRequest) {
	for _, p := range prs {
		span.Add(p.CreatedAt)
		if p.MergedAt != nil {
			span.Add(*p.MergedAt)
		}
		if p.ClosedAt != nil {
			span.Add(*p.ClosedAt)
		}
	}
}

func addIssueSpan(span *datespan.Span, issues []store.Issue) {
	for _, is := range issues {
		span.Add(is.CreatedAt)
		if is.ClosedAt != nil {
			span.Add(*is.ClosedAt)
		}
	}
}

// splitFullName splits "owner/name" into its parts. A name with no "/" yields
// (fullName, "").
func splitFullName(fullName string) (owner, name string) {
	owner, name, _ = strings.Cut(fullName, "/")
	return owner, name
}
