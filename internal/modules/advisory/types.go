// Package advisory is the shared "advisory data" capability named in
// BUILD_GUIDE.md Phase 5: dependency-vulnerability lookups against OSV.dev
// (documentation/05-module-specifications.md's depscan "Advisory lookup"
// sequence diagram) and the rules catalogue (documentation/06-database-design.md
// §4.15, documentation/07-api-specification.md §8). Both are reference data
// no particular scan owns, which is why Phase 5 groups them under one
// module rather than splitting them across two.
package advisory

import "strings"

// Dependency identifies one package version to check for advisories —
// depscan's inventory entry, without the scan-specific fields (manifest
// path, direct/transitive) that don't matter to a lookup.
type Dependency struct {
	Ecosystem string // e.g. "npm", "PyPI", "Go", "Maven", "Packagist"
	Name      string
	Version   string
}

// Advisory is a normalised vulnerability record, translated from OSV.dev's
// wire schema so depscan (Phase 6) never needs to know OSV.dev exists —
// only this package and adapters/osv do.
type Advisory struct {
	ID           string
	CVE          string // first CVE-prefixed alias, if OSV.dev has one
	Summary      string
	CVSSVector   string // best-available CVSS score string, empty if OSV.dev has none
	HasFix       bool
	FixedVersion string
	References   []string
}

// Result is the per-dependency outcome of a Lookup call.
type Result struct {
	Dependency Dependency
	Advisories []Advisory
	// Unavailable is true when OSV.dev could not be reached for this
	// dependency — the inventory entry is still valid, only advisory data is
	// missing (documentation/05-module-specifications.md depscan "Failure
	// modes": "OSV unreachable: Inventory kept, advisory_data_unavailable
	// flag on the result, job succeeds", FR-DEP-011).
	Unavailable bool
}

// cacheKey matches documentation/06-database-design.md §11's Redis key
// table exactly: gp:cache:osv:{eco}:{pkg}:{ver}.
func cacheKey(d Dependency) string {
	return "gp:cache:osv:" + strings.ToLower(d.Ecosystem) + ":" + d.Name + ":" + d.Version
}
