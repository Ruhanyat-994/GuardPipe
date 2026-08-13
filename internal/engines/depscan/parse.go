package depscan

// parseResult is what each ecosystem's parser reports — enough to build
// both the `dependencies` inventory and the depscan.hygiene.no-lockfile
// rule, which needs to know "a manifest exists but its lockfile doesn't,"
// not just the dependency list itself.
type parseResult struct {
	Dependencies  []Dependency
	ManifestFound bool
	ManifestPath  string // relative to the workspace root, for the no-lockfile finding's Location
	LockfileFound bool
}

// isWildcardRange reports whether a declared version range is
// effectively unpinned — depscan.hygiene.wildcard-version's exact
// condition (documentation/05-module-specifications.md's Core rules
// table: "Dependency pinned to `*` or `latest`").
func isWildcardRange(r string) bool {
	switch r {
	case "*", "latest", "x", "X":
		return true
	default:
		return false
	}
}
