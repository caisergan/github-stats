package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github-stats/internal/auth"
	"github-stats/internal/githubapi"
	"github-stats/internal/store"
)

// repoJSON is the wire shape for a tracked repo (M4/M5 depend on these keys).
type repoJSON struct {
	ID            int64   `json:"id"`
	FullName      string  `json:"full_name"`
	IsPrivate     bool    `json:"is_private"`
	DefaultBranch string  `json:"default_branch"`
	Description   string  `json:"description"`
	Stargazers    int64   `json:"stargazers"`
	Forks         int64   `json:"forks"`
	Language      string  `json:"language"`
	LanguageColor string  `json:"language_color"`
	SyncStatus    string  `json:"sync_status"`
	LastSyncedAt  *string `json:"last_synced_at"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// userClient mints a per-user GitHub client, preferring a stored fine-grained PAT
// over the OAuth token when present (an alternate credential — NOT a rate-limit bump,
// since GitHub's 5,000/hr bucket is shared across a user's OAuth + PATs; see spec §3).
func (s *Server) userClient(r *http.Request, userID int64) (*githubapi.Client, error) {
	cred, err := s.store.GetCredential(r.Context(), userID, "pat")
	if err == store.ErrNotFound {
		cred, err = s.store.GetCredential(r.Context(), userID, "oauth")
	}
	if err != nil {
		return nil, err
	}
	token, err := s.cipher.Decrypt(cred.EncToken)
	if err != nil {
		return nil, err
	}
	return githubapi.NewClient(githubapi.Options{
		Token:       string(token),
		GraphQLURL:  s.cfg.GitHubAPIBaseURL + "/graphql",
		RESTBaseURL: s.cfg.GitHubAPIBaseURL,
		Store:       s.store,
	}), nil
}

// addRepo handles POST /api/repos: fetch meta with the caller's token, upsert,
// track, and enqueue a backfill job.
func (s *Server) addRepo(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		FullName string `json:"full_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	owner, name := splitFullName(body.FullName)
	if owner == "" || name == "" {
		http.Error(w, "full_name must be owner/name", http.StatusBadRequest)
		return
	}

	client, err := s.userClient(r, u.ID)
	if err != nil {
		// A logged-in user without a usable OAuth credential is an unexpected
		// server-side state, not an upstream (GitHub) failure.
		http.Error(w, "no github credential", http.StatusInternalServerError)
		return
	}
	meta, err := client.FetchRepoMeta(r.Context(), owner, name)
	if err != nil {
		if repoInaccessible(err) {
			// GitHub returns "could not resolve" for both missing repos and private
			// repos a token can't see (it won't leak which). Give the user the fix.
			http.Error(w, "Couldn't access "+owner+"/"+name+". It doesn't exist, or "+
				`your GitHub token can't see it. Private repositories need the "repo" `+
				"scope — reconnect GitHub, or add a personal access token with the "+
				`"repo" scope in Settings.`, http.StatusNotFound)
			return
		}
		http.Error(w, "fetch repo failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	repoID, err := s.store.UpsertRepo(r.Context(), meta)
	if err != nil {
		http.Error(w, "persist repo failed", http.StatusInternalServerError)
		return
	}
	if err := s.store.TrackRepo(r.Context(), u.ID, repoID); err != nil {
		http.Error(w, "track failed", http.StatusInternalServerError)
		return
	}
	if _, err := s.engine.TriggerBackfill(r.Context(), repoID); err != nil {
		http.Error(w, "enqueue failed", http.StatusInternalServerError)
		return
	}

	ss, _ := s.store.GetSyncState(r.Context(), repoID)
	writeJSON(w, http.StatusCreated, toRepoJSON(meta, repoID, ss))
}

// listRepos handles GET /api/repos: the caller's tracked repos with sync status.
func (s *Server) listRepos(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	repos, err := s.store.ListTrackedRepos(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	out := make([]repoJSON, 0, len(repos))
	for i := range repos {
		ss, _ := s.store.GetSyncState(r.Context(), repos[i].ID)
		out = append(out, toRepoJSON(&repos[i], repos[i].ID, ss))
	}
	writeJSON(w, http.StatusOK, out)
}

// untrackRepo handles DELETE /api/repos/{id}.
func (s *Server) untrackRepo(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	repoID, err := repoIDParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := s.store.UntrackRepo(r.Context(), u.ID, repoID); err != nil {
		http.Error(w, "untrack failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// refreshRepo handles POST /api/repos/{id}/refresh: enqueue a delta job.
func (s *Server) refreshRepo(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	repoID, err := repoIDParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	tracked, err := s.store.IsTracked(r.Context(), u.ID, repoID)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	if !tracked {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if _, err := s.engine.TriggerDelta(r.Context(), repoID); err != nil {
		http.Error(w, "enqueue failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func repoIDParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func toRepoJSON(repo *store.Repo, repoID int64, ss *store.SyncState) repoJSON {
	j := repoJSON{
		ID:            repoID,
		FullName:      repo.FullName,
		IsPrivate:     repo.IsPrivate,
		DefaultBranch: repo.DefaultBranch,
		Description:   repo.Description,
		Stargazers:    repo.Stargazers,
		Forks:         repo.Forks,
		Language:      repo.PrimaryLanguage,
		LanguageColor: repo.LanguageColor,
	}
	if ss != nil {
		j.SyncStatus = ss.Status
		if ss.LastBackfillAt != nil {
			formatted := ss.LastBackfillAt.UTC().Format("2006-01-02T15:04:05Z07:00")
			j.LastSyncedAt = &formatted
		}
	}
	return j
}

// splitFullName splits "owner/name" into its parts. A name with no "/" yields
// (fullName, "").
func splitFullName(fullName string) (owner, name string) {
	owner, name, _ = strings.Cut(fullName, "/")
	return owner, name
}

// repoInaccessible reports whether a FetchRepoMeta error means GitHub couldn't
// resolve the repository for this token — i.e. it doesn't exist OR it's private
// and the token lacks access (GitHub deliberately returns the same signal for
// both). These are user-fixable (grant access), not upstream outages.
func repoInaccessible(err error) bool {
	s := err.Error()
	return strings.Contains(s, "Could not resolve to a Repository") ||
		strings.Contains(s, "empty data")
}
