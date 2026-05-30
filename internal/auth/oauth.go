package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// OAuthClient performs the GitHub OAuth code exchange and user lookup.
// Base URLs are injectable so tests can point at httptest servers.
type OAuthClient struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	OAuthBaseURL string // e.g. https://github.com
	APIBaseURL   string // e.g. https://api.github.com
	HTTP         *http.Client
}

func (c *OAuthClient) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// GitHubUser is the subset of the GitHub user object we persist.
type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

// AuthorizeURL builds the URL to redirect the user to for consent.
func (c *OAuthClient) AuthorizeURL(state, scopes string) string {
	q := url.Values{}
	q.Set("client_id", c.ClientID)
	q.Set("redirect_uri", c.RedirectURL)
	q.Set("scope", scopes)
	q.Set("state", state)
	return strings.TrimRight(c.OAuthBaseURL, "/") + "/login/oauth/authorize?" + q.Encode()
}

// Exchange swaps an authorization code for an access token; returns token and granted scope.
func (c *OAuthClient) Exchange(ctx context.Context, code string) (token, scope string, err error) {
	form := url.Values{}
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", c.RedirectURL)

	endpoint := strings.TrimRight(c.OAuthBaseURL, "/") + "/login/oauth/access_token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("oauth exchange: status %d", resp.StatusCode)
	}
	var body struct {
		AccessToken string `json:"access_token"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", err
	}
	if body.Error != "" || body.AccessToken == "" {
		return "", "", fmt.Errorf("oauth exchange failed: %s", body.Error)
	}
	return body.AccessToken, body.Scope, nil
}

// GetUser fetches the authenticated user with the given token.
func (c *OAuthClient) GetUser(ctx context.Context, token string) (*GitHubUser, error) {
	endpoint := strings.TrimRight(c.APIBaseURL, "/") + "/user"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get user: status %d", resp.StatusCode)
	}
	var u GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}
