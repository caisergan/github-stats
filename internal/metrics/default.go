package metrics

// DefaultRegistry returns a Registry with every shipped Extended metric (spec §7).
// Adding a stat is one new file + one Register line here.
func DefaultRegistry() *Registry {
	reg := NewRegistry()
	reg.Register(commitRate{})
	reg.Register(prThroughput{})
	reg.Register(timeToMerge{})
	reg.Register(reviewLatency{})
	reg.Register(issueLifetime{})
	reg.Register(openIssueAge{})
	reg.Register(codeChurn{})
	reg.Register(commentVolume{})
	reg.Register(contributorLeaderboard{})
	return reg
}
