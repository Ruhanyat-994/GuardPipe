package docreview

import "testing"

// TestDiscoverRepoDocuments_FindsMatchingFiles is the true-positive half:
// a file directly in the repo root and a nested file under a reviewed
// directory (documentation/17-adr/-shaped) must both be found.
func TestDiscoverRepoDocuments_FindsMatchingFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "README.md", "# Readme")
	writeTestFile(t, dir, "docs/architecture.md", "# Architecture\n\nSome content.")
	writeTestFile(t, dir, "documentation/17-adr/0001-example.md", "# ADR 1")

	docs, err := discoverRepoDocuments(dir)
	if err != nil {
		t.Fatalf("discoverRepoDocuments() error = %v", err)
	}
	if len(docs) != 3 {
		t.Fatalf("got %d documents, want 3: %+v", len(docs), docs)
	}
}

// TestDiscoverRepoDocuments_ExcludesNonMatches is the near-miss half:
// LICENSE/CHANGELOG-named files, the wrong extension, and anything inside
// an excluded directory name must not be discovered even though they sit
// inside a reviewed location.
func TestDiscoverRepoDocuments_ExcludesNonMatches(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "LICENSE.md", "MIT License text")
	writeTestFile(t, dir, "CHANGELOG.md", "## v1.0.0")
	writeTestFile(t, dir, "docs/openapi.yaml", "openapi: 3.0.0") // wrong extension
	writeTestFile(t, dir, "docs/node_modules/pkg/README.md", "should never be reached — excluded directory")
	writeTestFile(t, dir, "src/notes.md", "not under any reviewed location")

	docs, err := discoverRepoDocuments(dir)
	if err != nil {
		t.Fatalf("discoverRepoDocuments() error = %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("got %d documents, want 0 (all excluded): %+v", len(docs), docs)
	}
}

// TestDiscoverRepoDocuments_NoReviewedDirectories confirms an empty/absent
// set of reviewed directories is handled cleanly, not as an error — a
// repository with no docs/ (etc.) at all is a normal, common case.
func TestDiscoverRepoDocuments_NoReviewedDirectories(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "main.go", "package main")

	docs, err := discoverRepoDocuments(dir)
	if err != nil {
		t.Fatalf("discoverRepoDocuments() error = %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("got %d documents, want 0", len(docs))
	}
}
