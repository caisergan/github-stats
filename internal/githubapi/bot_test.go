package githubapi

import "testing"

func TestIsBot(t *testing.T) {
	cases := []struct {
		login string
		want  bool
	}{
		{"dependabot[bot]", true},
		{"renovate[bot]", true},
		{"github-actions[bot]", true},
		{"dependabot", true},   // known list, no suffix
		{"renovate", true},     // known list
		{"web-flow", true},     // GitHub's merge-commit author
		{"octocat", false},
		{"", false},
		{"Dependabot[Bot]", true}, // case-insensitive suffix
	}
	for _, c := range cases {
		if got := IsBot(c.login); got != c.want {
			t.Errorf("IsBot(%q) = %v, want %v", c.login, got, c.want)
		}
	}
}
