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

func TestIsBotSuffix(t *testing.T) {
	if !IsBot("renovate[bot]") {
		t.Fatal("expected [bot] suffix to be detected")
	}
	if IsBot("realhuman") {
		t.Fatal("did not expect realhuman to be a bot")
	}
}

func TestIsBotKnownList(t *testing.T) {
	// Known bots without the [bot] suffix (case-insensitive).
	for _, login := range []string{"dependabot", "Dependabot", "renovate", "github-actions", "snyk-bot"} {
		if !IsBot(login) {
			t.Errorf("expected %q to be a known bot", login)
		}
	}
}

func TestAddKnownBots(t *testing.T) {
	if IsBot("acme-ci") {
		t.Fatal("acme-ci should not be a bot before registration")
	}
	AddKnownBots("acme-ci", "Other-Bot")
	if !IsBot("acme-ci") || !IsBot("other-bot") {
		t.Fatal("AddKnownBots did not register the new bots (case-insensitive)")
	}
}
