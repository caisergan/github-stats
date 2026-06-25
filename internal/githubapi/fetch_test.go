package githubapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// gqlResponder routes by which top-level field the query selects.
func gqlResponder(t *testing.T, byField map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		for field, resp := range byField {
			if strings.Contains(req.Query, field) {
				_, _ = w.Write([]byte(resp))
				return
			}
		}
		t.Errorf("no canned response for query: %s", req.Query)
		w.WriteHeader(500)
	}
}

func TestFetchRepoMeta(t *testing.T) {
	srv := httptest.NewServer(gqlResponder(t, map[string]string{
		"databaseId": `{"data":{"repository":{
			"databaseId": 123, "nameWithOwner":"octocat/hello", "isPrivate":true,
			"description":"hi", "stargazerCount":7, "forkCount":2,
			"defaultBranchRef":{"name":"main"},
			"primaryLanguage":{"name":"Go","color":"#00ADD8"},
			"languages":{"totalSize":100,"edges":[
				{"size":80,"node":{"name":"Go","color":"#00ADD8"}},
				{"size":20,"node":{"name":"HTML","color":"#e34c26"}}
			]},
			"rateLimit":{"cost":1,"remaining":4999,"resetAt":"2026-04-01T13:00:00Z"}
		}}}`,
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	r, err := c.FetchRepoMeta(context.Background(), "octocat", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if r.GitHubID != 123 || r.FullName != "octocat/hello" || !r.IsPrivate ||
		r.DefaultBranch != "main" || r.Stargazers != 7 || r.Forks != 2 {
		t.Fatalf("repo meta = %+v", r)
	}
	if r.PrimaryLanguage != "Go" || r.LanguageColor != "#00ADD8" {
		t.Fatalf("language = %q/%q, want Go/#00ADD8", r.PrimaryLanguage, r.LanguageColor)
	}
	if r.Languages != `[{"name":"Go","color":"#00ADD8","size":80},{"name":"HTML","color":"#e34c26","size":20}]` {
		t.Fatalf("languages breakdown = %s", r.Languages)
	}
}

func TestFetchCommitsPage(t *testing.T) {
	srv := httptest.NewServer(gqlResponder(t, map[string]string{
		"history": `{"data":{"repository":{"ref":{"target":{"history":{
			"pageInfo":{"endCursor":"CUR1","hasNextPage":true},
			"nodes":[
				{"oid":"sha1","additions":10,"deletions":2,"committedDate":"2026-03-01T08:00:00Z",
				 "messageHeadline":"first","author":{"user":{"login":"neo"}}},
				{"oid":"sha2","additions":0,"deletions":0,"committedDate":"2026-03-01T09:00:00Z",
				 "messageHeadline":"bot bump","author":{"user":{"login":"dependabot[bot]"}}}
			]
		}}}},"rateLimit":{"cost":1,"remaining":4998,"resetAt":"2026-04-01T13:00:00Z"}}}`,
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	page, err := c.FetchCommits(context.Background(), "octocat", "hello", "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Commits) != 2 || page.EndCursor != "CUR1" || !page.HasNextPage {
		t.Fatalf("commits page = %+v", page)
	}
	if page.Commits[0].SHA != "sha1" || page.Commits[0].AuthorLogin != "neo" ||
		page.Commits[0].Additions != 10 || page.Commits[0].MsgFirstLine != "first" {
		t.Fatalf("commit[0] = %+v", page.Commits[0])
	}
	if !page.Commits[1].IsBot {
		t.Fatalf("commit[1] should be flagged bot: %+v", page.Commits[1])
	}
}

func TestFetchPullRequestsPage(t *testing.T) {
	srv := httptest.NewServer(gqlResponder(t, map[string]string{
		"pullRequests": `{"data":{"repository":{"pullRequests":{
			"pageInfo":{"endCursor":"PR1","hasNextPage":false},
			"nodes":[
				{"number":1,"state":"MERGED","title":"add x","createdAt":"2026-03-01T07:00:00Z",
				 "mergedAt":"2026-03-01T18:00:00Z","closedAt":"2026-03-01T18:00:00Z",
				 "additions":10,"deletions":2,"changedFiles":3,
				 "author":{"login":"neo"},"comments":{"totalCount":4},
				 "reviews":{"nodes":[{"submittedAt":"2026-03-01T12:00:00Z"}]}},
				{"number":2,"state":"OPEN","title":"bump","createdAt":"2026-03-02T07:00:00Z",
				 "mergedAt":null,"closedAt":null,"additions":1,"deletions":1,"changedFiles":1,
				 "author":{"login":"dependabot[bot]"},"comments":{"totalCount":0},
				 "reviews":{"nodes":[]}}
			]
		}},"rateLimit":{"cost":1,"remaining":4997,"resetAt":"2026-04-01T13:00:00Z"}}}`,
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	page, err := c.FetchPullRequests(context.Background(), "octocat", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.PRs) != 2 || page.HasNextPage {
		t.Fatalf("PR page = %+v", page)
	}
	pr := page.PRs[0]
	if pr.Number != 1 || pr.State != "MERGED" || pr.CommentsCount != 4 ||
		pr.MergedAt == nil || pr.FirstReviewAt == nil || pr.ChangedFiles != 3 {
		t.Fatalf("PR[0] = %+v", pr)
	}
	if page.PRs[1].MergedAt != nil || !page.PRs[1].IsBot {
		t.Fatalf("PR[1] = %+v", page.PRs[1])
	}
}

func TestFetchIssuesPage(t *testing.T) {
	srv := httptest.NewServer(gqlResponder(t, map[string]string{
		"issues": `{"data":{"repository":{"issues":{
			"pageInfo":{"endCursor":"IS1","hasNextPage":false},
			"nodes":[
				{"number":1,"state":"CLOSED","title":"bug","createdAt":"2026-03-01T06:00:00Z",
				 "closedAt":"2026-03-02T11:00:00Z","author":{"login":"neo"},"comments":{"totalCount":4}}
			]
		}},"rateLimit":{"cost":1,"remaining":4996,"resetAt":"2026-04-01T13:00:00Z"}}}`,
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	page, err := c.FetchIssues(context.Background(), "octocat", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Issues) != 1 || page.HasNextPage {
		t.Fatalf("issue page = %+v", page)
	}
	is := page.Issues[0]
	if is.Number != 1 || is.State != "CLOSED" || is.CommentsCount != 4 || is.ClosedAt == nil {
		t.Fatalf("issue[0] = %+v", is)
	}
}

func TestFetchReleasesPage(t *testing.T) {
	srv := httptest.NewServer(gqlResponder(t, map[string]string{
		"releases": `{"data":{"repository":{"releases":{
			"pageInfo":{"endCursor":"RE1","hasNextPage":false},
			"nodes":[
				{"tagName":"v1.0.0","name":"First","publishedAt":"2026-03-01T12:00:00Z","author":{"login":"neo"}},
				{"tagName":"v1.1.0","name":"Second","publishedAt":null,"author":null}
			]
		}},"rateLimit":{"cost":1,"remaining":4995,"resetAt":"2026-04-01T13:00:00Z"}}}`,
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL, "http://unused")

	page, err := c.FetchReleases(context.Background(), "octocat", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Releases) != 2 || page.HasNextPage {
		t.Fatalf("release page = %+v", page)
	}
	if page.Releases[0].Tag != "v1.0.0" || page.Releases[0].PublishedAt == nil ||
		page.Releases[0].AuthorLogin != "neo" {
		t.Fatalf("release[0] = %+v", page.Releases[0])
	}
	if page.Releases[1].PublishedAt != nil || page.Releases[1].AuthorLogin != "" {
		t.Fatalf("release[1] = %+v", page.Releases[1])
	}
}
