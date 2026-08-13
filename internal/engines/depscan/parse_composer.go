package depscan

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
)

type composerJSON struct {
	Require    map[string]string `json:"require"`
	RequireDev map[string]string `json:"require-dev"`
}

type composerLockJSON struct {
	Packages    []composerLockPackage `json:"packages"`
	PackagesDev []composerLockPackage `json:"packages-dev"`
}

type composerLockPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// parseComposer handles composer.json (direct requirements) and, when
// present, composer.lock (exact resolved versions —
// documentation/05-module-specifications.md's manifest-parsers table:
// "Lockfile preferred").
func parseComposer(workspaceDir string) (parseResult, error) {
	manifestPath := filepath.Join(workspaceDir, "composer.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if os.IsNotExist(err) {
		return parseResult{}, nil
	}
	if err != nil {
		return parseResult{}, fmt.Errorf("depscan: read composer.json: %w", err)
	}

	var manifest composerJSON
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return parseResult{}, fmt.Errorf("depscan: parse composer.json: %w", err)
	}
	direct := make(map[string]string, len(manifest.Require)+len(manifest.RequireDev))
	for name, r := range manifest.Require {
		if name == "php" {
			continue // a PHP version constraint, not a package
		}
		direct[name] = r
	}
	maps.Copy(direct, manifest.RequireDev)

	result := parseResult{ManifestFound: true, ManifestPath: "composer.json"}

	lockBytes, err := os.ReadFile(filepath.Join(workspaceDir, "composer.lock"))
	if err != nil {
		if !os.IsNotExist(err) {
			return parseResult{}, fmt.Errorf("depscan: read composer.lock: %w", err)
		}
		out := make([]Dependency, 0, len(direct))
		for name, r := range direct {
			out = append(out, Dependency{Ecosystem: "composer", Name: name, Version: r, IsDirect: true, ManifestPath: "composer.json", DeclaredRange: r})
		}
		result.Dependencies = out
		return result, nil
	}

	var lock composerLockJSON
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		return parseResult{}, fmt.Errorf("depscan: parse composer.lock: %w", err)
	}
	result.LockfileFound = true

	all := append(append([]composerLockPackage{}, lock.Packages...), lock.PackagesDev...)
	deps := make([]Dependency, 0, len(all))
	for _, p := range all {
		if p.Name == "" {
			continue
		}
		_, isDirect := direct[p.Name]
		deps = append(deps, Dependency{
			Ecosystem: "composer", Name: p.Name, Version: p.Version,
			IsDirect: isDirect, ManifestPath: "composer.lock", DeclaredRange: direct[p.Name],
		})
	}
	result.Dependencies = deps
	return result, nil
}
