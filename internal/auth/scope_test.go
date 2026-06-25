package auth

import (
	"reflect"
	"testing"
)

func TestMissingScopes(t *testing.T) {
	cases := []struct {
		requested, granted string
		want               []string
	}{
		{"read:user public_repo", "read:user public_repo", nil},
		{"read:user repo", "read:user", []string{"repo"}},
		{"repo read:org", "", []string{"read:org", "repo"}},
		// `repo` granted implies `public_repo`, so requesting public_repo is satisfied.
		{"public_repo", "repo", nil},
	}
	for _, c := range cases {
		got := MissingScopes(c.requested, c.granted)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("MissingScopes(%q,%q) = %v, want %v", c.requested, c.granted, got, c.want)
		}
	}
}
