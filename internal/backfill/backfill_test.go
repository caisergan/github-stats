package backfill

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// fakeGraphQL answers each query type, paging commits across two responses.
func fakeGraphQL(t *testing.T) http.HandlerFunc {
	var commitCalls int
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		w.Header().Set("Content-Type", "application/json")
		const rl = `"rateLimit":{"cost":1,"remaining":4990,"resetAt":"2026-04-01T13:00:00Z"}`
		switch {
		case strings.Contains(req.Query, "databaseId"):
			w.Write([]byte(`{"data":{"repository":{"databaseId":555,"nameWithOwner":"octocat/hello",
				"isPrivate":false,"description":"hi","stargazerCount":1,"forkCount":0,
				"defaultBranchRef":{"name":"main"}},` + rl + `}}`))
		case strings.Contains(req.Query, "history"):
			commitCalls++
			if commitCalls == 1 {
				w.Write([]byte(`{"data":{"repository":{"ref":{"target":{"history":{
					"pageInfo":{"endCursor":"C1","hasNextPage":true},
					"nodes":[{"oid":"sha1","additions":10,"deletions":2,
						"committedDate":"2026-03-01T08:00:00Z","messageHeadline":"first",
						"author":{"user":{"login":"neo"}}}]}}}}},` + rl + `}}`))
			} else {
				w.Write([]byte(`{"data":{"repository":{"ref":{"target":{"history":{
					"pageInfo":{"endCursor":"C2","hasNextPage":false},
					"nodes":[{"oid":"sha2","additions":3,"deletions":0,
						"committedDate":"2026-03-02T09:00:00Z","messageHeadline":"second",
						"author":{"user":{"login":"trinity"}}}]}}}}},` + rl + `}}`))
			}
		case strings.Contains(req.Query, "pullRequests"):
			w.Write([]byte(`{"data":{"repository":{"pullRequests":{
				"pageInfo":{"endCursor":"P1","hasNextPage":false},
				"nodes":[{"number":1,"state":"MERGED","title":"add","createdAt":"2026-03-01T07:00:00Z",
					"mergedAt":"2026-03-01T18:00:00Z","closedAt":"2026-03-01T18:00:00Z",
					"additions":10,"deletions":2,"changedFiles":3,"author":{"login":"neo"},
					"comments":{"totalCount":2},"reviews":{"nodes":[{"submittedAt":"2026-03-01T12:00:00Z"}]}}]
			}},` + rl + `}}`))
		case strings.Contains(req.Query, "issues"):
			w.Write([]byte(`{"data":{"repository":{"issues":{
				"pageInfo":{"endCursor":"I1","hasNextPage":false},
				"nodes":[{"number":1,"state":"OPEN","title":"bug","createdAt":"2026-03-02T06:00:00Z",
					"closedAt":null,"author":{"login":"trinity"},"comments":{"totalCount":1}}]
			}},` + rl + `}}`))
		case strings.Contains(req.Query, "releases"):
			w.Write([]byte(`{"data":{"repository":{"releases":{
				"pageInfo":{"endCursor":"R1","hasNextPage":false},
				"nodes":[{"tagName":"v1.0.0","name":"First","publishedAt":"2026-03-01T12:00:00Z",
					"author":{"login":"neo"}}]
			}},` + rl + `}}`))
		default:
			t.Errorf("unexpected query: %s", req.Query)
			w.WriteHeader(500)
		}
	}
}

