package api

import (
	"encoding/json"
	"net/http"

	"github-stats/internal/auth"
)

// me handles GET /api/me, returning the authenticated user as JSON.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":         u.ID,
		"github_id":  u.GitHubID,
		"login":      u.Login,
		"avatar_url": u.AvatarURL,
	})
}
