package store

import (
	"context"
	"testing"
)

func TestGetSyncStateDefaultsWhenAbsent(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)

	st, err := s.GetSyncState(ctx, repoID)
	if err != nil {
		t.Fatalf("GetSyncState should return zero-value state, not error: %v", err)
	}
	if st.RepoID != repoID {
		t.Fatalf("RepoID = %d, want %d", st.RepoID, repoID)
	}
	if st.Status != "" || st.LastPRCursor != "" || st.LastCommitAt != nil {
		t.Fatalf("expected empty state, got %+v", st)
	}
}

func TestUpsertSyncStateRoundTrip(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	repoID := seedRepo(t, s)

	last := ts("2026-02-01T00:00:00Z")
	in := &SyncState{
		RepoID:            repoID,
		LastCommitAt:      &last,
		LastCommitCursor:  "c1",
		LastPRCursor:      "p1",
		LastIssueCursor:   "i1",
		LastReleaseCursor: "r1",
		Status:            "backfilling",
	}
	if err := s.UpsertSyncState(ctx, in); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSyncState(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastPRCursor != "p1" || got.LastIssueCursor != "i1" ||
		got.LastCommitCursor != "c1" || got.LastReleaseCursor != "r1" ||
		got.Status != "backfilling" || got.LastCommitAt == nil ||
		!got.LastCommitAt.Equal(last) {
		t.Fatalf("round trip mismatch: %+v", got)
	}

	// Upsert again updates in place.
	in.Status = "complete"
	in.LastPRCursor = "p2"
	if err := s.UpsertSyncState(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetSyncState(ctx, repoID)
	if got.Status != "complete" || got.LastPRCursor != "p2" {
		t.Fatalf("update not applied: %+v", got)
	}
	var n int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sync_state WHERE repo_id = ?`, repoID).Scan(&n)
	if n != 1 {
		t.Fatalf("sync_state rows = %d, want 1", n)
	}
}
