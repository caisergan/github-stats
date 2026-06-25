package api

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestParsePackageJSON(t *testing.T) {
	input := []byte(`{
		"name": "demo",
		"dependencies": {
			"@octocat/widget": "^1.0.0",
			"react": "^18.0.0",
			"left-pad": "git+https://github.com/stevemao/left-pad.git",
			"vendored": "github:acme/vendored#v2"
		},
		"devDependencies": {
			"@types/node": "^20.0.0"
		}
	}`)
	res := ParsePackageJSON(input)
	wantResolved := []string{"acme/vendored", "octocat/widget", "stevemao/left-pad", "types/node"}
	got := append([]string(nil), res.Resolved...)
	sort.Strings(got)
	if !reflect.DeepEqual(got, wantResolved) {
		t.Fatalf("resolved = %v, want %v", got, wantResolved)
	}
	wantUnresolved := []string{"react"}
	if !reflect.DeepEqual(res.Unresolved, wantUnresolved) {
		t.Fatalf("unresolved = %v, want %v", res.Unresolved, wantUnresolved)
	}
}

func TestParsePackageJSONInvalid(t *testing.T) {
	res := ParsePackageJSON([]byte("not json"))
	if len(res.Resolved) != 0 || len(res.Unresolved) != 0 {
		t.Fatalf("expected empty result for invalid json, got %+v", res)
	}
}

func TestParseRequirementsTxt(t *testing.T) {
	input := []byte(strings.TrimSpace(`
# a comment
requests==2.31.0
Flask>=2.0   # inline comment
-e git+https://github.com/psf/black.git@main#egg=black
git+https://github.com/pallets/click
numpy

`))
	res := ParseRequirementsTxt(input)
	wantResolved := []string{"pallets/click", "psf/black"}
	got := append([]string(nil), res.Resolved...)
	sort.Strings(got)
	if !reflect.DeepEqual(got, wantResolved) {
		t.Fatalf("resolved = %v, want %v", got, wantResolved)
	}
	wantUnresolved := []string{"Flask", "numpy", "requests"}
	gotU := append([]string(nil), res.Unresolved...)
	sort.Strings(gotU)
	if !reflect.DeepEqual(gotU, wantUnresolved) {
		t.Fatalf("unresolved = %v, want %v", gotU, wantUnresolved)
	}
}
