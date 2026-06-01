package githubapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github-stats/internal/store"
)

// Options configures a Client. URLs are injectable so tests can point at
// httptest servers (spec/design contract).
type Options struct {
	Token       string
	GraphQLURL  string // e.g. https://api.github.com/graphql
	RESTBaseURL string // e.g. https://api.github.com
	Store       *store.Store
	HTTP        *http.Client // optional; one is built (with ETag transport) if nil
}

// Client is a rate-limit-aware GitHub API client (GraphQL + conditional REST).
type Client struct {
	token       string
	graphqlURL  string
	restBaseURL string
	http        *http.Client
	Budget      *Budget
}

// NewClient builds a Client. If Options.HTTP is nil, an http.Client whose
// transport is an ETagTransport (over the store) is created so REST GETs are
// conditional by default.
func NewClient(o Options) *Client {
	httpClient := o.HTTP
	if httpClient == nil {
		httpClient = &http.Client{
			Transport: &ETagTransport{Store: o.Store, Base: http.DefaultTransport},
		}
	}
	return &Client{
		token:       o.Token,
		graphqlURL:  o.GraphQLURL,
		restBaseURL: strings.TrimRight(o.RESTBaseURL, "/"),
		http:        httpClient,
		Budget:      NewBudget(),
	}
}

// restGET performs a GET against the REST base URL and returns the body bytes.
// Routed through the client's transport (ETag-conditional when wired that way).
func (c *Client) restGET(ctx context.Context, path string) ([]byte, int, error) {
	// Pre-flight: refuse if the REST bucket is drained, mirroring graphql().
	if c.Budget.RESTExhausted() {
		_, reset := c.Budget.REST()
		return nil, 0, &RateLimitError{Resource: "rest", Reset: reset}
	}
	url := c.restBaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	c.Budget.UpdateFromRESTHeaders(resp.Header)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode != http.StatusOK {
		return body, resp.StatusCode, fmt.Errorf("REST GET %s: status %d", path, resp.StatusCode)
	}
	return body, resp.StatusCode, nil
}
