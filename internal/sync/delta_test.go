package sync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github-stats/internal/githubapi"
	"github-stats/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func ptime(s string) time.Time {
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return v
}

// fakeDeltaGraphQL serves the three delta queries with one page each.
func fakeDeltaGraphQL(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		w.Header().Set("Content-Type", "application/json")
		const rl = `"rateLimit":{"cost":1,"remaining":4990,"resetAt":"2026-06-01T13:00:00Z"}`
		switch {
		case strings.Contains(req.Query, "databaseId"):
			w.Write([]byte(`{"data":{"repository":{"databaseId":1,"nameWithOwner":"octocat/hello",
				"isPrivate":false,"description":"hi","stargazerCount":42,"forkCount":5,
				"defaultBranchRef":{"name":"main"},
				"primaryLanguage":{"name":"Go","color":"#00ADD8"},
				"languages":{"totalSize":100,"edges":[{"size":80,"node":{"name":"Go","color":"#00ADD8"}},{"size":20,"node":{"name":"HTML","color":"#e34c26"}}]}},` + rl + `}}`))
		case strings.Contains(req.Query, "since:"):
			w.Write([]byte(`{"data":{"repository":{"ref":{"target":{"history":{
				"pageInfo":{"endCursor":"DC1","hasNextPage":false},
				"nodes":[{"oid":"d1","additions":7,"deletions":1,
					"committedDate":"2026-05-20T08:00:00Z","messageHeadline":"delta commit",
					"author":{"user":{"login":"neo"}}}]}}}}},` + rl + `}}`))
		case strings.Contains(req.Query, "pullRequests"):
			w.Write([]byte(`{"data":{"repository":{"pullRequests":{
				"pageInfo":{"endCursor":"DP1","hasNextPage":false},
				"nodes":[{"number":5,"state":"MERGED","title":"recent","createdAt":"2026-05-19T07:00:00Z",
					"updatedAt":"2026-05-20T10:00:00Z","mergedAt":"2026-05-20T10:00:00Z","closedAt":"2026-05-20T10:00:00Z",
					"additions":3,"deletions":1,"changedFiles":1,"author":{"login":"neo"},
					"comments":{"totalCount":2},"reviews":{"nodes":[]}}]
			}},` + rl + `}}`))
		case strings.Contains(req.Query, "issues"):
			w.Write([]byte(`{"data":{"repository":{"issues":{
				"pageInfo":{"endCursor":"DI1","hasNextPage":false},
				"nodes":[{"number":2,"state":"CLOSED","title":"fixed","createdAt":"2026-05-18T06:00:00Z",
					"updatedAt":"2026-05-20T09:00:00Z","closedAt":"2026-05-20T09:00:00Z",
					"author":{"login":"trinity"},"comments":{"totalCount":1}}]
			}},` + rl + `}}`))
		default:
			t.Errorf("unexpected delta query: %s", req.Query)
			w.WriteHeader(500)
		}
	}
}

