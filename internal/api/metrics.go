package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github-stats/internal/auth"
	"github-stats/internal/metrics"
	"github-stats/internal/store"
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

// overviewJSON is the repo-card / details bundle (spec §8). M5 renders these.
type overviewJSON struct {
	ID            int64   `json:"id"`
	FullName      string  `json:"full_name"`
	IsPrivate     bool    `json:"is_private"`
	DefaultBranch string  `json:"default_branch"`
	Description   string  `json:"description"`
	Stargazers    int64   `json:"stargazers"`
	Forks         int64   `json:"forks"`
	OpenIssues    int64   `json:"open_issues"`
	OpenPRs       int64   `json:"open_prs"`
	Contributors  int64   `json:"contributors"`
	CommitRate    float64 `json:"commit_rate"` // commits/day over the window
	IssueRate     float64 `json:"issue_rate"`  // issues opened/day over the window
	PRRate        float64 `json:"pr_rate"`     // PRs opened/day over the window
	Releases      int64   `json:"releases"`
	SyncStatus    string  `json:"sync_status"`
	LastSyncedAt  *string `json:"last_synced_at"`
	WindowFrom    string  `json:"window_from"`
	WindowTo      string  `json:"window_to"`
}

// repoOverview handles GET /api/repos/{id}: the repo metadata + headline numbers.
func (s *Server) repoOverview(w http.ResponseWriter, r *http.Request) {
	_, repoID, ok := s.requireTracked(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	q := r.URL.Query()
	excludeBots := q.Get("exclude_bots") == "true"

	win, err := metrics.ParseWindow(ctx, q.Get("window"), repoID, s.store, s.now)
	if err != nil {
		http.Error(w, "bad window: "+err.Error(), http.StatusBadRequest)
		return
	}
	repo, err := s.store.GetRepo(ctx, repoID)
	if err != nil {
		http.Error(w, "repo lookup failed", http.StatusInternalServerError)
		return
	}
	asOf, err := win.ToTime()
	if err != nil {
		http.Error(w, "bad window", http.StatusInternalServerError)
		return
	}

	ov, err := s.buildOverview(ctx, repo, repoID, win, asOf, excludeBots)
	if err != nil {
		http.Error(w, "overview failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ov)
}

// buildOverview composes the overview bundle from store reads + the window.
func (s *Server) buildOverview(ctx context.Context, repo *store.Repo, repoID int64, win metrics.Window, asOf time.Time, excludeBots bool) (overviewJSON, error) {
	openIssues, err := s.store.CountOpenIssues(ctx, repoID, asOf, excludeBots)
	if err != nil {
		return overviewJSON{}, err
	}
	openPRs, err := s.store.CountOpenPRs(ctx, repoID, asOf, excludeBots)
	if err != nil {
		return overviewJSON{}, err
	}
	contributors, err := s.store.CountContributors(ctx, repoID, win.From, win.To, excludeBots)
	if err != nil {
		return overviewJSON{}, err
	}
	releases, err := s.store.CountReleases(ctx, repoID)
	if err != nil {
		return overviewJSON{}, err
	}
	daily, err := s.store.DailyRepoStats(ctx, repoID, win.From, win.To)
	if err != nil {
		return overviewJSON{}, err
	}
	dates, err := win.Dates()
	if err != nil {
		return overviewJSON{}, err
	}
	days := float64(len(dates))
	if days == 0 {
		days = 1
	}
	var commits, issuesOpened, prsOpened int64
	for _, d := range daily {
		commits += d.Commits
		issuesOpened += d.IssuesOpened
		prsOpened += d.PRsOpened
	}

	ov := overviewJSON{
		ID:            repoID,
		FullName:      repo.FullName,
		IsPrivate:     repo.IsPrivate,
		DefaultBranch: repo.DefaultBranch,
		Description:   repo.Description,
		Stargazers:    repo.Stargazers,
		Forks:         repo.Forks,
		OpenIssues:    openIssues,
		OpenPRs:       openPRs,
		Contributors:  contributors,
		CommitRate:    float64(commits) / days,
		IssueRate:     float64(issuesOpened) / days,
		PRRate:        float64(prsOpened) / days,
		Releases:      releases,
		WindowFrom:    win.From,
		WindowTo:      win.To,
	}
	if ss, err := s.store.GetSyncState(ctx, repoID); err == nil && ss != nil {
		ov.SyncStatus = ss.Status
		if ss.LastBackfillAt != nil {
			formatted := ss.LastBackfillAt.UTC().Format("2006-01-02T15:04:05Z07:00")
			ov.LastSyncedAt = &formatted
		}
	}
	return ov, nil
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
