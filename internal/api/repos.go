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
	ID            int64           `json:"id"`
	FullName      string          `json:"full_name"`
	IsPrivate     bool            `json:"is_private"`
	DefaultBranch string          `json:"default_branch"`
	Description   string          `json:"description"`
	Stargazers    int64           `json:"stargazers"`
	Forks         int64           `json:"forks"`
	Language      string          `json:"language"`
	LanguageColor string          `json:"language_color"`
	Languages     json.RawMessage `json:"languages"`
	SyncStatus    string          `json:"sync_status"`
	LastSyncedAt  *string         `json:"last_synced_at"`
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
				"your token can't see it. The GitHub login is read-only and covers "+
				"public repositories; to track a private repository, add a fine-grained "+
				"personal access token with read-only access to it (Contents: Read) in "+
				"Settings.", http.StatusNotFound)
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

// untrackRepo handles DELETE /api/repos/{id}: stops tracking for the caller and,
// once no user tracks the repo anymore, hard-deletes all of its stored data.
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
	// Once no user tracks the repo, hard-delete all of its stored data (commits,
	// PRs, issues, stats, sync state, jobs, cache). This only ever touches the
	// local DB — never GitHub.
	n, err := s.store.CountTrackers(r.Context(), repoID)
	if err != nil {
		http.Error(w, "untrack failed", http.StatusInternalServerError)
		return
	}
	if n == 0 {
		if err := s.store.PurgeRepo(r.Context(), repoID); err != nil {
			http.Error(w, "purge failed", http.StatusInternalServerError)
			return
		}
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

// loadAllCommits handles POST /api/repos/{id}/load-all-commits: enqueue a
// backfill to ingest the repo's FULL commit history. The backfill is resumable
// (persisted cursor) and quota-aware — if GitHub's API quota is exhausted the
// job is rescheduled to the reset time and auto-resumes, so a too-big repo
// finishes across multiple windows without losing progress. If a sync job is
// already open for the repo it's a no-op (the open job already covers it).
func (s *Server) loadAllCommits(w http.ResponseWriter, r *http.Request) {
	_, repoID, ok := s.requireTracked(w, r)
	if !ok {
		return
	}
	open, err := s.store.HasOpenJob(r.Context(), repoID)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	if !open {
		if _, err := s.engine.TriggerBackfill(r.Context(), repoID); err != nil {
			http.Error(w, "enqueue failed", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

// syncStatusJSON reports a repo's most relevant sync job so the UI can show
// "loading…" / "waiting for quota until X" even after a reload (the SSE stream
// only carries live events). Active is true while pending or running.
type syncStatusJSON struct {
	Kind      string  `json:"kind"`
	Status    string  `json:"status"`
	NextRunAt *string `json:"next_run_at"`
	Attempts  int     `json:"attempts"`
	LastError string  `json:"last_error"`
	Active    bool    `json:"active"`
}

// repoSyncStatus handles GET /api/repos/{id}/sync/status: the newest open
// (pending/running) job, else the newest job overall, else an empty object.
func (s *Server) repoSyncStatus(w http.ResponseWriter, r *http.Request) {
	_, repoID, ok := s.requireTracked(w, r)
	if !ok {
		return
	}
	jobs, err := s.store.ListJobsForRepo(r.Context(), repoID)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	if len(jobs) == 0 {
		writeJSON(w, http.StatusOK, syncStatusJSON{})
		return
	}
	job := jobs[0] // newest (ListJobsForRepo is id DESC)
	for _, j := range jobs {
		if j.Status == "pending" || j.Status == "running" {
			job = j
			break
		}
	}
	nr := job.NextRunAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	writeJSON(w, http.StatusOK, syncStatusJSON{
		Kind:      job.Kind,
		Status:    job.Status,
		NextRunAt: &nr,
		Attempts:  job.Attempts,
		LastError: job.LastError,
		Active:    job.Status == "pending" || job.Status == "running",
	})
}

func repoIDParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

// rawLanguages passes the stored languages JSON through as-is, defaulting an
// empty value to an empty array so the response stays valid JSON.
func rawLanguages(s string) json.RawMessage {
	if s == "" {
		return json.RawMessage("[]")
	}
	return json.RawMessage(s)
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
		Languages:     rawLanguages(repo.Languages),
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
