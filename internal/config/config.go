package config

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Config holds all runtime configuration.
type Config struct {
	Addr               string
	DatabasePath       string
	BaseURL            string
	GitHubClientID     string
	GitHubClientSecret string
	GitHubScopes       string
	EncryptionKey      []byte
	SessionTTL         time.Duration

	// Overridable for tests; default to the public GitHub endpoints.
	GitHubOAuthBaseURL string // https://github.com
	GitHubAPIBaseURL   string // https://api.github.com
}

// RedirectURL is the OAuth callback URL derived from BaseURL.
func (c Config) RedirectURL() string {
	return strings.TrimRight(c.BaseURL, "/") + "/auth/github/callback"
}

func get(getenv func(string) string, key, def string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return def
}

// Load builds Config from a getenv function (os.Getenv in production).
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Addr:               get(getenv, "ADDR", ":8080"),
		DatabasePath:       get(getenv, "DATABASE_PATH", "app.db"),
		BaseURL:            get(getenv, "BASE_URL", "http://localhost:8080"),
		GitHubClientID:     getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: getenv("GITHUB_CLIENT_SECRET"),
		// Read-only by design: the app only ever READS GitHub data, so the OAuth
		// token deliberately carries no write/delete scope. "read:user" covers
		// login + public repositories; private repos are reached via a separate,
		// user-supplied read-only PAT (see settings). This guarantees that even a
		// bug here can never modify or delete a repository on GitHub.
		GitHubScopes:       get(getenv, "GITHUB_SCOPES", "read:user"),
		SessionTTL:         30 * 24 * time.Hour,
		GitHubOAuthBaseURL: get(getenv, "GITHUB_OAUTH_BASE_URL", "https://github.com"),
		GitHubAPIBaseURL:   get(getenv, "GITHUB_API_BASE_URL", "https://api.github.com"),
	}
	if cfg.GitHubClientID == "" {
		return Config{}, fmt.Errorf("GITHUB_CLIENT_ID is required")
	}
	if cfg.GitHubClientSecret == "" {
		return Config{}, fmt.Errorf("GITHUB_CLIENT_SECRET is required")
	}
	key, err := hex.DecodeString(getenv("ENCRYPTION_KEY"))
	if err != nil {
		return Config{}, fmt.Errorf("ENCRYPTION_KEY must be hex: %w", err)
	}
	if len(key) != 32 {
		return Config{}, fmt.Errorf("ENCRYPTION_KEY must decode to 32 bytes, got %d", len(key))
	}
	cfg.EncryptionKey = key
	return cfg, nil
}
