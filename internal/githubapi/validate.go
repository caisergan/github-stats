package githubapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// PATInfo is the validated identity behind a personal access token.
type PATInfo struct {
	Login  string
	Scopes string // raw X-OAuth-Scopes (informational; fine-grained PATs report "")
}

// ValidatePAT verifies a token by calling GET {restBaseURL}/user. A non-200
// response (e.g. 401) means the token is invalid and is returned as an error.
func ValidatePAT(ctx context.Context, hc *http.Client, restBaseURL, token string) (*PATInfo, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	endpoint := strings.TrimRight(restBaseURL, "/") + "/user"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("validate pat: status %d", resp.StatusCode)
	}
	var body struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return &PATInfo{Login: body.Login, Scopes: resp.Header.Get("X-OAuth-Scopes")}, nil
}

// HasRepoScope reports whether a classic token's X-OAuth-Scopes string grants the
// "repo" scope — the scope required to read PRIVATE repositories. The match is on
// the exact scope token (so "public_repo" does NOT count). Fine-grained tokens
// report an empty scope string; callers should treat "" as "unknown, can't tell"
// rather than "missing", since their access is per-repo and not in this header.
func HasRepoScope(scopes string) bool {
	for _, s := range strings.Split(scopes, ",") {
		if strings.TrimSpace(s) == "repo" {
			return true
		}
	}
	return false
}
