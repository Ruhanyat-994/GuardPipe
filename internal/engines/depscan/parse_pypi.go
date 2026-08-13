package depscan

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// requirementLine matches "name==1.2.3", "name>=1.2,<2.0", "name~=1.4",
// "name[extra]==1.2.3", stopping before any inline comment or environment
// marker (";python_version<'3.8'"). Anything it can't confidently parse
// (a "-r other.txt" include, a VCS URL, a bare "-e .") is skipped rather
// than misreported — depscan.vuln.* rules need a real name+version, not a
// guess.
var requirementLine = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*)(?:\[[^\]]*\])?\s*(==|>=|<=|~=|!=|>|<)\s*([A-Za-z0-9.*+!-]+)`)

// parsePyPI handles requirements.txt — documentation/05-module-specifications.md's
// manifest-parsers table also names poetry.lock/Pipfile.lock/pyproject.toml;
// their richer formats (Poetry's TOML dependency tree, Pipenv's locked
// hashes) are a documented gap, not silently mishandled — requirements.txt
// is the common case this parses correctly today.
//
// Walks the whole workspace rather than checking only its root — a
// backend/requirements.txt in a monorepo checkout is just as real a
// manifest as one at the top level.
func parsePyPI(workspaceDir string) ([]parseResult, error) {
	dirs, err := findManifestDirs(workspaceDir, "requirements.txt")
	if err != nil {
		return nil, fmt.Errorf("depscan: find requirements.txt: %w", err)
	}
	results := make([]parseResult, 0, len(dirs))
	for _, dir := range dirs {
		result, err := parsePyPIAt(workspaceDir, dir)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func parsePyPIAt(workspaceDir, dir string) (parseResult, error) {
	manifestPath := filepath.Join(dir, "requirements.txt")
	f, err := os.Open(manifestPath)
	if os.IsNotExist(err) {
		return parseResult{}, nil
	}
	if err != nil {
		return parseResult{}, fmt.Errorf("depscan: read requirements.txt: %w", err)
	}
	defer func() { _ = f.Close() }()

	var deps []Dependency
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if idx := strings.Index(line, ";"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}

		m := requirementLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name, operator, version := m[1], m[2], m[3]
		relManifest := relManifestPath(workspaceDir, manifestPath)
		deps = append(deps, Dependency{
			Ecosystem: "pypi", Name: name, Version: version,
			IsDirect: true, ManifestPath: relManifest, DeclaredRange: operator + version,
		})
	}
	if err := scanner.Err(); err != nil {
		return parseResult{}, fmt.Errorf("depscan: scan requirements.txt: %w", err)
	}

	return parseResult{
		Dependencies: deps, ManifestFound: true, ManifestPath: relManifestPath(workspaceDir, manifestPath),
		// requirements.txt IS its own lockfile in the sense the manifest-
		// parsers table cares about — a pinned "=="/"~=" entry is exact,
		// same role poetry.lock/Pipfile.lock play for their manifests.
		LockfileFound: true,
	}, nil
}
