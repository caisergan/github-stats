package botident

import "testing"

func TestIsBotSuffix(t *testing.T) {
	if !IsBot("renovate[bot]") {
		t.Fatal("expected [bot] suffix to be detected")
	}
	if !IsBot("Dependabot[Bot]") {
		t.Fatal("expected [bot] suffix to be case-insensitive")
	}
	if IsBot("realhuman") {
		t.Fatal("did not expect realhuman to be a bot")
	}
	if IsBot("") {
		t.Fatal("empty login should not be a bot")
	}
}

func TestIsBotKnownList(t *testing.T) {
	// Default known bots (the expanded M6 set), case-insensitive, no suffix.
	for _, login := range []string{
		"dependabot", "Dependabot", "dependabot-preview",
		"renovate", "renovate-bot", "github-actions",
		"greenkeeper", "snyk-bot", "mergify", "codecov",
		"imgbot", "allcontributors", "semantic-release-bot",
		"web-flow", // GitHub's synthetic merge-commit author (carried from M2)
	} {
		if !IsBot(login) {
			t.Errorf("expected %q to be a known bot", login)
		}
	}
}

func TestAddKnownBots(t *testing.T) {
	if IsBot("acme-ci") {
		t.Fatal("acme-ci should not be a bot before registration")
	}
	AddKnownBots("acme-ci", "Other-Bot", "  ", "")
	if !IsBot("acme-ci") || !IsBot("other-bot") {
		t.Fatal("AddKnownBots did not register the new bots (case-insensitive)")
	}
	// Blank/whitespace logins must be ignored, never matching a blank login.
	if IsBot("") || IsBot("   ") {
		t.Fatal("blank logins must not be registered")
	}
}
