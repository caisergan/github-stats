package api

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// ImportResult is the outcome of parsing a dependency manifest: repos we could
// resolve unambiguously to owner/repo, and names we could not (the UI asks the user).
type ImportResult struct {
	Resolved   []string `json:"resolved"`   // owner/repo, deduped + sorted
	Unresolved []string `json:"unresolved"` // raw package names, deduped + sorted
}

// githubURLRe captures owner/repo from a github URL or shorthand inside a version spec.
var githubURLRe = regexp.MustCompile(
	`(?:github\.com[:/]|github:)([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)`,
)

// resolveFromSpec returns owner/repo if the version spec embeds a github reference.
func resolveFromSpec(spec string) (string, bool) {
	m := githubURLRe.FindStringSubmatch(spec)
	if m == nil {
		return "", false
	}
	owner, repo := m[1], strings.TrimSuffix(m[2], ".git")
	repo = strings.TrimSuffix(repo, ".git")
	return owner + "/" + repo, true
}

// resolveFromName handles scoped npm names (@scope/pkg -> scope/pkg).
func resolveFromName(name string) (string, bool) {
	if strings.HasPrefix(name, "@") && strings.Contains(name, "/") {
		return strings.TrimPrefix(name, "@"), true
	}
	return "", false
}

func finalize(resolved, unresolved map[string]struct{}) ImportResult {
	r := keys(resolved)
	u := keys(unresolved)
	sort.Strings(r)
	sort.Strings(u)
	return ImportResult{Resolved: r, Unresolved: u}
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ParsePackageJSON parses an npm package.json's dependency maps.
func ParsePackageJSON(data []byte) ImportResult {
	var pkg struct {
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		PeerDependencies     map[string]string `json:"peerDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
	}
	resolved := map[string]struct{}{}
	unresolved := map[string]struct{}{}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return finalize(resolved, unresolved)
	}
	for _, deps := range []map[string]string{
		pkg.Dependencies, pkg.DevDependencies, pkg.PeerDependencies, pkg.OptionalDependencies,
	} {
		for name, spec := range deps {
			if rr, ok := resolveFromSpec(spec); ok {
				resolved[rr] = struct{}{}
				continue
			}
			if rr, ok := resolveFromName(name); ok {
				resolved[rr] = struct{}{}
				continue
			}
			unresolved[name] = struct{}{}
		}
	}
	return finalize(resolved, unresolved)
}

// reqNameRe captures the package name at the start of a requirements.txt line.
var reqNameRe = regexp.MustCompile(`^([A-Za-z0-9_.-]+)`)

// ParseRequirementsTxt parses a pip requirements.txt.
func ParseRequirementsTxt(data []byte) ImportResult {
	resolved := map[string]struct{}{}
	unresolved := map[string]struct{}{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip an inline comment.
		if i := strings.Index(line, " #"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		// VCS/editable installs: -e git+https://github.com/owner/repo...
		if strings.Contains(line, "github.com") {
			if rr, ok := resolveFromSpec(line); ok {
				resolved[rr] = struct{}{}
				continue
			}
		}
		// Skip flags/editable markers that are not package names.
		if strings.HasPrefix(line, "-") {
			continue
		}
		// Plain requirement: NAME[==|>=|<=|~=|!=|<|>|;|[]...
		if m := reqNameRe.FindStringSubmatch(line); m != nil {
			unresolved[m[1]] = struct{}{}
		}
	}
	return finalize(resolved, unresolved)
}