func TestRunBackfillEndToEnd(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	srv := httptest.NewServer(fakeGraphQL(t))
	defer srv.Close()

	client := githubapi.NewClient(githubapi.Options{
		Token:       "gho_test",
		GraphQLURL:  srv.URL,
		RESTBaseURL: srv.URL,
		Store:       st,
		HTTP:        &http.Client{},
	})

	repoID, err := st.UpsertRepo(ctx, &store.Repo{
		GitHubID: 555, FullName: "octocat/hello", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := Run(ctx, st, client, repoID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Events: two commits (both pages), one PR, one issue, one release.
	assertCount(t, st, "SELECT COUNT(*) FROM commits WHERE repo_id=?", repoID, 2)
	assertCount(t, st, "SELECT COUNT(*) FROM pull_requests WHERE repo_id=?", repoID, 1)
	assertCount(t, st, "SELECT COUNT(*) FROM issues WHERE repo_id=?", repoID, 1)
	assertCount(t, st, "SELECT COUNT(*) FROM releases WHERE repo_id=?", repoID, 1)

	// Aggregates were recomputed across the touched span (2026-03-01..2026-03-02).
	var commitsDay1 int
	st.DB.QueryRowContext(ctx,
		`SELECT commits FROM daily_repo_stats WHERE repo_id=? AND date='2026-03-01'`, repoID,
	).Scan(&commitsDay1)
	if commitsDay1 != 1 {
		t.Fatalf("day1 commits aggregate = %d, want 1", commitsDay1)
	}
	var prsMergedDay1 int
	st.DB.QueryRowContext(ctx,
		`SELECT prs_merged FROM daily_repo_stats WHERE repo_id=? AND date='2026-03-01'`, repoID,
	).Scan(&prsMergedDay1)
	if prsMergedDay1 != 1 {
		t.Fatalf("day1 prs_merged = %d, want 1", prsMergedDay1)
	}

	// Sync state marked complete with cursors persisted.
	ss, err := st.GetSyncState(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}
	if ss.Status != "complete" {
		t.Fatalf("status = %q, want complete", ss.Status)
	}
	if ss.LastCommitCursor != "C2" || ss.LastPRCursor != "P1" ||
		ss.LastIssueCursor != "I1" || ss.LastReleaseCursor != "R1" {
		t.Fatalf("cursors not persisted: %+v", ss)
	}
	if ss.LastBackfillAt == nil {
		t.Fatal("last_backfill_at not set")
	}

	// Repo meta refreshed from GraphQL (description + stargazers).
	r, _ := st.GetRepo(ctx, repoID)
	if r.Description != "hi" || r.Stargazers != 1 {
		t.Fatalf("repo meta not refreshed: %+v", r)
	}
}

func assertCount(t *testing.T, st *store.Store, query string, repoID int64, want int) {
	t.Helper()
	var n int
	if err := st.DB.QueryRow(query, repoID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("%s = %d, want %d", query, n, want)
	}
}

func TestRunResumesFromSavedCursor(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	// Server that fails if the commit query arrives WITHOUT a cursor — proving
	// the backfill resumed from the saved cursor instead of restarting at page 0.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.Unmarshal(raw, &req)
		w.Header().Set("Content-Type", "application/json")
		const rl = `"rateLimit":{"cost":1,"remaining":4990,"resetAt":"2026-04-01T13:00:00Z"}`
		switch {
		case strings.Contains(req.Query, "databaseId"):
			w.Write([]byte(`{"data":{"repository":{"databaseId":555,"nameWithOwner":"octocat/hello",
				"isPrivate":false,"description":"hi","stargazerCount":1,"forkCount":0,
				"defaultBranchRef":{"name":"main"}},` + rl + `}}`))
		case strings.Contains(req.Query, "history"):
			if req.Variables["after"] != "SAVED" {
				t.Errorf("expected resume cursor SAVED, got %v", req.Variables["after"])
			}
			w.Write([]byte(`{"data":{"repository":{"ref":{"target":{"history":{
				"pageInfo":{"endCursor":"C9","hasNextPage":false},
				"nodes":[{"oid":"sha9","additions":1,"deletions":0,
					"committedDate":"2026-03-03T09:00:00Z","messageHeadline":"resumed",
					"author":{"user":{"login":"neo"}}}]}}}}},` + rl + `}}`))
		case strings.Contains(req.Query, "pullRequests"):
			w.Write([]byte(`{"data":{"repository":{"pullRequests":{"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[]}},` + rl + `}}`))
		case strings.Contains(req.Query, "issues"):
			w.Write([]byte(`{"data":{"repository":{"issues":{"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[]}},` + rl + `}}`))
		case strings.Contains(req.Query, "releases"):
			w.Write([]byte(`{"data":{"repository":{"releases":{"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[]}},` + rl + `}}`))
		default:
			t.Errorf("unexpected query: %s", req.Query)
		}
	}))
	defer srv.Close()

	client := githubapi.NewClient(githubapi.Options{
		Token: "gho_test", GraphQLURL: srv.URL, RESTBaseURL: srv.URL, Store: st, HTTP: &http.Client{},
	})
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 555, FullName: "octocat/hello", DefaultBranch: "main"})

	// Pre-seed a saved commit cursor as if a prior run was interrupted.
	if err := st.UpsertSyncState(ctx, &store.SyncState{RepoID: repoID, LastCommitCursor: "SAVED", Status: "backfilling"}); err != nil {
		t.Fatal(err)
	}

	if err := Run(ctx, st, client, repoID); err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCount(t, st, "SELECT COUNT(*) FROM commits WHERE repo_id=?", repoID, 1)
}
