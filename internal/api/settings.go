package api

import (
	"encoding/json"
	"net/http"

	"github-stats/internal/auth"
	"github-stats/internal/githubapi"
	"github-stats/internal/store"
)

// patStatus is the GET /api/settings/pat response.
type patStatus struct {
	HasPAT bool   `json:"has_pat"`
	Login  string `json:"login,omitempty"`
}

// getPATStatus handles GET /api/settings/pat.
func (s *Server) getPATStatus(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	cred, err := s.store.GetCredential(r.Context(), u.ID, "pat")
	if err == store.ErrNotFound {
		writeJSON(w, http.StatusOK, patStatus{HasPAT: false})
		return
	}
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	// "scopes" doubles as a stash for the validated login (set on save).
	writeJSON(w, http.StatusOK, patStatus{HasPAT: true, Login: cred.Scopes})
}

// savePAT handles PUT /api/settings/pat: validate the token then store it encrypted.
func (s *Server) savePAT(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
		http.Error(w, "token is required", http.StatusBadRequest)
		return
	}
	info, err := githubapi.ValidatePAT(r.Context(), http.DefaultClient, s.cfg.GitHubAPIBaseURL, body.Token)
	if err != nil {
		http.Error(w, "invalid token: "+err.Error(), http.StatusBadRequest)
		return
	}
	// This PAT is the read-only path to PRIVATE repos (the OAuth login is read-only
	// and public-only). The safest choice is a fine-grained token with read-only
	// access — those report no classic scopes (info.Scopes == ""), so we accept
	// them. A classic token reports its scopes; if it lacks "repo" it can't read
	// private repos at all, so reject it here rather than fail cryptically on track.
	if info.Scopes != "" && !githubapi.HasRepoScope(info.Scopes) {
		http.Error(w, "this token can't read private repositories. Use a fine-grained "+
			`personal access token with read-only access (Contents: Read) to the repos `+
			`you want to track, or a classic token with the "repo" scope.`,
			http.StatusBadRequest)
		return
	}
	enc, err := s.cipher.Encrypt([]byte(body.Token))
	if err != nil {
		http.Error(w, "encrypt failed", http.StatusInternalServerError)
		return
	}
	if err := s.store.UpsertCredential(r.Context(), &store.Credential{
		UserID: u.ID, Kind: "pat", EncToken: enc, Scopes: info.Login,
	}); err != nil {
		http.Error(w, "store failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, patStatus{HasPAT: true, Login: info.Login})
}

// deletePAT handles DELETE /api/settings/pat.
func (s *Server) deletePAT(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := s.store.DeleteCredential(r.Context(), u.ID, "pat"); err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
