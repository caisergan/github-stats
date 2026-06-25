package githubapi

import (
	"context"
	"encoding/json"
	"time"

	"github-stats/internal/store"
)

const pageSize = 100

// parseTime parses an RFC3339 timestamp; the zero value on empty/invalid input.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// parseTimePtr returns a *time.Time, nil for empty/invalid input.
func parseTimePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

// --- Repo meta -------------------------------------------------------------

const repoMetaQuery = `
query($owner:String!, $name:String!) {
  repository(owner:$owner, name:$name) {
    databaseId
    nameWithOwner
    isPrivate
    description
    stargazerCount
    forkCount
    defaultBranchRef {
      name
      target { ... on Commit { history { totalCount } } }
    }
    primaryLanguage { name color }
    languages(first: 12, orderBy: {field: SIZE, direction: DESC}) {
      totalSize
      edges { size node { name color } }
    }
  }
  rateLimit { cost remaining resetAt }
}`

// FetchRepoMeta returns repository metadata as a store.Repo (ID unset; the
// caller upserts to obtain the local id).
func (c *Client) FetchRepoMeta(ctx context.Context, owner, name string) (*store.Repo, error) {
	var data struct {
		Repository struct {
			DatabaseID     int64  `json:"databaseId"`
			NameWithOwner  string `json:"nameWithOwner"`
			IsPrivate      bool   `json:"isPrivate"`
			Description    string `json:"description"`
			StargazerCount int64  `json:"stargazerCount"`
			ForkCount      int64  `json:"forkCount"`
			DefaultBranch  struct {
				Name   string `json:"name"`
				Target struct {
					History struct {
						TotalCount int64 `json:"totalCount"`
					} `json:"history"`
				} `json:"target"`
			} `json:"defaultBranchRef"`
			PrimaryLanguage struct {
				Name  string `json:"name"`
				Color string `json:"color"`
			} `json:"primaryLanguage"`
			Languages struct {
				Edges []struct {
					Size int64 `json:"size"`
					Node struct {
						Name  string `json:"name"`
						Color string `json:"color"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"languages"`
		} `json:"repository"`
		RateLimit RateLimit `json:"rateLimit"`
	}
	if err := c.graphql(ctx, repoMetaQuery,
		map[string]any{"owner": owner, "name": name}, &data); err != nil {
		return nil, err
	}
	r := &store.Repo{
		GitHubID:        data.Repository.DatabaseID,
		FullName:        data.Repository.NameWithOwner,
		IsPrivate:       data.Repository.IsPrivate,
		DefaultBranch:   data.Repository.DefaultBranch.Name,
		Description:     data.Repository.Description,
		Stargazers:      data.Repository.StargazerCount,
		Forks:           data.Repository.ForkCount,
		PrimaryLanguage: data.Repository.PrimaryLanguage.Name,
		LanguageColor:   data.Repository.PrimaryLanguage.Color,
		CommitCount:     data.Repository.DefaultBranch.Target.History.TotalCount,
	}
	// Marshal the language breakdown (name/color/size, desc by size) to JSON for
	// the repos.languages column; always at least "[]".
	type langEntry struct {
		Name  string `json:"name"`
		Color string `json:"color"`
		Size  int64  `json:"size"`
	}
	langs := make([]langEntry, 0, len(data.Repository.Languages.Edges))
	for _, e := range data.Repository.Languages.Edges {
		langs = append(langs, langEntry{Name: e.Node.Name, Color: e.Node.Color, Size: e.Size})
	}
	if b, err := json.Marshal(langs); err == nil {
		r.Languages = string(b)
	} else {
		r.Languages = "[]"
	}
	return r, nil
}

// --- Commits ---------------------------------------------------------------

// CommitPage is one page of commit history.
type CommitPage struct {
	Commits     []store.Commit
	EndCursor   string
	HasNextPage bool
}

const commitsQuery = `
query($owner:String!, $name:String!, $branch:String!, $after:String) {
  repository(owner:$owner, name:$name) {
    ref(qualifiedName:$branch) {
      target {
        ... on Commit {
          history(first:100, after:$after) {
            pageInfo { endCursor hasNextPage }
            nodes {
              oid
              additions
              deletions
              committedDate
              messageHeadline
              author { user { login } }
            }
          }
        }
      }
    }
  }
  rateLimit { cost remaining resetAt }
}`

func (c *Client) FetchCommits(ctx context.Context, owner, name, branch, after string) (*CommitPage, error) {
	var data struct {
		Repository struct {
			Ref struct {
				Target struct {
					History struct {
						PageInfo pageInfo `json:"pageInfo"`
						Nodes    []struct {
							OID             string `json:"oid"`
							Additions       int64  `json:"additions"`
							Deletions       int64  `json:"deletions"`
							CommittedDate   string `json:"committedDate"`
							MessageHeadline string `json:"messageHeadline"`
							Author          struct {
								User struct {
									Login string `json:"login"`
								} `json:"user"`
							} `json:"author"`
						} `json:"nodes"`
					} `json:"history"`
				} `json:"target"`
			} `json:"ref"`
		} `json:"repository"`
		RateLimit RateLimit `json:"rateLimit"`
	}
	vars := map[string]any{"owner": owner, "name": name, "branch": "refs/heads/" + branch}
	if after != "" {
		vars["after"] = after
	}
	if err := c.graphql(ctx, commitsQuery, vars, &data); err != nil {
		return nil, err
	}
	h := data.Repository.Ref.Target.History
	page := &CommitPage{EndCursor: h.PageInfo.EndCursor, HasNextPage: h.PageInfo.HasNextPage}
	for _, n := range h.Nodes {
		login := n.Author.User.Login
		page.Commits = append(page.Commits, store.Commit{
			SHA:          n.OID,
			AuthorLogin:  login,
			CommittedAt:  parseTime(n.CommittedDate),
			Additions:    n.Additions,
			Deletions:    n.Deletions,
			IsBot:        IsBot(login),
			MsgFirstLine: n.MessageHeadline,
		})
	}
	return page, nil
}

// --- Pull requests ---------------------------------------------------------

// PRPage is one page of pull requests.
type PRPage struct {
	PRs         []store.PullRequest
	EndCursor   string
	HasNextPage bool
}

const pullRequestsQuery = `
query($owner:String!, $name:String!, $after:String) {
  repository(owner:$owner, name:$name) {
    pullRequests(first:100, after:$after, orderBy:{field:CREATED_AT, direction:ASC}) {
      pageInfo { endCursor hasNextPage }
      nodes {
        number
        state
        title
        createdAt
        mergedAt
        closedAt
        additions
        deletions
        changedFiles
        author { login }
        comments { totalCount }
        reviews(first:1) { nodes { submittedAt } }
      }
    }
  }
  rateLimit { cost remaining resetAt }
}`

func (c *Client) FetchPullRequests(ctx context.Context, owner, name, after string) (*PRPage, error) {
	var data struct {
		Repository struct {
			PullRequests struct {
				PageInfo pageInfo `json:"pageInfo"`
				Nodes    []struct {
					Number       int64  `json:"number"`
					State        string `json:"state"`
					Title        string `json:"title"`
					CreatedAt    string `json:"createdAt"`
					MergedAt     string `json:"mergedAt"`
					ClosedAt     string `json:"closedAt"`
					Additions    int64  `json:"additions"`
					Deletions    int64  `json:"deletions"`
					ChangedFiles int64  `json:"changedFiles"`
					Author       struct {
						Login string `json:"login"`
					} `json:"author"`
					Comments struct {
						TotalCount int64 `json:"totalCount"`
					} `json:"comments"`
					Reviews struct {
						Nodes []struct {
							SubmittedAt string `json:"submittedAt"`
						} `json:"nodes"`
					} `json:"reviews"`
				} `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
		RateLimit RateLimit `json:"rateLimit"`
	}
	vars := map[string]any{"owner": owner, "name": name}
	if after != "" {
		vars["after"] = after
	}
	if err := c.graphql(ctx, pullRequestsQuery, vars, &data); err != nil {
		return nil, err
	}
	prs := data.Repository.PullRequests
	page := &PRPage{EndCursor: prs.PageInfo.EndCursor, HasNextPage: prs.PageInfo.HasNextPage}
	for _, n := range prs.Nodes {
		var firstReview *time.Time
		if len(n.Reviews.Nodes) > 0 {
			firstReview = parseTimePtr(n.Reviews.Nodes[0].SubmittedAt)
		}
		login := n.Author.Login
		page.PRs = append(page.PRs, store.PullRequest{
			Number:        n.Number,
			AuthorLogin:   login,
			State:         n.State,
			CreatedAt:     parseTime(n.CreatedAt),
			MergedAt:      parseTimePtr(n.MergedAt),
			ClosedAt:      parseTimePtr(n.ClosedAt),
			Additions:     n.Additions,
			Deletions:     n.Deletions,
			ChangedFiles:  n.ChangedFiles,
			CommentsCount: n.Comments.TotalCount,
			FirstReviewAt: firstReview,
			IsBot:         IsBot(login),
			Title:         n.Title,
		})
	}
	return page, nil
}

// --- Issues ----------------------------------------------------------------

// IssuePage is one page of issues.
type IssuePage struct {
	Issues      []store.Issue
	EndCursor   string
	HasNextPage bool
}

const issuesQuery = `
query($owner:String!, $name:String!, $after:String) {
  repository(owner:$owner, name:$name) {
    issues(first:100, after:$after, orderBy:{field:CREATED_AT, direction:ASC}) {
      pageInfo { endCursor hasNextPage }
      nodes {
        number
        state
        title
        createdAt
        closedAt
        author { login }
        comments { totalCount }
      }
    }
  }
  rateLimit { cost remaining resetAt }
}`

func (c *Client) FetchIssues(ctx context.Context, owner, name, after string) (*IssuePage, error) {
	var data struct {
		Repository struct {
			Issues struct {
				PageInfo pageInfo `json:"pageInfo"`
				Nodes    []struct {
					Number    int64  `json:"number"`
					State     string `json:"state"`
					Title     string `json:"title"`
					CreatedAt string `json:"createdAt"`
					ClosedAt  string `json:"closedAt"`
					Author    struct {
						Login string `json:"login"`
					} `json:"author"`
					Comments struct {
						TotalCount int64 `json:"totalCount"`
					} `json:"comments"`
				} `json:"nodes"`
			} `json:"issues"`
		} `json:"repository"`
		RateLimit RateLimit `json:"rateLimit"`
	}
	vars := map[string]any{"owner": owner, "name": name}
	if after != "" {
		vars["after"] = after
	}
	if err := c.graphql(ctx, issuesQuery, vars, &data); err != nil {
		return nil, err
	}
	iss := data.Repository.Issues
	page := &IssuePage{EndCursor: iss.PageInfo.EndCursor, HasNextPage: iss.PageInfo.HasNextPage}
	for _, n := range iss.Nodes {
		login := n.Author.Login
		page.Issues = append(page.Issues, store.Issue{
			Number:        n.Number,
			AuthorLogin:   login,
			State:         n.State,
			CreatedAt:     parseTime(n.CreatedAt),
			ClosedAt:      parseTimePtr(n.ClosedAt),
			CommentsCount: n.Comments.TotalCount,
			IsBot:         IsBot(login),
			Title:         n.Title,
		})
	}
	return page, nil
}

// --- Releases --------------------------------------------------------------

// ReleasePage is one page of releases.
type ReleasePage struct {
	Releases    []store.Release
	EndCursor   string
	HasNextPage bool
}

const releasesQuery = `
query($owner:String!, $name:String!, $after:String) {
  repository(owner:$owner, name:$name) {
    releases(first:100, after:$after, orderBy:{field:CREATED_AT, direction:ASC}) {
      pageInfo { endCursor hasNextPage }
      nodes {
        tagName
        name
        publishedAt
        author { login }
      }
    }
  }
  rateLimit { cost remaining resetAt }
}`

func (c *Client) FetchReleases(ctx context.Context, owner, name, after string) (*ReleasePage, error) {
	var data struct {
		Repository struct {
			Releases struct {
				PageInfo pageInfo `json:"pageInfo"`
				Nodes    []struct {
					TagName     string `json:"tagName"`
					Name        string `json:"name"`
					PublishedAt string `json:"publishedAt"`
					Author      *struct {
						Login string `json:"login"`
					} `json:"author"`
				} `json:"nodes"`
			} `json:"releases"`
		} `json:"repository"`
		RateLimit RateLimit `json:"rateLimit"`
	}
	vars := map[string]any{"owner": owner, "name": name}
	if after != "" {
		vars["after"] = after
	}
	if err := c.graphql(ctx, releasesQuery, vars, &data); err != nil {
		return nil, err
	}
	rel := data.Repository.Releases
	page := &ReleasePage{EndCursor: rel.PageInfo.EndCursor, HasNextPage: rel.PageInfo.HasNextPage}
	for _, n := range rel.Nodes {
		login := ""
		if n.Author != nil {
			login = n.Author.Login
		}
		page.Releases = append(page.Releases, store.Release{
			Tag:         n.TagName,
			Name:        n.Name,
			PublishedAt: parseTimePtr(n.PublishedAt),
			AuthorLogin: login,
		})
	}
	return page, nil
}

// pageInfo is the GraphQL pagination cursor block.
type pageInfo struct {
	EndCursor   string `json:"endCursor"`
	HasNextPage bool   `json:"hasNextPage"`
}

// --- Viewer repositories (for the "track repo" picker) ---------------------

// ViewerRepo is one of the signed-in user's accessible repositories.
type ViewerRepo struct {
	NameWithOwner string `json:"nameWithOwner"`
	IsPrivate     bool   `json:"isPrivate"`
	Description   string `json:"description"`
}

const listViewerReposQuery = `
query($after: String) {
  viewer {
    repositories(first: 100, after: $after,
      orderBy: {field: PUSHED_AT, direction: DESC},
      affiliations: [OWNER, COLLABORATOR, ORGANIZATION_MEMBER]) {
      pageInfo { endCursor hasNextPage }
      nodes { nameWithOwner isPrivate description }
    }
  }
  rateLimit { cost remaining resetAt }
}`

// maxViewerRepoPages bounds how many pages of the viewer's repos we page through
// (100 per page, most-recently-pushed first) so a user with thousands of repos
// doesn't stall the picker. The manual owner/name input remains a fallback.
const maxViewerRepoPages = 5

// ListViewerRepos returns the signed-in user's accessible repositories, newest
// push first, paging up to maxViewerRepoPages.
func (c *Client) ListViewerRepos(ctx context.Context) ([]ViewerRepo, error) {
	var all []ViewerRepo
	after := ""
	for page := 0; page < maxViewerRepoPages; page++ {
		var data struct {
			Viewer struct {
				Repositories struct {
					PageInfo pageInfo     `json:"pageInfo"`
					Nodes    []ViewerRepo `json:"nodes"`
				} `json:"repositories"`
			} `json:"viewer"`
			RateLimit RateLimit `json:"rateLimit"`
		}
		vars := map[string]any{}
		if after != "" {
			vars["after"] = after
		}
		if err := c.graphql(ctx, listViewerReposQuery, vars, &data); err != nil {
			return nil, err
		}
		all = append(all, data.Viewer.Repositories.Nodes...)
		if !data.Viewer.Repositories.PageInfo.HasNextPage {
			break
		}
		after = data.Viewer.Repositories.PageInfo.EndCursor
	}
	return all, nil
}
