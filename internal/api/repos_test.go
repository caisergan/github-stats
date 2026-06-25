package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github-stats/internal/auth"
	"github-stats/internal/config"
	"github-stats/internal/crypto"
	"github-stats/internal/githubapi"
	"github-stats/internal/store"
	"github-stats/internal/sync"
)

// serverWithGitHub builds a Server whose per-user client factory points at the
// given fake GitHub URL, plus a seeded logged-in user with an encrypted oauth
// credential. Returns the server, store, and the user's session cookie.
func serverWithGitHub(t *testing.T, ghURL string) (*Server, *store.Store, *http.Cookie) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cph, _ := crypto.NewCipher(make([]byte, 32))
	cfg := config.Config{
		SessionTTL:       time.Hour,
		BaseURL:          "http://localhost:8080",
		GitHubAPIBaseURL: ghURL,
	}
	svc := auth.NewService(cfg, st, &auth.OAuthClient{}, cph)

	ctx := context.Background()
	uid, _ := st.UpsertUser(ctx, &store.User{GitHubID: 1, Login: "neo"})
	enc, _ := cph.Encrypt([]byte("gho_user_token"))
	if err := st.UpsertCredential(ctx, &store.Credential{UserID: uid, Kind: "oauth", EncToken: enc, Scopes: "repo"}); err != nil {
		t.Fatal(err)
	}
	sess, _ := st.CreateSession(ctx, uid, time.Hour)

	factory := func(repoID int64) (*githubapi.Client, error) {
		return githubapi.NewClient(githubapi.Options{
			Token: "gho_user_token", GraphQLURL: ghURL, RESTBaseURL: ghURL, Store: st, HTTP: &http.Client{},
		}), nil
	}
	eng := sync.NewEngine(st, factory, sync.Config{})
	srv := NewServer(cfg, st, svc, eng, cph)
	return srv, st, &http.Cookie{Name: "gs_session", Value: sess.ID}
}

func TestAddRepoFetchesTracksAndEnqueues(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		if r.Header.Get("Authorization") != "Bearer gho_user_token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"repository":{"databaseId":777,"nameWithOwner":"octocat/hello",
			"isPrivate":true,"description":"hi","stargazerCount":3,"forkCount":1,
			"defaultBranchRef":{"name":"main"},
			"primaryLanguage":{"name":"Go","color":"#00ADD8"}},
			"rateLimit":{"cost":1,"remaining":4999,"resetAt":"2026-06-01T13:00:00Z"}}}`))
	}))
	defer gh.Close()

	srv, st, cookie := serverWithGitHub(t, gh.URL)

	body := strings.NewReader(`{"full_name":"octocat/hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/repos", body)
	req.AddCookie(cookie)
	withCSRF(req)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["full_name"] != "octocat/hello" || got["is_private"] != true || got["default_branch"] != "main" {
		t.Fatalf("repo json = %v", got)
	}
	if got["language"] != "Go" || got["language_color"] != "#00ADD8" {
		t.Fatalf("language json = %v / %v, want Go / #00ADD8", got["language"], got["language_color"])
	}

	// Repo upserted, tracked, and a backfill job enqueued.
	ctx := context.Background()
	r, err := st.GetRepoByFullName(ctx, "octocat/hello")
	if err != nil {
		t.Fatal(err)
	}
	if tracked, _ := st.IsTracked(ctx, 1, r.ID); !tracked {
		t.Fatal("repo not tracked")
	}
	jobs, _ := st.ListJobsForRepo(ctx, r.ID)
	if len(jobs) != 1 || jobs[0].Kind != "backfill" {
		t.Fatalf("expected 1 backfill job, got %+v", jobs)
	}
}

