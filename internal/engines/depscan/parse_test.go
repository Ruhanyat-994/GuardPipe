package depscan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// single unwraps a one-result parse into that result, failing the test if
// the count isn't exactly one — every parseXXX below now returns one
// parseResult per manifest *instance* found, so tests that plant exactly
// one manifest use this instead of repeating results[0] plus a length
// assertion at every call site.
func single(t *testing.T, results []parseResult, err error) parseResult {
	t.Helper()
	require.NoError(t, err)
	require.Len(t, results, 1)
	return results[0]
}

func parseNPMSingle(t *testing.T, dir string) parseResult {
	t.Helper()
	results, err := parseNPM(dir)
	return single(t, results, err)
}

func parsePyPISingle(t *testing.T, dir string) parseResult {
	t.Helper()
	results, err := parsePyPI(dir)
	return single(t, results, err)
}

func parseGoSingle(t *testing.T, dir string) parseResult {
	t.Helper()
	results, err := parseGo(dir)
	return single(t, results, err)
}

func parseMavenSingle(t *testing.T, dir string) parseResult {
	t.Helper()
	results, err := parseMaven(dir)
	return single(t, results, err)
}

func parseComposerSingle(t *testing.T, dir string) parseResult {
	t.Helper()
	results, err := parseComposer(dir)
	return single(t, results, err)
}

// --- npm ---

func TestParseNPM_NoManifest(t *testing.T) {
	results, err := parseNPM(t.TempDir())
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestParseNPM_ManifestOnly_NoLockfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"dependencies":{"lodash":"^4.17.21"},"devDependencies":{"jest":"*"}}`)

	result := parseNPMSingle(t, dir)
	require.True(t, result.ManifestFound)
	require.False(t, result.LockfileFound, "manifest present with no lockfile — depscan.hygiene.no-lockfile's exact condition")
	require.Len(t, result.Dependencies, 2)
}

func TestParseNPM_WithLockfile_ResolvesExactVersionsAndTransitive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"dependencies":{"lodash":"^4.17.20"}}`)
	writeFile(t, dir, "package-lock.json", `{
		"lockfileVersion": 3,
		"packages": {
			"": {"name": "root"},
			"node_modules/lodash": {"version": "4.17.21"},
			"node_modules/lodash/node_modules/nested-transitive": {"version": "1.0.0"}
		}
	}`)

	result := parseNPMSingle(t, dir)
	require.True(t, result.LockfileFound)
	require.Len(t, result.Dependencies, 2)

	byName := depsByName(result.Dependencies)
	require.Equal(t, "4.17.21", byName["lodash"].Version)
	require.True(t, byName["lodash"].IsDirect)
	require.False(t, byName["nested-transitive"].IsDirect)
}

// --- pypi ---

func TestParsePyPI_NoManifest(t *testing.T) {
	results, err := parsePyPI(t.TempDir())
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestParsePyPI_RequirementsTxt(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "requirements.txt", "django==3.2.0\nrequests>=2.25.0  # http client\n\n# a comment\n-r other.txt\nflask~=2.0\n")

	result := parsePyPISingle(t, dir)
	require.True(t, result.ManifestFound)
	require.True(t, result.LockfileFound)
	require.Len(t, result.Dependencies, 3)

	byName := depsByName(result.Dependencies)
	require.Equal(t, "3.2.0", byName["django"].Version)
	require.Equal(t, "2.25.0", byName["requests"].Version)
}

// --- go ---

func TestParseGo_NoManifest(t *testing.T) {
	results, err := parseGo(t.TempDir())
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestParseGo_ModOnly_NoSum(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.23\n\nrequire github.com/gin-gonic/gin v1.10.1\n")

	result := parseGoSingle(t, dir)
	require.True(t, result.ManifestFound)
	require.False(t, result.LockfileFound)
	require.Len(t, result.Dependencies, 1)
	require.Equal(t, "v1.10.1", result.Dependencies[0].Version)
}

func TestParseGo_WithSum_IncludesTransitive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.23\n\nrequire (\n\tgithub.com/gin-gonic/gin v1.10.1\n)\n")
	writeFile(t, dir, "go.sum", strings.Join([]string{
		"github.com/gin-gonic/gin v1.10.1 h1:abc=",
		"github.com/gin-gonic/gin v1.10.1/go.mod h1:def=",
		"golang.org/x/net v0.20.0 h1:ghi=",
		"golang.org/x/net v0.20.0/go.mod h1:jkl=",
	}, "\n")+"\n")

	result := parseGoSingle(t, dir)
	require.True(t, result.LockfileFound)
	require.Len(t, result.Dependencies, 2)

	byName := depsByName(result.Dependencies)
	require.True(t, byName["github.com/gin-gonic/gin"].IsDirect)
	require.False(t, byName["golang.org/x/net"].IsDirect)
}

// --- maven ---

