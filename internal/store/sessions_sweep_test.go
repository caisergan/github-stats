package store

import (
	"context"
	"testing"
	"time"
)

func TestDeleteSessionsForUser(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	uid := seedUser(t, s)

	a, _ := s.CreateSession(ctx, uid, time.Hour)
	b, _ := s.CreateSession(ctx, uid, time.Hour)

	n, err := s.DeleteSessionsForUser(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("deleted = %d, want 2", n)
	}
	for _, id := range []string{a.ID, b.ID} {
		if _, err := s.GetSession(ctx, id); err != ErrNotFound {
			t.Fatalf("session %s survived: %v", id, err)
		}
	}
}

func TestDeleteExpiredSessions(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	uid := seedUser(t, s)

	live, _ := s.CreateSession(ctx, uid, time.Hour)
	dead, _ := s.CreateSession(ctx, uid, -time.Hour) // already expired

	now := time.Now().UTC()
	n, err := s.DeleteExpiredSessions(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("swept = %d, want 1", n)
	}
	// The live one is untouched; the dead one is gone.
	if _, err := s.GetSession(ctx, live.ID); err != nil {
		t.Fatalf("live session removed: %v", err)
	}
	var c int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, dead.ID).Scan(&c)
	if c != 0 {
		t.Fatalf("expired session not swept (count=%d)", c)
	}
}