func TestAddRepoInaccessibleRepoReturns404(t *testing.T) {
	// GitHub returns a "could not resolve" GraphQL error for a private repo the
	// token can't see (and for nonexistent repos). The handler should translate
	// that into an actionable 404, not a generic 502.
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"errors":[{"message":"Could not resolve to a Repository with the name 'caisergan/trade-station'."}]}`))
	}))
	defer gh.Close()

	srv, _, cookie := serverWithGitHub(t, gh.URL)

	req := httptest.NewRequest(http.MethodPost, "/api/repos",
		strings.NewReader(`{"full_name":"caisergan/trade-station"}`))
	req.AddCookie(cookie)
	withCSRF(req)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "repo") {
		t.Fatalf("expected an actionable message mentioning the repo scope, got: %s", rec.Body.String())
	}
}

func TestAddRepoRejectsBadBody(t *testing.T) {
	srv, _, cookie := serverWithGitHub(t, "http://unused")
	req := httptest.NewRequest(http.MethodPost, "/api/repos", strings.NewReader(`{"full_name":""}`))
	req.AddCookie(cookie)
	withCSRF(req)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestListReposIncludesSyncStatus(t *testing.T) {
	srv, st, cookie := serverWithGitHub(t, "http://unused")
	ctx := context.Background()

	repoID, _ := st.UpsertRepo(ctx, &store.Repo{
		GitHubID: 5, FullName: "octocat/hello", IsPrivate: false, DefaultBranch: "main",
	})
	st.TrackRepo(ctx, 1, repoID)
	last := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	st.UpsertSyncState(ctx, &store.SyncState{RepoID: repoID, LastBackfillAt: &last, Status: "complete"})

	req := httptest.NewRequest(http.MethodGet, "/api/repos", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var repos []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &repos); err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("len = %d, want 1", len(repos))
	}
	r := repos[0]
	for _, k := range []string{"id", "full_name", "is_private", "default_branch", "sync_status", "last_synced_at"} {
		if _, ok := r[k]; !ok {
			t.Fatalf("repo json missing key %q: %v", k, r)
		}
	}
	if r["sync_status"] != "complete" {
		t.Fatalf("sync_status = %v, want complete", r["sync_status"])
	}
}

func TestUntrackRepo(t *testing.T) {
	srv, st, cookie := serverWithGitHub(t, "http://unused")
	ctx := context.Background()
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 5, FullName: "a/b", DefaultBranch: "main"})
	st.TrackRepo(ctx, 1, repoID)

	req := httptest.NewRequest(http.MethodDelete, "/api/repos/"+strconv.FormatInt(repoID, 10), nil)
	req.AddCookie(cookie)
	withCSRF(req)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if tracked, _ := st.IsTracked(ctx, 1, repoID); tracked {
		t.Fatal("repo still tracked after delete")
	}
}

func TestUntrackRepoHardDeletesWhenOrphaned(t *testing.T) {
	srv, st, cookie := serverWithGitHub(t, "http://unused") // seeds user 1 (neo)
	ctx := context.Background()
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 42, FullName: "octo/gone", DefaultBranch: "main"})
	st.TrackRepo(ctx, 1, repoID)
	if _, err := st.DB.ExecContext(ctx,
		`INSERT INTO commits(repo_id, sha, committed_at) VALUES (?, 'deadbeef', '2026-01-01T00:00:00Z')`, repoID); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/repos/"+strconv.FormatInt(repoID, 10), nil)
	req.AddCookie(cookie)
	withCSRF(req)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body %s)", rec.Code, rec.Body.String())
	}

	// The last tracker left, so the repo and its data are hard-deleted.
	if _, err := st.GetRepoByFullName(ctx, "octo/gone"); err != store.ErrNotFound {
		t.Fatalf("repo not hard-deleted: %v", err)
	}
	var n int
	st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM commits WHERE repo_id = ?`, repoID).Scan(&n)
	if n != 0 {
		t.Fatalf("commits not purged: %d", n)
	}
}

func TestUntrackRepoKeepsDataWhileOthersTrack(t *testing.T) {
	srv, st, cookie := serverWithGitHub(t, "http://unused") // seeds user 1 (neo)
	ctx := context.Background()
	uid2, _ := st.UpsertUser(ctx, &store.User{GitHubID: 2, Login: "trinity"})
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 7, FullName: "octo/shared", DefaultBranch: "main"})
	st.TrackRepo(ctx, 1, repoID)
	st.TrackRepo(ctx, uid2, repoID)

	req := httptest.NewRequest(http.MethodDelete, "/api/repos/"+strconv.FormatInt(repoID, 10), nil)
	req.AddCookie(cookie) // user 1 untracks
	withCSRF(req)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	// User 2 still tracks it, so nothing is purged.
	if _, err := st.GetRepoByFullName(ctx, "octo/shared"); err != nil {
		t.Fatalf("repo wrongly purged while still tracked: %v", err)
	}
	if tracked, _ := st.IsTracked(ctx, uid2, repoID); !tracked {
		t.Fatal("second user's tracking was removed")
	}
}

