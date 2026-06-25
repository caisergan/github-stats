package store

import (
	"context"
	"testing"
)

func TestCredentialUpsertGet(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	uid := seedUser(t, s)

	if err := s.UpsertCredential(ctx, &Credential{
		UserID: uid, Kind: "oauth", EncToken: "enc1", Scopes: "read:user",
	}); err != nil {
		t.Fatal(err)
	}

	// Upsert same (user, kind) replaces the token.
	if err := s.UpsertCredential(ctx, &Credential{
		UserID: uid, Kind: "oauth", EncToken: "enc2", Scopes: "repo",
	}); err != nil {
		t.Fatal(err)
	}

	c, err := s.GetCredential(ctx, uid, "oauth")
	if err != nil {
		t.Fatal(err)
	}
	if c.EncToken != "enc2" || c.Scopes != "repo" {
		t.Fatalf("upsert did not replace: %+v", c)
	}
}

func TestGetCredentialNotFound(t *testing.T) {
	s := openTemp(t)
	uid := seedUser(t, s)
	if _, err := s.GetCredential(context.Background(), uid, "pat"); err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestDeleteCredential(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	uid := seedUser(t, s)

	if err := s.UpsertCredential(ctx, &Credential{
		UserID: uid, Kind: "pat", EncToken: "enc", Scopes: "octocat",
	}); err != nil {
		t.Fatal(err)
	}
	// Unrelated credential of a different kind must survive the delete.
	if err := s.UpsertCredential(ctx, &Credential{
		UserID: uid, Kind: "oauth", EncToken: "enc-oauth", Scopes: "repo",
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteCredential(ctx, uid, "pat"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCredential(ctx, uid, "pat"); err != ErrNotFound {
		t.Fatalf("pat still present: %v", err)
	}
	if _, err := s.GetCredential(ctx, uid, "oauth"); err != nil {
		t.Fatalf("oauth credential wrongly removed: %v", err)
	}

	// Deleting a missing credential is a no-op (idempotent).
	if err := s.DeleteCredential(ctx, uid, "pat"); err != nil {
		t.Fatalf("delete of missing credential returned error: %v", err)
	}
}
