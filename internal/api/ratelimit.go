package api

import (
	"net/http"
	"time"

	"github-stats/internal/auth"
)

type budgetBucket struct {
	Remaining int    `json:"remaining"`
	Reset     string `json:"reset"` // RFC3339; empty if unknown
}

type rateLimitJSON struct {
	REST    budgetBucket `json:"rest"`
	GraphQL budgetBucket `json:"graphql"`
}

func fmtReset(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// rateLimit handles GET /api/rate-limit — the engine's current GitHub budget.
func (s *Server) rateLimit(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFromContext(r.Context()); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	b := s.engine.Budget()
	restRem, restReset := b.REST()
	gqlRem, gqlReset := b.GraphQL()
	writeJSON(w, http.StatusOK, rateLimitJSON{
		REST:    budgetBucket{Remaining: restRem, Reset: fmtReset(restReset)},
		GraphQL: budgetBucket{Remaining: gqlRem, Reset: fmtReset(gqlReset)},
	})
}
