package githubapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchCommitsSincePassesSinceVar(t *testing.T) {
	var gotSince any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.Unmarshal(raw, &req)
		if !strings.Contains(req.Query, "since:") && !strings.Contains(req.Query, "$since") {
			t.Errorf("query missing since arg: %s", req.Query)
		}
		gotSince = req.Variables["since"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"repository":{"ref":{"target":{"history":{
			"pageInfo":{"endCursor":"DC1","hasNextPage":false},
			"nodes":[{"oid":"s1","additions":4,"deletions":1,
				"committedDate":"2026-05-10T08:00:00Z","messageHeadline":"delta",
				"author":{"user":{"login":"neo"}}}]
		}}}},"rateLimit":{"cost":1,"remaining":4999,"resetAt":"2026-06-01T13:00:00Z"}}}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	since := time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC)
	page, err := c.FetchCommitsSince(context.Background(), "octocat", "hello", "main", since, "")
	if err != nil {
		t.Fatal(err)
	}
	if gotSince != "2026-05-09T00:00:00Z" {
		t.Fatalf("since variable = %v, want RFC3339 UTC", gotSince)
	}
	if len(page.Commits) != 1 || page.Commits[0].SHA != "s1" || page.EndCursor != "DC1" {
		t.Fatalf("commits-since page = %+v", page)
	}
}

func TestFetchPullRequestsUpdatedOrdersByUpdatedAt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		if !strings.Contains(req.Query, "UPDATED_AT") || !strings.Contains(req.Query, "DESC") {
			t.Errorf("PR query not ordered by UPDATED_AT DESC: %s", req.Query)
		}
		if !strings.Contains(req.Query, "updatedAt") {
			t.Errorf("PR query missing updatedAt selection: %s", req.Query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"repository":{"pullRequests":{
			"pageInfo":{"endCursor":"DP1","hasNextPage":true},
			"nodes":[
				{"number":9,"state":"MERGED","title":"recent","createdAt":"2026-05-01T07:00:00Z",
				 "updatedAt":"2026-05-11T10:00:00Z","mergedAt":"2026-05-11T10:00:00Z","closedAt":"2026-05-11T10:00:00Z",
				 "additions":2,"deletions":1,"changedFiles":1,"author":{"login":"neo"},
				 "comments":{"totalCount":1},"reviews":{"nodes":[]}}
			]
		}},"rateLimit":{"cost":1,"remaining":4998,"resetAt":"2026-06-01T13:00:00Z"}}}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	page, err := c.FetchPullRequestsUpdated(context.Background(), "octocat", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.PRs) != 1 || !page.HasNextPage || page.EndCursor != "DP1" {
		t.Fatalf("PR updated page = %+v", page)
	}
	pr := page.PRs[0]
	if pr.PullRequest.Number != 9 || pr.PullRequest.State != "MERGED" || pr.UpdatedAt.IsZero() {
		t.Fatalf("PR[0] = %+v (UpdatedAt=%v)", pr, pr.UpdatedAt)
	}
	if !pr.UpdatedAt.Equal(time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("UpdatedAt = %v, want 2026-05-11T10:00:00Z", pr.UpdatedAt)
	}
}

func TestFetchIssuesUpdatedOrdersByUpdatedAt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		if !strings.Contains(req.Query, "UPDATED_AT") || !strings.Contains(req.Query, "DESC") {
			t.Errorf("issue query not ordered by UPDATED_AT DESC: %s", req.Query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"repository":{"issues":{
			"pageInfo":{"endCursor":"DI1","hasNextPage":false},
			"nodes":[
				{"number":3,"state":"CLOSED","title":"fixed","createdAt":"2026-05-02T06:00:00Z",
				 "updatedAt":"2026-05-12T09:00:00Z","closedAt":"2026-05-12T09:00:00Z",
				 "author":{"login":"trinity"},"comments":{"totalCount":2}}
			]
		}},"rateLimit":{"cost":1,"remaining":4997,"resetAt":"2026-06-01T13:00:00Z"}}}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	page, err := c.FetchIssuesUpdated(context.Background(), "octocat", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Issues) != 1 || page.HasNextPage {
		t.Fatalf("issue updated page = %+v", page)
	}
	is := page.Issues[0]
	if is.Issue.Number != 3 || is.Issue.State != "CLOSED" || is.UpdatedAt.IsZero() {
		t.Fatalf("issue[0] = %+v (UpdatedAt=%v)", is, is.UpdatedAt)
	}
}
