package githubapi

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimit mirrors the GraphQL `rateLimit { cost remaining resetAt }` field.
type RateLimit struct {
	Cost      int    `json:"cost"`
	Remaining int    `json:"remaining"`
	ResetAt   string `json:"resetAt"` // RFC3339
}

// defaultBackoff is the minimum wait applied to a secondary-limit response that
// carries no Retry-After header.
const defaultBackoff = 60 * time.Second

// Budget tracks the REST and GraphQL rate-limit buckets separately. GitHub's
// REST (5,000 req/hr) and GraphQL (5,000 points/hr) limits are distinct pools
// (spec §3), so each is tracked on its own. Safe for concurrent use.
type Budget struct {
	mu            sync.Mutex
	restRemaining int
	restReset     time.Time
	gqlRemaining  int
	gqlReset      time.Time
}

// NewBudget returns a Budget with optimistic full buckets.
func NewBudget() *Budget {
	return &Budget{restRemaining: 5000, gqlRemaining: 5000}
}

// UpdateFromRESTHeaders ingests X-RateLimit-* headers from a REST response.
func (b *Budget) UpdateFromRESTHeaders(h http.Header) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if v := h.Get("X-RateLimit-Remaining"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			b.restRemaining = n
		}
	}
	if v := h.Get("X-RateLimit-Reset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			b.restReset = time.Unix(n, 0).UTC()
		}
	}
}

// UpdateFromGraphQL ingests a GraphQL rateLimit object.
func (b *Budget) UpdateFromGraphQL(rl RateLimit) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gqlRemaining = rl.Remaining
	if rl.ResetAt != "" {
		if t, err := time.Parse(time.RFC3339, rl.ResetAt); err == nil {
			b.gqlReset = t.UTC()
		}
	}
}

// REST returns the REST bucket's remaining count and reset time.
func (b *Budget) REST() (remaining int, reset time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.restRemaining, b.restReset
}

// GraphQL returns the GraphQL bucket's remaining points and reset time.
func (b *Budget) GraphQL() (remaining int, reset time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.gqlRemaining, b.gqlReset
}

// GraphQLExhausted reports whether the GraphQL bucket is empty.
func (b *Budget) GraphQLExhausted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.gqlRemaining <= 0
}

// RESTExhausted reports whether the REST bucket is empty.
func (b *Budget) RESTExhausted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.restRemaining <= 0
}

// BackoffFor returns how long to wait before retrying after a response. It
// returns 0 for non-limit statuses. For 403/429 it honours Retry-After (seconds
// or HTTP-date) and otherwise falls back to defaultBackoff.
func (b *Budget) BackoffFor(status int, h http.Header, now time.Time) time.Duration {
	if status != http.StatusForbidden && status != http.StatusTooManyRequests {
		return 0
	}
	if ra := h.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			return time.Duration(secs) * time.Second
		}
		if t, err := http.ParseTime(ra); err == nil {
			if d := t.Sub(now); d > 0 {
				return d
			}
		}
	}
	return defaultBackoff
}
