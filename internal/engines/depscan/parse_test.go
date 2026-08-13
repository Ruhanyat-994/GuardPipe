package depscan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

// --- npm ---

func TestParseNPM_NoManifest(t *testing.T) {
	result, err := parseNPM(t.TempDir())
	require.NoError(t, err)
	require.False(t, result.ManifestFound)
}

func TestParseNPM_ManifestOnly_NoLockfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"dependencies":{"lodash":"^4.17.21"},"devDependencies":{"jest":"*"}}`)

	result, err := parseNPM(dir)
	require.NoError(t, err)
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

	result, err := parseNPM(dir)
	require.NoError(t, err)
	require.True(t, result.LockfileFound)
	require.Len(t, result.Dependencies, 2)

	byName := depsByName(result.Dependencies)
	require.Equal(t, "4.17.21", byName["lodash"].Version)
	require.True(t, byName["lodash"].IsDirect)
	require.False(t, byName["nested-transitive"].IsDirect)
}

// --- pypi ---

func TestParsePyPI_NoManifest(t *testing.T) {
	result, err := parsePyPI(t.TempDir())
	require.NoError(t, err)
	require.False(t, result.ManifestFound)
}

func TestParsePyPI_RequirementsTxt(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "requirements.txt", "django==3.2.0\nrequests>=2.25.0  # http client\n\n# a comment\n-r other.txt\nflask~=2.0\n")

	result, err := parsePyPI(dir)
	require.NoError(t, err)
	require.True(t, result.ManifestFound)
	require.True(t, result.LockfileFound)
	require.Len(t, result.Dependencies, 3)

	byName := depsByName(result.Dependencies)
	require.Equal(t, "3.2.0", byName["django"].Version)
	require.Equal(t, "2.25.0", byName["requests"].Version)
}

// --- go ---

func TestParseGo_NoManifest(t *testing.T) {
	result, err := parseGo(t.TempDir())
	require.NoError(t, err)
	require.False(t, result.ManifestFound)
}

func TestParseGo_ModOnly_NoSum(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.23\n\nrequire github.com/gin-gonic/gin v1.10.1\n")

	result, err := parseGo(dir)
	require.NoError(t, err)
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

	result, err := parseGo(dir)
	require.NoError(t, err)
	require.True(t, result.LockfileFound)
	require.Len(t, result.Dependencies, 2)

	byName := depsByName(result.Dependencies)
	require.True(t, byName["github.com/gin-gonic/gin"].IsDirect)
	require.False(t, byName["golang.org/x/net"].IsDirect)
}

// --- maven ---

func TestParseMaven_NoManifest(t *testing.T) {
	result, err := parseMaven(t.TempDir())
	require.NoError(t, err)
	require.False(t, result.ManifestFound)
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

	result, err := parseMaven(dir)
	require.NoError(t, err)
	require.True(t, result.ManifestFound)
	require.Len(t, result.Dependencies, 1)
	require.Equal(t, "org.apache.logging.log4j:log4j-core", result.Dependencies[0].Name)
	require.Equal(t, "2.14.1", result.Dependencies[0].Version)
}

// --- composer ---

func TestParseComposer_NoManifest(t *testing.T) {
	result, err := parseComposer(t.TempDir())
	require.NoError(t, err)
	require.False(t, result.ManifestFound)
}

func TestParseComposer_ManifestOnly_NoLockfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", `{"require":{"php":"^8.0","monolog/monolog":"^2.0"}}`)

	result, err := parseComposer(dir)
	require.NoError(t, err)
	require.False(t, result.LockfileFound)
	require.Len(t, result.Dependencies, 1, "the php constraint itself is not a package")
	require.Equal(t, "monolog/monolog", result.Dependencies[0].Name)
}

func TestParseComposer_WithLockfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", `{"require":{"monolog/monolog":"^2.0"}}`)
	writeFile(t, dir, "composer.lock", `{"packages":[{"name":"monolog/monolog","version":"2.3.5"}],"packages-dev":[]}`)

	result, err := parseComposer(dir)
	require.NoError(t, err)
	require.True(t, result.LockfileFound)
	require.Len(t, result.Dependencies, 1)
	require.Equal(t, "2.3.5", result.Dependencies[0].Version)
}

// --- shared test helper ---

func depsByName(deps []Dependency) map[string]Dependency {
	out := make(map[string]Dependency, len(deps))
	for _, d := range deps {
		out[d.Name] = d
	}
	return out
}
