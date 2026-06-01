package githubapi

import "strings"

// knownBots is a small allowlist of bot logins that do not carry the "[bot]"
// suffix in every context (e.g. the GraphQL author login can drop it).
var knownBots = map[string]bool{
	"dependabot":     true,
	"renovate":       true,
	"renovate-bot":   true,
	"github-actions": true,
	"web-flow":       true, // GitHub's synthetic merge-commit author
	"imgbot":         true,
	"codecov":        true,
	"mergify":        true,
}

// IsBot reports whether a login belongs to a bot. True when the login ends in
// "[bot]" (case-insensitive) or matches the known-bot allowlist. Used to set the
// is_bot flag at ingest so the dashboard's exclude-bots toggle works (spec §7).
func IsBot(login string) bool {
	if login == "" {
		return false
	}
	if strings.HasSuffix(strings.ToLower(login), "[bot]") {
		return true
	}
	return knownBots[strings.ToLower(login)]
}
