package store

import (
	"context"
	"testing"
)

// seedNamedRepo inserts a minimal repo and returns its id (collections reference repos).
// Named to avoid colliding with the existing seedRepo(t, s) helper in events_test.go.
func seedNamedRepo(t *testing.T, s *Store, fullName string, githubID int64) int64 {
	t.Helper()
	id, err := s.UpsertRepo(context.Background(), &Repo{
		GitHubID: githubID, FullName: fullName, DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCollectionCRUD(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	uid := seedUser(t, s)

	id, err := s.CreateCollection(ctx, uid, "Backend")
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero collection id")
	}

	cols, err := s.ListCollections(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 1 || cols[0].Name != "Backend" || cols[0].UserID != uid {
		t.Fatalf("ListCollections = %+v", cols)
	}

	if err := s.RenameCollection(ctx, uid, id, "Services"); err != nil {
		t.Fatal(err)
	}
	cols, _ = s.ListCollections(ctx, uid)
	if cols[0].Name != "Services" {
		t.Fatalf("rename not applied: %+v", cols[0])
	}

	if err := s.DeleteCollection(ctx, uid, id); err != nil {
		t.Fatal(err)
	}
	cols, _ = s.ListCollections(ctx, uid)
	if len(cols) != 0 {
		t.Fatalf("expected 0 collections after delete, got %d", len(cols))
	}
}

func TestCollectionOwnershipIsolation(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	owner := seedUser(t, s)
	other, _ := s.UpsertUser(ctx, &User{GitHubID: 2, Login: "other"})

	id, _ := s.CreateCollection(ctx, owner, "Mine")

	// Another user cannot rename or delete someone else's collection.
	if err := s.RenameCollection(ctx, other, id, "Hijacked"); err != ErrNotFound {
		t.Fatalf("rename by non-owner = %v, want ErrNotFound", err)
	}
	if err := s.DeleteCollection(ctx, other, id); err != ErrNotFound {
		t.Fatalf("delete by non-owner = %v, want ErrNotFound", err)
	}
	// And it does not appear in their list.
	cols, _ := s.ListCollections(ctx, other)
	if len(cols) != 0 {
		t.Fatalf("other user sees %d collections, want 0", len(cols))
	}
}

func TestCollectionRepoMembership(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	uid := seedUser(t, s)
	cid, _ := s.CreateCollection(ctx, uid, "Web")
	r1 := seedNamedRepo(t, s, "octo/a", 10)
	r2 := seedNamedRepo(t, s, "octo/b", 11)

	if err := s.AddRepoToCollection(ctx, uid, cid, r1); err != nil {
		t.Fatal(err)
	}
	if err := s.AddRepoToCollection(ctx, uid, cid, r2); err != nil {
		t.Fatal(err)
	}
	// Idempotent re-add.
	if err := s.AddRepoToCollection(ctx, uid, cid, r1); err != nil {
		t.Fatal(err)
	}

	repos, err := s.ListCollectionRepos(ctx, uid, cid)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("ListCollectionRepos = %d rows, want 2", len(repos))
	}

	if err := s.RemoveRepoFromCollection(ctx, uid, cid, r1); err != nil {
		t.Fatal(err)
	}
	repos, _ = s.ListCollectionRepos(ctx, uid, cid)
	if len(repos) != 1 || repos[0].FullName != "octo/b" {
		t.Fatalf("after remove = %+v", repos)
	}
}

func TestAddRepoToForeignCollectionRejected(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	owner := seedUser(t, s)
	other, _ := s.UpsertUser(ctx, &User{GitHubID: 2, Login: "other"})
	cid, _ := s.CreateCollection(ctx, owner, "Mine")
	rid := seedNamedRepo(t, s, "octo/a", 10)

	if err := s.AddRepoToCollection(ctx, other, cid, rid); err != ErrNotFound {
		t.Fatalf("add to foreign collection = %v, want ErrNotFound", err)
	}
}
