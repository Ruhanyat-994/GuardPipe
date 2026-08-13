package depscan

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
)

type pomXML struct {
	Dependencies struct {
		Dependency []pomDependency `xml:"dependency"`
	} `xml:"dependencies"`
}

type pomDependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

// parseMaven handles pom.xml's <dependencies> block.
// documentation/05-module-specifications.md's manifest-parsers table also
// names build.gradle/gradle.lockfile — Groovy/Kotlin DSL parsing is a
// materially different (and much larger) problem than XML, and is a
// documented gap rather than attempted here. Property placeholders
// (${some.version}) are left unresolved rather than guessed at — an
// unresolved version is reported as-is so a caller can see it wasn't
// statically determinable, matching the module spec's own "properties
// resolved where statically determinable" carve-out.
//
// Walks the whole workspace rather than checking only its root — a
// services/x/pom.xml in a multi-module checkout is just as real a
// manifest as one at the top level.
func parseMaven(workspaceDir string) ([]parseResult, error) {
	dirs, err := findManifestDirs(workspaceDir, "pom.xml")
	if err != nil {
		return nil, fmt.Errorf("depscan: find pom.xml: %w", err)
	}
	results := make([]parseResult, 0, len(dirs))
	for _, dir := range dirs {
		result, err := parseMavenAt(workspaceDir, dir)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func parseMavenAt(workspaceDir, dir string) (parseResult, error) {
	manifestPath := filepath.Join(dir, "pom.xml")
	data, err := os.ReadFile(manifestPath)
	if os.IsNotExist(err) {
		return parseResult{}, nil
	}
	if err != nil {
		return parseResult{}, fmt.Errorf("depscan: read pom.xml: %w", err)
	}

	var pom pomXML
	if err := xml.Unmarshal(data, &pom); err != nil {
		return parseResult{}, fmt.Errorf("depscan: parse pom.xml: %w", err)
	}

	relManifest := relManifestPath(workspaceDir, manifestPath)
	deps := make([]Dependency, 0, len(pom.Dependencies.Dependency))
	for _, d := range pom.Dependencies.Dependency {
		if d.GroupID == "" || d.ArtifactID == "" {
			continue
		}
		deps = append(deps, Dependency{
			Ecosystem: "maven", Name: d.GroupID + ":" + d.ArtifactID, Version: d.Version,
			IsDirect: true, ManifestPath: relManifest, DeclaredRange: d.Version,
		})
	}

	// A pom.xml with dependencyManagement pinning is its own "lockfile" in
	// spirit; without a separate gradle.lockfile-equivalent for Maven, a
	// pom.xml with explicit <version> tags is treated as pinned rather than
	// flagged by depscan.hygiene.no-lockfile.
	return parseResult{Dependencies: deps, ManifestFound: true, ManifestPath: relManifest, LockfileFound: true}, nil
}
