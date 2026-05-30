package store

import (
	"context"
	"testing"
	"time"
)

func seedUser(t *testing.T, s *Store) int64 {
	t.Helper()
	id, err := s.UpsertUser(context.Background(), &User{GitHubID: 1, Login: "u"})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSessionCreateGetDelete(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	uid := seedUser(t, s)

	sess, err := s.CreateSession(ctx, uid, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID == "" {
		t.Fatal("expected non-empty session id")
	}

	got, err := s.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != uid {
		t.Fatalf("UserID = %d, want %d", got.UserID, uid)
	}

	if err := s.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, sess.ID); err != ErrNotFound {
		t.Fatalf("after delete got %v, want ErrNotFound", err)
	}
}

func TestGetSessionExpired(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	uid := seedUser(t, s)

	sess, err := s.CreateSession(ctx, uid, -time.Minute) // already expired
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, sess.ID); err != ErrNotFound {
		t.Fatalf("expired session got %v, want ErrNotFound", err)
	}
}
