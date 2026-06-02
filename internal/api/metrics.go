package api

import (
	"net/http"
	"strings"

	"github-stats/internal/auth"
	"github-stats/internal/metrics"
)

// requireTracked resolves the {id} URL param, confirms the caller tracks the repo,
// and returns (userID, repoID, ok). It writes the appropriate error response and
// returns ok=false on any failure (401 unauthenticated, 400 bad id, 404 untracked).
func (s *Server) requireTracked(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return 0, 0, false
	}
	repoID, err := repoIDParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return 0, 0, false
	}
	tracked, err := s.store.IsTracked(r.Context(), u.ID, repoID)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return 0, 0, false
	}
	if !tracked {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return 0, 0, false
	}
	return u.ID, repoID, true
}

// parseKeys splits a comma-separated keys parameter, trimming blanks. Empty → nil
// (registry computes all keys).
func parseKeys(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// repoMetrics handles GET /api/repos/{id}/metrics?keys=&window=&exclude_bots=.
func (s *Server) repoMetrics(w http.ResponseWriter, r *http.Request) {
	_, repoID, ok := s.requireTracked(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	keys := parseKeys(q.Get("keys"))
	opts := metrics.Opts{ExcludeBots: q.Get("exclude_bots") == "true"}

	win, err := metrics.ParseWindow(r.Context(), q.Get("window"), repoID, s.store, s.now)
	if err != nil {
		http.Error(w, "bad window: "+err.Error(), http.StatusBadRequest)
		return
	}
	out, err := s.registry.Compute(r.Context(), s.store, repoID, keys, win, opts)
	if err != nil {
		// Unknown metric key → 400; anything else is a 500.
		if strings.HasPrefix(err.Error(), "unknown metric") {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "compute failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
