package depscan

import (
	"os"
	"path/filepath"
)

// parseResult is what each ecosystem's parser reports — enough to build
// both the `dependencies` inventory and the depscan.hygiene.no-lockfile
// rule, which needs to know "a manifest exists but its lockfile doesn't,"
// not just the dependency list itself. One parseResult per manifest
// *instance* found — a monorepo with backend/go.mod and a services/x/go.mod
// produces two, not one merged result, so depscan.hygiene.no-lockfile can
// point at the specific directory that's missing its lockfile.
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

// findManifestDirs walks the whole workspace (skipping the same noise
// directories the secret sweep skips — dependency trees, VCS internals,
// build output) and returns every directory that directly contains
// filename. Manifests live wherever a project's build layout puts them —
// backend/go.mod and frontend/package.json in the same checkout, for
// instance — not only at the workspace root, so this is a full recursive
// search rather than the single os.Stat(workspaceDir, filename) call this
// package started with.
func findManifestDirs(workspaceDir, filename string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(workspaceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != workspaceDir && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == filename {
			dirs = append(dirs, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dirs, nil
}

// relManifestPath renders path relative to workspaceDir with forward
// slashes, for Dependency.ManifestPath / parseResult.ManifestPath — those
// are used as Finding.Location.Path and in suppression-comment matching,
// so they need to read as "backend/go.mod", not an absolute filesystem
// path.
func relManifestPath(workspaceDir, path string) string {
	rel, err := filepath.Rel(workspaceDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