func TestParseMaven_NoManifest(t *testing.T) {
	results, err := parseMaven(t.TempDir())
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestParseMaven_PomXML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pom.xml", `<project>
		<dependencies>
			<dependency>
				<groupId>org.apache.logging.log4j</groupId>
				<artifactId>log4j-core</artifactId>
				<version>2.14.1</version>
			</dependency>
		</dependencies>
	</project>`)

	result := parseMavenSingle(t, dir)
	require.True(t, result.ManifestFound)
	require.Len(t, result.Dependencies, 1)
	require.Equal(t, "org.apache.logging.log4j:log4j-core", result.Dependencies[0].Name)
	require.Equal(t, "2.14.1", result.Dependencies[0].Version)
}

// --- composer ---

func TestParseComposer_NoManifest(t *testing.T) {
	results, err := parseComposer(t.TempDir())
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestParseComposer_ManifestOnly_NoLockfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", `{"require":{"php":"^8.0","monolog/monolog":"^2.0"}}`)

	result := parseComposerSingle(t, dir)
	require.False(t, result.LockfileFound)
	require.Len(t, result.Dependencies, 1, "the php constraint itself is not a package")
	require.Equal(t, "monolog/monolog", result.Dependencies[0].Name)
}

func TestParseComposer_WithLockfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", `{"require":{"monolog/monolog":"^2.0"}}`)
	writeFile(t, dir, "composer.lock", `{"packages":[{"name":"monolog/monolog","version":"2.3.5"}],"packages-dev":[]}`)

	result := parseComposerSingle(t, dir)
	require.True(t, result.LockfileFound)
	require.Len(t, result.Dependencies, 1)
	require.Equal(t, "2.3.5", result.Dependencies[0].Version)
}

// --- recursive discovery (the actual behaviour under test: manifests must
// be found anywhere in the checkout, not only at its root) ---

func TestParseNPM_ManifestInSubdirectory_IsFound(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "frontend/package.json", `{"dependencies":{"react":"^18.0.0"}}`)

	result := parseNPMSingle(t, dir)
	require.True(t, result.ManifestFound)
	require.Equal(t, "frontend/package.json", result.ManifestPath)
}

func TestParseGo_ManifestInSubdirectory_IsFound(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "backend/go.mod", "module example.com/app\n\ngo 1.23\n\nrequire github.com/gin-gonic/gin v1.10.1\n")

	result := parseGoSingle(t, dir)
	require.True(t, result.ManifestFound)
	require.Equal(t, "backend/go.mod", result.ManifestPath)
}

// TestBackendAndFrontendSubdirectories_BothDiscovered is the exact shape
// this recursion exists for: a repository with nothing at its root except
// backend/ and frontend/ subdirectories, each carrying its own ecosystem's
// manifest. Before recursive discovery, Applicable's root-only os.Stat
// meant depscan reported "skipped, no findings" for a checkout like this
// even though real dependencies existed one level down.
func TestBackendAndFrontendSubdirectories_BothDiscovered(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "backend/go.mod", "module example.com/app\n\ngo 1.23\n\nrequire github.com/gin-gonic/gin v1.10.1\n")
	writeFile(t, dir, "frontend/package.json", `{"dependencies":{"react":"^18.0.0"}}`)

	goResult := parseGoSingle(t, dir)
	npmResult := parseNPMSingle(t, dir)
	require.Equal(t, "backend/go.mod", goResult.ManifestPath)
	require.Equal(t, "frontend/package.json", npmResult.ManifestPath)
}

// TestParseNPM_ManifestInsideNodeModules_NearMiss is the near-miss half:
// a package.json nested inside node_modules (every installed npm package
// ships one) must not be reported as a project manifest — it isn't
// something this checkout's own dependency-hygiene rules apply to, and
// walking into node_modules on a real repository would also be enormously
// slow.
func TestParseNPM_ManifestInsideNodeModules_NearMiss(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "node_modules/some-pkg/package.json", `{"dependencies":{"leftpad":"1.0.0"}}`)

	results, err := parseNPM(dir)
	require.NoError(t, err)
	require.Empty(t, results, "a package.json inside node_modules is not this checkout's own manifest")
}

// TestParseGo_ManifestInsideVendor_NearMiss mirrors the npm case for Go's
// vendor/ directory.
func TestParseGo_ManifestInsideVendor_NearMiss(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "vendor/github.com/some/dep/go.mod", "module github.com/some/dep\n\ngo 1.23\n")

	results, err := parseGo(dir)
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestApplicable_ManifestOnlyInSubdirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "backend/go.mod", "module example.com/app\n\ngo 1.23\n")
	writeFile(t, dir, "frontend/package.json", `{"dependencies":{}}`)

	e := New(nil)
	ok, reason := e.Applicable(context.Background(), domain.ScanInput{WorkspaceDir: dir})
	require.True(t, ok, "reason: %s", reason)
}

// --- shared test helper ---

func depsByName(deps []Dependency) map[string]Dependency {
	out := make(map[string]Dependency, len(deps))
	for _, d := range deps {
		out[d.Name] = d
	}
	return out
}
