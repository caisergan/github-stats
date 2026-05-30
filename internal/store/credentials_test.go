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
