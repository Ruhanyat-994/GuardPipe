package depscan

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

type packageJSON struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

type packageLockJSON struct {
	LockfileVersion int                           `json:"lockfileVersion"`
	Packages        map[string]packageLockPackage `json:"packages"`
}

type packageLockPackage struct {
	Version string `json:"version"`
}

// parseNPM handles package.json (direct dependencies + declared ranges)
// and, when present, package-lock.json's v2/v3 "packages" format (exact
// resolved versions, direct and transitive alike —
// documentation/05-module-specifications.md's manifest-parsers table).
// lockfileVersion 1 and yarn.lock/pnpm-lock.yaml are a documented gap, not
// silently mishandled: falls back to package.json's direct-only view
// rather than misparsing a format this function doesn't understand.
func parseNPM(workspaceDir string) (parseResult, error) {
	manifestPath := filepath.Join(workspaceDir, "package.json")
	pkgBytes, err := os.ReadFile(manifestPath)
	if os.IsNotExist(err) {
		return parseResult{}, nil
	}
	if err != nil {
		return parseResult{}, fmt.Errorf("depscan: read package.json: %w", err)
	}

	var pkg packageJSON
	if err := json.Unmarshal(pkgBytes, &pkg); err != nil {
		return parseResult{}, fmt.Errorf("depscan: parse package.json: %w", err)
	}

	direct := make(map[string]string, len(pkg.Dependencies)+len(pkg.DevDependencies))
	maps.Copy(direct, pkg.Dependencies)
	maps.Copy(direct, pkg.DevDependencies)

	result := parseResult{ManifestFound: true, ManifestPath: "package.json"}

	lockBytes, err := os.ReadFile(filepath.Join(workspaceDir, "package-lock.json"))
	if err != nil {
		if !os.IsNotExist(err) {
			return parseResult{}, fmt.Errorf("depscan: read package-lock.json: %w", err)
		}
		result.Dependencies = directOnlyNPM(direct)
		return result, nil
	}

	var lock packageLockJSON
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		return parseResult{}, fmt.Errorf("depscan: parse package-lock.json: %w", err)
	}
	if lock.LockfileVersion < 2 {
		result.Dependencies = directOnlyNPM(direct)
		return result, nil
	}
	result.LockfileFound = true

	deps := make([]Dependency, 0, len(lock.Packages))
	for pkgKey, entry := range lock.Packages {
		if pkgKey == "" || entry.Version == "" {
			continue // "" is the root package entry itself, not a dependency
		}
		name := npmPackageName(pkgKey)
		_, isDirect := direct[name]
		deps = append(deps, Dependency{
			Ecosystem: "npm", Name: name, Version: entry.Version,
			IsDirect: isDirect, ManifestPath: "package-lock.json", DeclaredRange: directRangeOrEmpty(isDirect, name, pkg),
		})
	}
	result.Dependencies = deps
	return result, nil
}

func directOnlyNPM(direct map[string]string) []Dependency {
	out := make([]Dependency, 0, len(direct))
	for name, r := range direct {
		out = append(out, Dependency{
			Ecosystem: "npm", Name: name, Version: strings.TrimLeft(r, "^~>=< "),
			IsDirect: true, ManifestPath: "package.json", DeclaredRange: r,
		})
	}
	return out
}

// npmPackageName extracts a package name from a package-lock.json v2/v3
// "packages" key, which is a full node_modules path for nested/transitive
// dependencies (e.g. "node_modules/foo/node_modules/bar" -> "bar").
func npmPackageName(pkgKey string) string {
	if idx := strings.LastIndex(pkgKey, "node_modules/"); idx >= 0 {
		return pkgKey[idx+len("node_modules/"):]
	}
	return pkgKey
}

func directRangeOrEmpty(isDirect bool, name string, pkg packageJSON) string {
	if !isDirect {
		return ""
	}
	if r, ok := pkg.Dependencies[name]; ok {
		return r
	}
	return pkg.DevDependencies[name]
}
