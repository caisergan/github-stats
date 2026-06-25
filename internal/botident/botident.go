// Package botident classifies whether a GitHub login belongs to a bot. It is the
// single source of truth shared by ingest (githubapi, which stamps the is_bot
// flag) and the metrics layer (which filters bot logins), so the exclude-bots
// behaviour is identical everywhere — a login flagged is_bot at ingest is the
// same login the leaderboard drops.
package botident

import (
	"strings"
	"sync"
)

// knownBots is the configurable allowlist of bot logins that do not carry the
// "[bot]" suffix in every context (e.g. the GraphQL author login can drop it,
// and "web-flow" is GitHub's synthetic merge-commit author). It is guarded by
// knownBotsMu so AddKnownBots (writer) and IsBot (reader) are safe for
// concurrent use. Self-hosters extend it via AddKnownBots.
var (
	knownBotsMu sync.RWMutex
	knownBots   = map[string]struct{}{
		"dependabot":           {},
		"dependabot-preview":   {},
		"renovate":             {},
		"renovate-bot":         {},
		"github-actions":       {},
		"greenkeeper":          {},
		"snyk-bot":             {},
		"mergify":              {},
		"codecov":              {},
		"imgbot":               {},
		"allcontributors":      {},
		"semantic-release-bot": {},
		"web-flow":             {},
	}
)

// IsBot reports whether login belongs to a bot: true when the login ends in
// "[bot]" (case-insensitive) or matches the configurable known-bot set.
func IsBot(login string) bool {
	l := strings.ToLower(strings.TrimSpace(login))
	if l == "" {
		return false
	}
	if strings.HasSuffix(l, "[bot]") {
		return true
	}
	knownBotsMu.RLock()
	_, ok := knownBots[l]
	knownBotsMu.RUnlock()
	return ok
}

// AddKnownBots registers additional known-bot logins (case-insensitive). It is
// safe for concurrent use and lets self-hosters extend bot detection. Blank or
// whitespace-only logins are ignored.
func AddKnownBots(logins ...string) {
	knownBotsMu.Lock()
	defer knownBotsMu.Unlock()
	for _, l := range logins {
		l = strings.ToLower(strings.TrimSpace(l))
		if l != "" {
			knownBots[l] = struct{}{}
		}
	}
}
