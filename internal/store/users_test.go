package store

import (
	"context"
	"testing"
)

func TestUpsertUserInsertsThenUpdates(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	id, err := s.UpsertUser(ctx, &User{GitHubID: 42, Login: "octocat", AvatarURL: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	// Same github_id with changed login should update, not duplicate.
	id2, err := s.UpsertUser(ctx, &User{GitHubID: 42, Login: "octocat-renamed", AvatarURL: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id {
		t.Fatalf("upsert created new row: %d != %d", id2, id)
	}

	u, err := s.GetUserByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if u.Login != "octocat-renamed" || u.AvatarURL != "b" {
		t.Fatalf("update not applied: %+v", u)
	}
}

func TestGetUserByIDNotFound(t *testing.T) {
	s := openTemp(t)
	_, err := s.GetUserByID(context.Background(), 999)
	if err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
