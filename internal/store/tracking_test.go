package store

import (
	"context"
	"testing"
)

func TestTrackUntrackRepo(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	uid := seedUser(t, s)
	repoID := seedRepo(t, s)

	if tracked, err := s.IsTracked(ctx, uid, repoID); err != nil || tracked {
		t.Fatalf("IsTracked before = %v err=%v, want false", tracked, err)
	}

	if err := s.TrackRepo(ctx, uid, repoID); err != nil {
		t.Fatal(err)
	}
	// Tracking twice is idempotent.
	if err := s.TrackRepo(ctx, uid, repoID); err != nil {
		t.Fatal(err)
	}

	if tracked, err := s.IsTracked(ctx, uid, repoID); err != nil || !tracked {
		t.Fatalf("IsTracked after track = %v err=%v, want true", tracked, err)
	}

	repos, err := s.ListTrackedRepos(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].ID != repoID {
		t.Fatalf("ListTrackedRepos = %+v", repos)
	}

	if err := s.UntrackRepo(ctx, uid, repoID); err != nil {
		t.Fatal(err)
	}
	if tracked, _ := s.IsTracked(ctx, uid, repoID); tracked {
		t.Fatal("expected untracked")
	}
	repos, _ = s.ListTrackedRepos(ctx, uid)
	if len(repos) != 0 {
		t.Fatalf("ListTrackedRepos after untrack = %+v", repos)
	}
}

func TestListTrackedReposIsPerUser(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	u1 := seedUser(t, s)
	u2, err := s.UpsertUser(ctx, &User{GitHubID: 2, Login: "second"})
	if err != nil {
		t.Fatal(err)
	}
	repoID := seedRepo(t, s)

	if err := s.TrackRepo(ctx, u1, repoID); err != nil {
		t.Fatal(err)
	}
	r1, _ := s.ListTrackedRepos(ctx, u1)
	r2, _ := s.ListTrackedRepos(ctx, u2)
	if len(r1) != 1 || len(r2) != 0 {
		t.Fatalf("per-user isolation broken: u1=%d u2=%d", len(r1), len(r2))
	}
}
