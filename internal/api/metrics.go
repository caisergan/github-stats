package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

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
	OpenIssues    int64           `json:"open_issues"`
	OpenPRs       int64           `json:"open_prs"`
	Contributors  int64           `json:"contributors"`
	CommitRate    float64         `json:"commit_rate"` // commits/day over the window
	IssueRate     float64         `json:"issue_rate"`  // issues opened/day over the window
	PRRate        float64         `json:"pr_rate"`     // PRs opened/day over the window
	Releases      int64           `json:"releases"`
	SyncStatus    string          `json:"sync_status"`
	LastSyncedAt  *string         `json:"last_synced_at"`
	WindowFrom    string          `json:"window_from"`
	WindowTo      string          `json:"window_to"`
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
		Language:      repo.PrimaryLanguage,
		LanguageColor: repo.LanguageColor,
		Languages:     rawLanguages(repo.Languages),
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

// commitJSON / prJSON / issueJSON are the wire shapes for the latest-items lists.
type commitJSON struct {
	SHA          string `json:"sha"`
	AuthorLogin  string `json:"author_login"`
	CommittedAt  string `json:"committed_at"`
	Additions    int64  `json:"additions"`
	Deletions    int64  `json:"deletions"`
	IsBot        bool   `json:"is_bot"`
	MsgFirstLine string `json:"msg_first_line"`
}

type prJSON struct {
	Number        int64   `json:"number"`
	AuthorLogin   string  `json:"author_login"`
	State         string  `json:"state"`
	CreatedAt     string  `json:"created_at"`
	MergedAt      *string `json:"merged_at"`
	ClosedAt      *string `json:"closed_at"`
	CommentsCount int64   `json:"comments_count"`
	IsBot         bool    `json:"is_bot"`
	Title         string  `json:"title"`
}

type issueJSON struct {
	Number        int64   `json:"number"`
	AuthorLogin   string  `json:"author_login"`
	State         string  `json:"state"`
	CreatedAt     string  `json:"created_at"`
	ClosedAt      *string `json:"closed_at"`
	CommentsCount int64   `json:"comments_count"`
	IsBot         bool    `json:"is_bot"`
	Title         string  `json:"title"`
}

const isoLayout = "2006-01-02T15:04:05Z07:00"

func fmtTime(t time.Time) string { return t.UTC().Format(isoLayout) }

func fmtTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(isoLayout)
	return &s
}

// parseLimit reads ?limit= (default 20, min 1, max 100).
func parseLimit(raw string) int {
	const def, max = 20, 100
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

// repoLatest handles GET /api/repos/{id}/latest/{commits|prs|issues}?limit=.
func (s *Server) repoLatest(w http.ResponseWriter, r *http.Request) {
	_, repoID, ok := s.requireTracked(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	limit := parseLimit(r.URL.Query().Get("limit"))

	switch chi.URLParam(r, "kind") {
	case "commits":
		rows, err := s.store.LatestCommits(ctx, repoID, limit)
		if err != nil {
			http.Error(w, "load failed", http.StatusInternalServerError)
			return
		}
		out := make([]commitJSON, 0, len(rows))
		for _, c := range rows {
			out = append(out, commitJSON{
				SHA: c.SHA, AuthorLogin: c.AuthorLogin, CommittedAt: fmtTime(c.CommittedAt),
				Additions: c.Additions, Deletions: c.Deletions, IsBot: c.IsBot, MsgFirstLine: c.MsgFirstLine,
			})
		}
		writeJSON(w, http.StatusOK, out)
	case "prs":
		rows, err := s.store.LatestPRs(ctx, repoID, limit)
		if err != nil {
			http.Error(w, "load failed", http.StatusInternalServerError)
			return
		}
		out := make([]prJSON, 0, len(rows))
		for _, p := range rows {
			out = append(out, prJSON{
				Number: p.Number, AuthorLogin: p.AuthorLogin, State: p.State, CreatedAt: fmtTime(p.CreatedAt),
				MergedAt: fmtTimePtr(p.MergedAt), ClosedAt: fmtTimePtr(p.ClosedAt),
				CommentsCount: p.CommentsCount, IsBot: p.IsBot, Title: p.Title,
			})
		}
		writeJSON(w, http.StatusOK, out)
	case "issues":
		rows, err := s.store.LatestIssues(ctx, repoID, limit)
		if err != nil {
			http.Error(w, "load failed", http.StatusInternalServerError)
			return
		}
		out := make([]issueJSON, 0, len(rows))
		for _, is := range rows {
			out = append(out, issueJSON{
				Number: is.Number, AuthorLogin: is.AuthorLogin, State: is.State, CreatedAt: fmtTime(is.CreatedAt),
				ClosedAt: fmtTimePtr(is.ClosedAt), CommentsCount: is.CommentsCount, IsBot: is.IsBot, Title: is.Title,
			})
		}
		writeJSON(w, http.StatusOK, out)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown kind"})
	}
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
		if errors.Is(err, metrics.ErrUnknownMetric) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "compute failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
