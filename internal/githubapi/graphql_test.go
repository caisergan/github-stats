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

func newTestClient(t *testing.T, gqlURL, restURL string) *Client {
	t.Helper()
	st := openTestStore(t)
	return NewClient(Options{
		Token:       "gho_test",
		GraphQLURL:  gqlURL,
		RESTBaseURL: restURL,
		Store:       st,
		HTTP:        &http.Client{},
	})
}

func TestGraphQLDecodesDataAndRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gho_test" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.Unmarshal(body, &req)
		if !strings.Contains(req.Query, "rateLimit") {
			t.Errorf("query missing rateLimit: %s", req.Query)
		}
		if req.Variables["owner"] != "octocat" {
			t.Errorf("variables = %v", req.Variables)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"thing": {"name": "hello"},
				"rateLimit": {"cost": 1, "remaining": 4998, "resetAt": "2026-04-01T13:00:00Z"}
			}
		}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, "http://unused")

	var data struct {
		Thing struct {
			Name string `json:"name"`
		} `json:"thing"`
		RateLimit RateLimit `json:"rateLimit"`
	}
	err := c.graphql(context.Background(),
		`query($owner:String!){ thing rateLimit { cost remaining resetAt } }`,
		map[string]any{"owner": "octocat"}, &data)
	if err != nil {
		t.Fatal(err)
	}
	if data.Thing.Name != "hello" {
		t.Fatalf("decoded name = %q", data.Thing.Name)
	}
	rem, _ := c.Budget.GraphQL()
	if rem != 4998 {
		t.Fatalf("budget not updated from rateLimit: remaining=%d", rem)
	}
}

func TestGraphQLSurfacesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"Could not resolve to a Repository"}]}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, "http://unused")
	var data struct{}
	err := c.graphql(context.Background(), `query{x}`, nil, &data)
	if err == nil || !strings.Contains(err.Error(), "Could not resolve") {
		t.Fatalf("expected GraphQL error surfaced, got %v", err)
	}
}
