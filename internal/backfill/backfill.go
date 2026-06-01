// Package backfill performs a one-time, resumable, single-repo backfill of
// commits, pull requests, issues and releases into the store, recomputing daily
// aggregates over the touched date span. It orchestrates only: fetching is
// delegated to githubapi and persistence to store (spec §4/§6).
package backfill

import (
	"context"
	"strings"
	"time"

	"github-stats/internal/datespan"
	"github-stats/internal/githubapi"
	"github-stats/internal/store"
)

// Run performs the full backfill for repoID. It is resumable: cursors are saved
// to sync_state after every page, so a re-run after interruption continues from
// the last saved cursor rather than re-fetching from the start.
func Run(ctx context.Context, st *store.Store, client *githubapi.Client, repoID int64) error {
	repo, err := st.GetRepo(ctx, repoID)
	if err != nil {
		return err
	}
	owner, name := splitFullName(repo.FullName)

	ss, err := st.GetSyncState(ctx, repoID)
	if err != nil {
		return err
	}
	ss.Status = "backfilling"
	if err := st.UpsertSyncState(ctx, ss); err != nil {
		return err
	}

	// Refresh repo metadata (and pick up the real default branch).
	meta, err := client.FetchRepoMeta(ctx, owner, name)
	if err != nil {
		return err
	}
	meta.ID = repo.ID
	if _, err := st.UpsertRepo(ctx, meta); err != nil {
		return err
	}
	branch := meta.DefaultBranch
	if branch == "" {
		branch = repo.DefaultBranch
	}

	span := &datespan.Span{}

	if err := backfillCommits(ctx, st, client, repoID, owner, name, branch, ss, span); err != nil {
		return err
	}
	if err := backfillPRs(ctx, st, client, repoID, owner, name, ss, span); err != nil {
		return err
	}
	if err := backfillIssues(ctx, st, client, repoID, owner, name, ss, span); err != nil {
		return err
	}
	if err := backfillReleases(ctx, st, client, repoID, owner, name, ss, span); err != nil {
		return err
	}

	// Recompute aggregates over the full touched span (no-op if nothing touched).
	if !span.Empty() {
		from, to := span.Range()
		if err := st.RecomputeDailyStats(ctx, repoID, from, to); err != nil {
			return err
		}
	}

	now := time.Now().UTC()
	ss.Status = "complete"
	ss.LastBackfillAt = &now
	return st.UpsertSyncState(ctx, ss)
}

func backfillCommits(ctx context.Context, st *store.Store, client *githubapi.Client,
	repoID int64, owner, name, branch string, ss *store.SyncState, span *datespan.Span) error {
	after := ss.LastCommitCursor
	for {
		page, err := client.FetchCommits(ctx, owner, name, branch, after)
		if err != nil {
			return err
		}
		if err := st.UpsertCommits(ctx, repoID, page.Commits); err != nil {
			return err
		}
		for _, c := range page.Commits {
			span.Add(c.CommittedAt)
			if ss.LastCommitAt == nil || c.CommittedAt.After(*ss.LastCommitAt) {
				t := c.CommittedAt
				ss.LastCommitAt = &t
			}
		}
		ss.LastCommitCursor = page.EndCursor
		if err := st.UpsertSyncState(ctx, ss); err != nil { // resumable: save cursor each page
			return err
		}
		if !page.HasNextPage {
			return nil
		}
		after = page.EndCursor
	}
}

func backfillPRs(ctx context.Context, st *store.Store, client *githubapi.Client,
	repoID int64, owner, name string, ss *store.SyncState, span *datespan.Span) error {
	after := ss.LastPRCursor
	for {
		page, err := client.FetchPullRequests(ctx, owner, name, after)
		if err != nil {
			return err
		}
		if err := st.UpsertPullRequests(ctx, repoID, page.PRs); err != nil {
			return err
		}
		for _, p := range page.PRs {
			span.Add(p.CreatedAt)
			if p.MergedAt != nil {
				span.Add(*p.MergedAt)
			}
			if p.ClosedAt != nil {
				span.Add(*p.ClosedAt)
			}
		}
		ss.LastPRCursor = page.EndCursor
		if err := st.UpsertSyncState(ctx, ss); err != nil {
			return err
		}
		if !page.HasNextPage {
			return nil
		}
		after = page.EndCursor
	}
}

func backfillIssues(ctx context.Context, st *store.Store, client *githubapi.Client,
	repoID int64, owner, name string, ss *store.SyncState, span *datespan.Span) error {
	after := ss.LastIssueCursor
	for {
		page, err := client.FetchIssues(ctx, owner, name, after)
		if err != nil {
			return err
		}
		if err := st.UpsertIssues(ctx, repoID, page.Issues); err != nil {
			return err
		}
		for _, is := range page.Issues {
			span.Add(is.CreatedAt)
			if is.ClosedAt != nil {
				span.Add(*is.ClosedAt)
			}
		}
		ss.LastIssueCursor = page.EndCursor
		if err := st.UpsertSyncState(ctx, ss); err != nil {
			return err
		}
		if !page.HasNextPage {
			return nil
		}
		after = page.EndCursor
	}
}

func backfillReleases(ctx context.Context, st *store.Store, client *githubapi.Client,
	repoID int64, owner, name string, ss *store.SyncState, span *datespan.Span) error {
	after := ss.LastReleaseCursor
	for {
		page, err := client.FetchReleases(ctx, owner, name, after)
		if err != nil {
			return err
		}
		if err := st.UpsertReleases(ctx, repoID, page.Releases); err != nil {
			return err
		}
		for _, rel := range page.Releases {
			if rel.PublishedAt != nil {
				span.Add(*rel.PublishedAt)
			}
		}
		ss.LastReleaseCursor = page.EndCursor
		if err := st.UpsertSyncState(ctx, ss); err != nil {
			return err
		}
		if !page.HasNextPage {
			return nil
		}
		after = page.EndCursor
	}
}

// splitFullName splits "owner/name" into its parts. A name with no "/" yields
// (fullName, "").
func splitFullName(fullName string) (owner, name string) {
	owner, name, _ = strings.Cut(fullName, "/")
	return owner, name
}
