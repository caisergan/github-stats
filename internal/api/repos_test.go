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
			"defaultBranchRef":{"name":"main"}},
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
