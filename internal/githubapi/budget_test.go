package githubapi

import (
	"net/http"
	"testing"
	"time"
)

func TestBudgetUpdateFromRESTHeaders(t *testing.T) {
	b := NewBudget()
	reset := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	h := http.Header{}
	h.Set("X-RateLimit-Remaining", "4321")
	h.Set("X-RateLimit-Reset", "1775044800") // 2026-04-01T12:00:00Z unix
	b.UpdateFromRESTHeaders(h)

	rem, gotReset := b.REST()
	if rem != 4321 {
		t.Fatalf("REST remaining = %d, want 4321", rem)
	}
	if !gotReset.Equal(reset) {
		t.Fatalf("REST reset = %v, want %v", gotReset, reset)
	}
}

func TestBudgetUpdateFromGraphQL(t *testing.T) {
	b := NewBudget()
	resetAt := "2026-04-01T13:00:00Z"
	b.UpdateFromGraphQL(RateLimit{Cost: 1, Remaining: 4990, ResetAt: resetAt})

	rem, reset := b.GraphQL()
	if rem != 4990 {
		t.Fatalf("GraphQL remaining = %d, want 4990", rem)
	}
	want, _ := time.Parse(time.RFC3339, resetAt)
	if !reset.Equal(want) {
		t.Fatalf("GraphQL reset = %v, want %v", reset, want)
	}
}

func TestBudgetExhaustion(t *testing.T) {
	b := NewBudget()
	b.UpdateFromGraphQL(RateLimit{Remaining: 0, ResetAt: "2026-04-01T13:00:00Z"})
	if !b.GraphQLExhausted() {
		t.Fatal("expected GraphQL exhausted at 0 remaining")
	}
	b.UpdateFromGraphQL(RateLimit{Remaining: 10, ResetAt: "2026-04-01T13:00:00Z"})
	if b.GraphQLExhausted() {
		t.Fatal("expected not exhausted at 10 remaining")
	}
}

func TestBackoffForSecondaryLimit(t *testing.T) {
	b := NewBudget()
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	// Retry-After in seconds.
	h := http.Header{}
	h.Set("Retry-After", "30")
	d := b.BackoffFor(http.StatusForbidden, h, now)
	if d != 30*time.Second {
		t.Fatalf("Retry-After seconds backoff = %v, want 30s", d)
	}

	// Retry-After as HTTP date.
	h2 := http.Header{}
	h2.Set("Retry-After", now.Add(45*time.Second).UTC().Format(http.TimeFormat))
	d2 := b.BackoffFor(http.StatusTooManyRequests, h2, now)
	if d2 < 44*time.Second || d2 > 46*time.Second {
		t.Fatalf("Retry-After date backoff = %v, want ~45s", d2)
	}

	// 429 with no Retry-After falls back to a default minimum.
	d3 := b.BackoffFor(http.StatusTooManyRequests, http.Header{}, now)
	if d3 <= 0 {
		t.Fatalf("default backoff = %v, want > 0", d3)
	}

	// A normal 200 yields no backoff.
	if d4 := b.BackoffFor(http.StatusOK, http.Header{}, now); d4 != 0 {
		t.Fatalf("200 backoff = %v, want 0", d4)
	}
}
