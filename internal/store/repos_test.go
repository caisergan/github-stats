package store

import (
	"context"
	"testing"
)

func TestNewTablesExist(t *testing.T) {
	s := openTemp(t)
	for _, table := range []string{
		"repos", "commits", "pull_requests", "issues", "releases",
		"daily_repo_stats", "daily_contributor_stats", "sync_state", "etags",
	} {
		var name string
		err := s.DB.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestUpsertRepoInsertsThenUpdates(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	id, err := s.UpsertRepo(ctx, &Repo{
		GitHubID: 100, FullName: "octocat/hello", IsPrivate: true,
		DefaultBranch: "main", Description: "first", Stargazers: 3, Forks: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	id2, err := s.UpsertRepo(ctx, &Repo{
		GitHubID: 100, FullName: "octocat/hello", IsPrivate: false,
		DefaultBranch: "trunk", Description: "second", Stargazers: 9, Forks: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id {
		t.Fatalf("upsert created new row: %d != %d", id2, id)
	}

	r, err := s.GetRepo(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if r.DefaultBranch != "trunk" || r.Description != "second" || r.Stargazers != 9 || r.IsPrivate {
		t.Fatalf("update not applied: %+v", r)
	}
}

func TestGetRepoByFullName(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	id, _ := s.UpsertRepo(ctx, &Repo{GitHubID: 7, FullName: "a/b", DefaultBranch: "main"})

	r, err := s.GetRepoByFullName(ctx, "a/b")
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != id {
		t.Fatalf("ID = %d, want %d", r.ID, id)
	}
	if _, err := s.GetRepoByFullName(ctx, "missing/repo"); err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestGetRepoNotFound(t *testing.T) {
	s := openTemp(t)
	if _, err := s.GetRepo(context.Background(), 999); err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
