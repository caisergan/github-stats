package api

import (
	"net/http"

	"github-stats/internal/auth"
)

// csrfToken handles GET /api/csrf: issues a CSRF token (cookie + body) for the SPA.
func (s *Server) csrfToken(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFromContext(r.Context()); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	token := s.auth.IssueCSRF(w)
	writeJSON(w, http.StatusOK, map[string]string{"csrf_token": token})
}