func TestRunDeltaIngestsAndRecomputes(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	srv := httptest.NewServer(fakeDeltaGraphQL(t))
	defer srv.Close()
	client := githubapi.NewClient(githubapi.Options{
		Token: "gho_test", GraphQLURL: srv.URL, RESTBaseURL: srv.URL, Store: st, HTTP: &http.Client{},
	})

	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 1, FullName: "octocat/hello", DefaultBranch: "main"})
	// Seed prior sync state so the delta has a cutoff.
	last := ptime("2026-05-15T00:00:00Z")
	if err := st.UpsertSyncState(ctx, &store.SyncState{RepoID: repoID, LastCommitAt: &last, Status: "complete"}); err != nil {
		t.Fatal(err)
	}

	now := ptime("2026-05-21T00:00:00Z")
	var progress []string
	emit := func(phase, detail string) { progress = append(progress, phase+": "+detail) }
	if err := RunDelta(ctx, st, client, repoID, func() time.Time { return now }, emit); err != nil {
		t.Fatalf("RunDelta: %v", err)
	}

	// Per-page progress was surfaced for each phase that ingested rows.
	wantProgress := []string{
		"commits: 1 new commits fetched",
		"prs: 1 updated pull requests",
		"issues: 1 updated issues",
	}
	for _, want := range wantProgress {
		found := false
		for _, got := range progress {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing progress event %q; got %v", want, progress)
		}
	}

	// Events ingested.
	var c, p, i int
	st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM commits WHERE repo_id=?`, repoID).Scan(&c)
	st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pull_requests WHERE repo_id=?`, repoID).Scan(&p)
	st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM issues WHERE repo_id=?`, repoID).Scan(&i)
	if c != 1 || p != 1 || i != 1 {
		t.Fatalf("ingested commits=%d prs=%d issues=%d, want 1/1/1", c, p, i)
	}

	// Aggregates recomputed for the touched dates.
	var commitsDay int
	st.DB.QueryRowContext(ctx,
		`SELECT commits FROM daily_repo_stats WHERE repo_id=? AND date='2026-05-20'`, repoID).Scan(&commitsDay)
	if commitsDay != 1 {
		t.Fatalf("2026-05-20 commits aggregate = %d, want 1", commitsDay)
	}

	// LastCommitAt advanced to newest commit.
	ss, _ := st.GetSyncState(ctx, repoID)
	if ss.LastCommitAt == nil || !ss.LastCommitAt.Equal(ptime("2026-05-20T08:00:00Z")) {
		t.Fatalf("LastCommitAt = %v, want 2026-05-20T08:00:00Z", ss.LastCommitAt)
	}

	// Repo metadata refreshed during the delta (stars + language breakdown).
	rp, _ := st.GetRepo(ctx, repoID)
	if rp.Stargazers != 42 || rp.PrimaryLanguage != "Go" {
		t.Fatalf("metadata not refreshed: stars=%d lang=%q", rp.Stargazers, rp.PrimaryLanguage)
	}
	if !strings.Contains(rp.Languages, `"name":"Go"`) || !strings.Contains(rp.Languages, `"name":"HTML"`) {
		t.Fatalf("languages not refreshed: %s", rp.Languages)
	}
}

func TestRunDeltaStopsAtOverlapCutoff(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	// PR page returns one recent PR (newer than cutoff) and one stale PR (older
	// than cutoff-overlap). The stale one must NOT be ingested because paging
	// stops at the first item past the cutoff.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		w.Header().Set("Content-Type", "application/json")
		const rl = `"rateLimit":{"cost":1,"remaining":4990,"resetAt":"2026-06-01T13:00:00Z"}`
		switch {
		case strings.Contains(req.Query, "databaseId"):
			w.Write([]byte(`{"data":{"repository":{"databaseId":1,"nameWithOwner":"octocat/hello",
				"isPrivate":false,"description":"hi","stargazerCount":42,"forkCount":5,
				"defaultBranchRef":{"name":"main"},
				"primaryLanguage":{"name":"Go","color":"#00ADD8"},
				"languages":{"totalSize":100,"edges":[{"size":80,"node":{"name":"Go","color":"#00ADD8"}},{"size":20,"node":{"name":"HTML","color":"#e34c26"}}]}},` + rl + `}}`))
		case strings.Contains(req.Query, "since:"):
			w.Write([]byte(`{"data":{"repository":{"ref":{"target":{"history":{
				"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[]}}}}},` + rl + `}}`))
		case strings.Contains(req.Query, "pullRequests"):
			w.Write([]byte(`{"data":{"repository":{"pullRequests":{
				"pageInfo":{"endCursor":"P","hasNextPage":true},
				"nodes":[
					{"number":9,"state":"OPEN","title":"recent","createdAt":"2026-05-19T07:00:00Z",
					 "updatedAt":"2026-05-20T10:00:00Z","mergedAt":null,"closedAt":null,
					 "additions":1,"deletions":0,"changedFiles":1,"author":{"login":"neo"},
					 "comments":{"totalCount":0},"reviews":{"nodes":[]}},
					{"number":1,"state":"MERGED","title":"ancient","createdAt":"2020-01-01T07:00:00Z",
					 "updatedAt":"2020-01-01T07:00:00Z","mergedAt":"2020-01-01T07:00:00Z","closedAt":"2020-01-01T07:00:00Z",
					 "additions":1,"deletions":0,"changedFiles":1,"author":{"login":"neo"},
					 "comments":{"totalCount":0},"reviews":{"nodes":[]}}
				]
			}},` + rl + `}}`))
		case strings.Contains(req.Query, "issues"):
			w.Write([]byte(`{"data":{"repository":{"issues":{
				"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[]}},` + rl + `}}`))
		default:
			t.Errorf("unexpected query: %s", req.Query)
		}
	}))
	defer srv.Close()
	client := githubapi.NewClient(githubapi.Options{
		Token: "gho_test", GraphQLURL: srv.URL, RESTBaseURL: srv.URL, Store: st, HTTP: &http.Client{},
	})

	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 1, FullName: "octocat/hello", DefaultBranch: "main"})
	last := ptime("2026-05-18T00:00:00Z")
	st.UpsertSyncState(ctx, &store.SyncState{RepoID: repoID, LastCommitAt: &last, Status: "complete"})

	now := ptime("2026-05-21T00:00:00Z")
	if err := RunDelta(ctx, st, client, repoID, func() time.Time { return now }, nil); err != nil {
		t.Fatal(err)
	}

	// Only the recent PR (#9) ingested; the ancient PR (#1) is past the cutoff.
	var n int
	st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pull_requests WHERE repo_id=?`, repoID).Scan(&n)
	if n != 1 {
		t.Fatalf("PR count = %d, want 1 (stale PR must be skipped at cutoff)", n)
	}
	var exists int
	st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pull_requests WHERE repo_id=? AND number=9`, repoID).Scan(&exists)
	if exists != 1 {
		t.Fatal("recent PR #9 should be ingested")
	}
}