func TestRefreshRepoEnqueuesDelta(t *testing.T) {
	srv, st, cookie := serverWithGitHub(t, "http://unused")
	ctx := context.Background()
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 5, FullName: "a/b", DefaultBranch: "main"})
	st.TrackRepo(ctx, 1, repoID)

	req := httptest.NewRequest(http.MethodPost, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/refresh", nil)
	req.AddCookie(cookie)
	withCSRF(req)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	jobs, _ := st.ListJobsForRepo(ctx, repoID)
	if len(jobs) != 1 || jobs[0].Kind != "delta" {
		t.Fatalf("expected 1 delta job, got %+v", jobs)
	}
}

func TestLoadAllCommitsEnqueuesBackfill(t *testing.T) {
	srv, st, cookie := serverWithGitHub(t, "http://unused")
	ctx := context.Background()
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 5, FullName: "a/b", DefaultBranch: "main"})
	st.TrackRepo(ctx, 1, repoID)
	path := "/api/repos/" + strconv.FormatInt(repoID, 10) + "/load-all-commits"

	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.AddCookie(cookie)
		withCSRF(req)
		rec := httptest.NewRecorder()
		srv.Router().ServeHTTP(rec, req)
		return rec
	}

	if rec := post(); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (body %s)", rec.Code, rec.Body.String())
	}
	jobs, _ := st.ListJobsForRepo(ctx, repoID)
	if len(jobs) != 1 || jobs[0].Kind != "backfill" {
		t.Fatalf("expected 1 backfill job, got %+v", jobs)
	}

	// Idempotent while a job is open: a second call enqueues nothing more.
	if rec := post(); rec.Code != http.StatusAccepted {
		t.Fatalf("second status = %d, want 202", rec.Code)
	}
	jobs2, _ := st.ListJobsForRepo(ctx, repoID)
	if len(jobs2) != 1 {
		t.Fatalf("open job should dedupe; got %d jobs", len(jobs2))
	}
}

func TestRepoSyncStatusReportsActiveJob(t *testing.T) {
	srv, st, _ := serverWithGitHub(t, "http://unused")
	ctx := context.Background()
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 6, FullName: "c/d", DefaultBranch: "main"})
	st.TrackRepo(ctx, 1, repoID)
	path := "/api/repos/" + strconv.FormatInt(repoID, 10) + "/sync/status"

	// No jobs yet → idle (empty) status.
	rec := authedGet(t, srv, st, path)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var s0 struct {
		Status string `json:"status"`
		Active bool   `json:"active"`
	}
	json.Unmarshal(rec.Body.Bytes(), &s0)
	if s0.Active || s0.Status != "" {
		t.Fatalf("idle status = %+v, want empty/inactive", s0)
	}

	// Enqueue a backfill → status reports it active.
	if _, err := st.EnqueueJob(ctx, repoID, "backfill", time.Now()); err != nil {
		t.Fatal(err)
	}
	rec2 := authedGet(t, srv, st, path)
	var s1 struct {
		Kind   string `json:"kind"`
		Status string `json:"status"`
		Active bool   `json:"active"`
	}
	json.Unmarshal(rec2.Body.Bytes(), &s1)
	if !s1.Active || s1.Kind != "backfill" || s1.Status != "pending" {
		t.Fatalf("active status = %+v, want backfill/pending/active", s1)
	}
}

func TestRefreshRejectsUntrackedRepo(t *testing.T) {
	srv, st, cookie := serverWithGitHub(t, "http://unused")
	ctx := context.Background()
	// Repo exists but the caller does not track it.
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 5, FullName: "a/b", DefaultBranch: "main"})

	req := httptest.NewRequest(http.MethodPost, "/api/repos/"+strconv.FormatInt(repoID, 10)+"/refresh", nil)
	req.AddCookie(cookie)
	withCSRF(req)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for untracked repo", rec.Code)
	}
}

func mustTrackedRepo(t *testing.T, st *store.Store) (int64, *store.Repo) {
	t.Helper()
	ctx := context.Background()
	repoID, err := st.UpsertRepo(ctx, &store.Repo{GitHubID: 42, FullName: "octocat/hello", DefaultBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.TrackRepo(ctx, 1, repoID); err != nil {
		t.Fatal(err)
	}
	r, _ := st.GetRepo(ctx, repoID)
	return repoID, r
}
