package auth

import (
	"sort"
	"strings"
)

// scopeSet parses a comma- or space-separated scope string into a set. When
// expandImplications is true it expands the `repo` => `public_repo` implication
// GitHub applies. The expansion is only meaningful for the GRANTED set (holding
// `repo` implies you hold `public_repo`); expanding the requested set would
// fabricate phantom missing scopes.
func scopeSet(s string, expandImplications bool) map[string]struct{} {
	out := map[string]struct{}{}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		out[f] = struct{}{}
		if expandImplications && f == "repo" {
			out["public_repo"] = struct{}{}
		}
	}
	return out
}

// MissingScopes returns the requested scopes not present in the granted set
// (sorted; nil when all are satisfied).
func MissingScopes(requested, granted string) []string {
	req := scopeSet(requested, false)
	got := scopeSet(granted, true)
	var missing []string
	for s := range req {
		if _, ok := got[s]; !ok {
			missing = append(missing, s)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return missing
}
