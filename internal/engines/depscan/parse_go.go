package depscan

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// parseGo handles go.mod (direct requirements) and go.sum (the full
// resolved transitive set, exact versions —
// documentation/05-module-specifications.md's manifest-parsers table: "go.sum
// gives the full transitive set"). Deliberately not using
// golang.org/x/mod/modfile: go.mod's `require` block format is simple and
// stable enough that a small hand-written parser avoids a dependency for
// a handful of lines of well-documented syntax.
//
// Walks the whole workspace rather than checking only its root, the same
// as every other ecosystem parser in this package — a monorepo can have
// go.mod under backend/ (or several, one per Go module) with nothing at
// the checkout root.
func parseGo(workspaceDir string) ([]parseResult, error) {
	dirs, err := findManifestDirs(workspaceDir, "go.mod")
	if err != nil {
		return nil, fmt.Errorf("depscan: find go.mod: %w", err)
	}
	results := make([]parseResult, 0, len(dirs))
	for _, dir := range dirs {
		result, err := parseGoAt(workspaceDir, dir)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func parseGoAt(workspaceDir, dir string) (parseResult, error) {
	manifestPath := filepath.Join(dir, "go.mod")
	direct, err := parseGoMod(manifestPath)
	if os.IsNotExist(err) {
		return parseResult{}, nil
	}
	if err != nil {
		return parseResult{}, err
	}

	relManifest := relManifestPath(workspaceDir, manifestPath)
	result := parseResult{ManifestFound: true, ManifestPath: relManifest}

	sumPath := filepath.Join(dir, "go.sum")
	sumDeps, err := parseGoSum(sumPath, direct, relManifestPath(workspaceDir, sumPath))
	if err != nil {
		if os.IsNotExist(err) {
			out := make([]Dependency, 0, len(direct))
			for name, version := range direct {
				out = append(out, Dependency{Ecosystem: "go", Name: name, Version: version, IsDirect: true, ManifestPath: relManifest, DeclaredRange: version})
			}
			result.Dependencies = out
			return result, nil
		}
		return parseResult{}, err
	}
	result.LockfileFound = true
	result.Dependencies = sumDeps
	return result, nil
}

// parseGoMod reads only the `require` block(s) — both the single-line
// `require example.com/x v1.2.3` form and the parenthesised block form.
func parseGoMod(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	direct := make(map[string]string)
	inBlock := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "require ("):
			inBlock = true
			continue
		case inBlock && line == ")":
			inBlock = false
			continue
		case inBlock:
			addGoModRequireLine(direct, line)
		case strings.HasPrefix(line, "require "):
			addGoModRequireLine(direct, strings.TrimPrefix(line, "require "))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("depscan: scan go.mod: %w", err)
	}
	return direct, nil
}

func addGoModRequireLine(direct map[string]string, line string) {
	line = strings.TrimSuffix(strings.TrimSpace(line), "// indirect")
	line = strings.TrimSpace(line)
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return
	}
	direct[fields[0]] = fields[1]
}

// parseGoSum reads go.sum's "module version hash" lines. go.sum lists two
// lines per module (the module hash and its go.mod hash, the latter
// suffixed "/go.mod") — only the bare module line is a real dependency
// entry, so the "/go.mod" suffix is filtered out to avoid double-counting.
func parseGoSum(path string, direct map[string]string, relPath string) ([]Dependency, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	seen := make(map[string]bool)
	var deps []Dependency
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		name, version := fields[0], fields[1]
		if strings.HasSuffix(version, "/go.mod") {
			continue
		}
		key := name + "@" + version
		if seen[key] {
			continue
		}
		seen[key] = true

		_, isDirect := direct[name]
		deps = append(deps, Dependency{
			// Go module versions keep their "v" prefix — it's the
			// canonical form OSV.dev's Go ecosystem entries use too,
			// unlike npm/PyPI where a leading "v"/operator is stripped.
			Ecosystem: "go", Name: name, Version: version,
			IsDirect: isDirect, ManifestPath: relPath, DeclaredRange: direct[name],
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("depscan: scan go.sum: %w", err)
	}
	return deps, nil
}
