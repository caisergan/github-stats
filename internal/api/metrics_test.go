package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github-stats/internal/store"
)

// seedMetricsRepo creates a tracked repo for user 1 with a small event set and
// recomputed aggregates, and pins the server clock for deterministic windows.
func seedMetricsRepo(t *testing.T, srv *Server, st *store.Store) int64 {
	t.Helper()
	ctx := context.Background()
	uid, _ := st.UpsertUser(ctx, &store.User{GitHubID: 1, Login: "neo"})
	if uid != 1 {
		t.Fatalf("expected user id 1, got %d", uid)
	}
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 10, FullName: "a/b", DefaultBranch: "main"})
	if err := st.TrackRepo(ctx, uid, repoID); err != nil {
		t.Fatal(err)
	}

	merged := tparse("2026-03-02T12:00:00Z") // created 03-02T00 → 12h
	if err := st.UpsertCommits(ctx, repoID, []store.Commit{
		{SHA: "c1", AuthorLogin: "neo", CommittedAt: tparse("2026-03-01T08:00:00Z"), Additions: 10, Deletions: 2},
		{SHA: "c2", AuthorLogin: "trinity", CommittedAt: tparse("2026-03-02T09:00:00Z"), Additions: 5, Deletions: 1},
		{SHA: "c3", AuthorLogin: "dependabot[bot]", CommittedAt: tparse("2026-03-02T10:00:00Z"), Additions: 3, Deletions: 0, IsBot: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertPullRequests(ctx, repoID, []store.PullRequest{
		{Number: 1, AuthorLogin: "neo", State: "MERGED", CreatedAt: tparse("2026-03-02T00:00:00Z"), MergedAt: &merged, CommentsCount: 1, Title: "pr"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.RecomputeDailyStats(ctx, repoID, "2026-03-01", "2026-03-02"); err != nil {
		t.Fatal(err)
	}
	// Pin the clock so the 30d window covers the fixture.
	srv.now = func() time.Time { return tparse("2026-03-15T00:00:00Z") }
	return repoID
}

func tparse(s string) time.Time {
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return v
}

func authedGet(t *testing.T, srv *Server, st *store.Store, path string) *httptest.ResponseRecorder {
	t.Helper()
	sess, _ := st.CreateSession(context.Background(), 1, time.Hour)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(&http.Cookie{Name: "gs_session", Value: sess.ID})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	return rec
}

func TestRepoMetricsReturnsRequestedKeys(t *testing.T) {
	srv, st := testServer(t)
	repoID := seedMetricsRepo(t, srv, st)

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/metrics?keys=commit_rate,time_to_merge&window=30d")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("keys returned = %v", out)
	}
	if out["commit_rate"]["kind"] != "time_series" {
		t.Fatalf("commit_rate kind = %v", out["commit_rate"]["kind"])
	}
	if out["time_to_merge"]["kind"] != "scalar" {
		t.Fatalf("time_to_merge kind = %v", out["time_to_merge"]["kind"])
	}
	if out["time_to_merge"]["value"].(float64) != 12 {
		t.Fatalf("time_to_merge value = %v, want 12", out["time_to_merge"]["value"])
	}
}

func TestRepoMetricsExcludeBots(t *testing.T) {
	srv, st := testServer(t)
	repoID := seedMetricsRepo(t, srv, st)

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/metrics?keys=contributor_leaderboard&window=30d&exclude_bots=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out map[string]struct {
		Kind string `json:"kind"`
		Rows []struct {
			Login string `json:"login"`
		} `json:"rows"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	for _, row := range out["contributor_leaderboard"].Rows {
		if row.Login == "dependabot[bot]" {
			t.Fatalf("exclude_bots leaked a bot: %v", out["contributor_leaderboard"].Rows)
		}
	}
}

func TestRepoMetricsAllKeysWhenOmitted(t *testing.T) {
	srv, st := testServer(t)
	repoID := seedMetricsRepo(t, srv, st)

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/metrics?window=30d")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 9 {
		t.Fatalf("default keys = %d, want 9", len(out))
	}
}

func TestRepoMetricsUnknownKey400(t *testing.T) {
	srv, st := testServer(t)
	repoID := seedMetricsRepo(t, srv, st)

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/metrics?keys=bogus")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRepoMetricsUntracked404(t *testing.T) {
	srv, st := testServer(t)
	_ = seedMetricsRepo(t, srv, st) // user 1 tracks repo 1
	ctx := context.Background()
	// A repo the user does NOT track.
	other, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 99, FullName: "x/y", DefaultBranch: "main"})

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(other, 10)+"/metrics?keys=commit_rate")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestRepoMetricsUnauthorized401(t *testing.T) {
	srv, st := testServer(t)
	repoID := seedMetricsRepo(t, srv, st)

	// No session cookie.
	req := httptest.NewRequest(http.MethodGet, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/metrics", nil)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRepoOverviewBundle(t *testing.T) {
	srv, st := testServer(t)
	repoID := seedMetricsRepo(t, srv, st)

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(repoID, 10)+"?window=30d&exclude_bots=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var ov struct {
		ID            int64   `json:"id"`
		FullName      string  `json:"full_name"`
		DefaultBranch string  `json:"default_branch"`
		OpenIssues    int64   `json:"open_issues"`
		OpenPRs       int64   `json:"open_prs"`
		Contributors  int64   `json:"contributors"`
		CommitRate    float64 `json:"commit_rate"`
		IssueRate     float64 `json:"issue_rate"`
		PRRate        float64 `json:"pr_rate"`
		Releases      int64   `json:"releases"`
		SyncStatus    string  `json:"sync_status"`
		LastSyncedAt  *string `json:"last_synced_at"`
		WindowFrom    string  `json:"window_from"`
		WindowTo      string  `json:"window_to"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ov); err != nil {
		t.Fatal(err)
	}
	if ov.ID != repoID || ov.FullName != "a/b" {
		t.Fatalf("overview meta = %+v", ov)
	}
	// As of window end (2026-03-15), no open issues/PRs in the fixture.
	if ov.OpenIssues != 0 || ov.OpenPRs != 0 {
		t.Fatalf("open counts: issues=%d prs=%d", ov.OpenIssues, ov.OpenPRs)
	}
	// Contributors excl bots in window: neo + trinity = 2 (dependabot excluded).
	if ov.Contributors != 2 {
		t.Fatalf("contributors = %d, want 2", ov.Contributors)
	}
	// commit_rate = 3 commits / 30 days window. window 02-13..03-15 inclusive = 31 days.
	if ov.CommitRate <= 0 {
		t.Fatalf("commit_rate = %v, want > 0", ov.CommitRate)
	}
	if ov.WindowTo != "2026-03-15" {
		t.Fatalf("window_to = %q, want 2026-03-15", ov.WindowTo)
	}
}

func TestRepoOverviewUntracked404(t *testing.T) {
	srv, st := testServer(t)
	_ = seedMetricsRepo(t, srv, st)
	ctx := context.Background()
	other, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 77, FullName: "p/q", DefaultBranch: "main"})

	rec := authedGet(t, srv, st, "/api/repos/"+strconv.FormatInt(other, 10))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
