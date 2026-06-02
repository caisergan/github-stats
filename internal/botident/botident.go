// Package botident classifies whether a GitHub login belongs to a bot. It is the
// single source of truth shared by ingest (githubapi, which stamps the is_bot
// flag) and the metrics layer (which filters bot logins), so the exclude-bots
// behaviour is identical everywhere — a login flagged is_bot at ingest is the
// same login the leaderboard drops.
package botident

import "strings"

// knownBots is a small allowlist of bot logins that do not carry the "[bot]"
// suffix in every context (e.g. the GraphQL author login can drop it, and
// "web-flow" is GitHub's synthetic merge-commit author).
var knownBots = map[string]bool{
	"dependabot":     true,
	"renovate":       true,
	"renovate-bot":   true,
	"github-actions": true,
	"web-flow":       true,
	"imgbot":         true,
	"codecov":        true,
	"mergify":        true,
}

// IsBot reports whether login belongs to a bot: true when the login ends in
// "[bot]" (case-insensitive) or matches the known-bot allowlist.
func IsBot(login string) bool {
	if login == "" {
		return false
	}
	l := strings.ToLower(login)
	if strings.HasSuffix(l, "[bot]") {
		return true
	}
	return knownBots[l]
}
