package api

import (
	"net/http"

	"github-stats/internal/auth"
)

// me handles GET /api/me, returning the authenticated user as JSON, including
// the granted OAuth scope and whether a PAT credential is configured.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	scopes := ""
	if c, err := s.store.GetCredential(r.Context(), u.ID, "oauth"); err == nil {
		scopes = c.Scopes
	}
	_, patErr := s.store.GetCredential(r.Context(), u.ID, "pat")
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         u.ID,
		"github_id":  u.GitHubID,
		"login":      u.Login,
		"avatar_url": u.AvatarURL,
		"scopes":     scopes,
		"has_pat":    patErr == nil,
	})
}
