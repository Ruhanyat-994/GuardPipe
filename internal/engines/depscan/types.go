// Package depscan is GuardPipe's own dependency-inventory, secret-sweep,
// and advisory-lookup engine — the vertical slice BUILD_GUIDE.md Phase 6
// builds first because it proves the whole architecture (ingest → analyse
// → normalise → persist) end to end before six more engines are built on
// top of it (documentation/05-module-specifications.md §4).
package depscan

// Dependency is one manifest entry, the engine's own inventory shape —
// richer than advisory.Dependency (which only carries what an OSV lookup
// needs) since this one also has to become a `dependencies` table row
// (documentation/06-database-design.md §4.16).
type Dependency struct {
	Ecosystem     string // "npm" | "pypi" | "go" | "maven" | "composer"
	Name          string
	Version       string
	IsDirect      bool
	ManifestPath  string
	DeclaredRange string // the raw version range as written in the manifest, e.g. "^4.17.0"
}
