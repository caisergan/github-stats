package githubapi

import "github-stats/internal/botident"

// IsBot reports whether a login belongs to a bot. It delegates to the shared
// botident package so ingest (which stamps the is_bot flag here) and the metrics
// layer classify bots identically. Used to set is_bot at ingest so the
// dashboard's exclude-bots toggle works (spec §7).
func IsBot(login string) bool { return botident.IsBot(login) }
