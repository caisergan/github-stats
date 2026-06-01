package githubapi

import (
	"context"
	"time"

	"github-stats/internal/store"
)

// --- Commits since ---------------------------------------------------------

const commitsSinceQuery = `
query($owner:String!, $name:String!, $branch:String!, $since:GitTimestamp!, $after:String) {
  repository(owner:$owner, name:$name) {
    ref(qualifiedName:$branch) {
      target {
        ... on Commit {
          history(first:100, after:$after, since:$since) {
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

// FetchCommitsSince pages commit history restricted to commits at or after
// `since` (delta sync). It reuses CommitPage; commits have no "updated" notion.
func (c *Client) FetchCommitsSince(ctx context.Context, owner, name, branch string, since time.Time, after string) (*CommitPage, error) {
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
	vars := map[string]any{
		"owner":  owner,
		"name":   name,
		"branch": "refs/heads/" + branch,
		"since":  since.UTC().Format(time.RFC3339),
	}
	if after != "" {
		vars["after"] = after
	}
	if err := c.graphql(ctx, commitsSinceQuery, vars, &data); err != nil {
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

// --- Pull requests updated -------------------------------------------------

// UpdatedPR pairs a store.PullRequest with its updatedAt so callers can stop
// paging once items predate the overlap window.
type UpdatedPR struct {
	PullRequest store.PullRequest
	UpdatedAt   time.Time
}

// UpdatedPRPage is one page of pull requests ordered by UPDATED_AT DESC.
type UpdatedPRPage struct {
	PRs         []UpdatedPR
	EndCursor   string
	HasNextPage bool
}

const pullRequestsUpdatedQuery = `
query($owner:String!, $name:String!, $after:String) {
  repository(owner:$owner, name:$name) {
    pullRequests(first:100, after:$after, orderBy:{field:UPDATED_AT, direction:DESC}) {
      pageInfo { endCursor hasNextPage }
      nodes {
        number
        state
        title
        createdAt
        updatedAt
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

// FetchPullRequestsUpdated pages PRs newest-updated first. The caller stops once
// UpdatedAt falls before its overlap-adjusted cutoff.
func (c *Client) FetchPullRequestsUpdated(ctx context.Context, owner, name, after string) (*UpdatedPRPage, error) {
	var data struct {
		Repository struct {
			PullRequests struct {
				PageInfo pageInfo `json:"pageInfo"`
				Nodes    []struct {
					Number       int64  `json:"number"`
					State        string `json:"state"`
					Title        string `json:"title"`
					CreatedAt    string `json:"createdAt"`
					UpdatedAt    string `json:"updatedAt"`
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
	if err := c.graphql(ctx, pullRequestsUpdatedQuery, vars, &data); err != nil {
		return nil, err
	}
	prs := data.Repository.PullRequests
	page := &UpdatedPRPage{EndCursor: prs.PageInfo.EndCursor, HasNextPage: prs.PageInfo.HasNextPage}
	for _, n := range prs.Nodes {
		var firstReview *time.Time
		if len(n.Reviews.Nodes) > 0 {
			firstReview = parseTimePtr(n.Reviews.Nodes[0].SubmittedAt)
		}
		login := n.Author.Login
		page.PRs = append(page.PRs, UpdatedPR{
			PullRequest: store.PullRequest{
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
			},
			UpdatedAt: parseTime(n.UpdatedAt),
		})
	}
	return page, nil
}

// --- Issues updated --------------------------------------------------------

// UpdatedIssue pairs a store.Issue with its updatedAt.
type UpdatedIssue struct {
	Issue     store.Issue
	UpdatedAt time.Time
}

// UpdatedIssuePage is one page of issues ordered by UPDATED_AT DESC.
type UpdatedIssuePage struct {
	Issues      []UpdatedIssue
	EndCursor   string
	HasNextPage bool
}

const issuesUpdatedQuery = `
query($owner:String!, $name:String!, $after:String) {
  repository(owner:$owner, name:$name) {
    issues(first:100, after:$after, orderBy:{field:UPDATED_AT, direction:DESC}) {
      pageInfo { endCursor hasNextPage }
      nodes {
        number
        state
        title
        createdAt
        updatedAt
        closedAt
        author { login }
        comments { totalCount }
      }
    }
  }
  rateLimit { cost remaining resetAt }
}`

// FetchIssuesUpdated pages issues newest-updated first.
func (c *Client) FetchIssuesUpdated(ctx context.Context, owner, name, after string) (*UpdatedIssuePage, error) {
	var data struct {
		Repository struct {
			Issues struct {
				PageInfo pageInfo `json:"pageInfo"`
				Nodes    []struct {
					Number    int64  `json:"number"`
					State     string `json:"state"`
					Title     string `json:"title"`
					CreatedAt string `json:"createdAt"`
					UpdatedAt string `json:"updatedAt"`
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
	if err := c.graphql(ctx, issuesUpdatedQuery, vars, &data); err != nil {
		return nil, err
	}
	iss := data.Repository.Issues
	page := &UpdatedIssuePage{EndCursor: iss.PageInfo.EndCursor, HasNextPage: iss.PageInfo.HasNextPage}
	for _, n := range iss.Nodes {
		login := n.Author.Login
		page.Issues = append(page.Issues, UpdatedIssue{
			Issue: store.Issue{
				Number:        n.Number,
				AuthorLogin:   login,
				State:         n.State,
				CreatedAt:     parseTime(n.CreatedAt),
				ClosedAt:      parseTimePtr(n.ClosedAt),
				CommentsCount: n.Comments.TotalCount,
				IsBot:         IsBot(login),
				Title:         n.Title,
			},
			UpdatedAt: parseTime(n.UpdatedAt),
		})
	}
	return page, nil
}
