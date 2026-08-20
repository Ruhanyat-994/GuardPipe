package docreview

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// maxDocumentBytes caps how much of one document (uploaded or
// repo-discovered) is sent for review — documentation/05-module-specifications.md
// §11's own cap: 100 KB each. A document over the cap is truncated with a
// recorded note (§11's own failure-mode table), not rejected outright.
const maxDocumentBytes = 100 * 1024

// maxDocumentsPerScan caps the combined uploaded + repo-discovered set —
// §11's own cap, 20 documents per scan.
const maxDocumentsPerScan = 20

// reviewDirs is where docreview looks for documents inside a repository
// checkout — §11's own discovery list: the repository root itself (scanned
// shallow — "root" means its own files, not the whole repository walked
// recursively from there) plus docs/, documentation/, design/,
// architecture/, adr/ (each walked recursively — a doc set nested a level
// or two deep, e.g. this very repository's own documentation/17-adr/, is
// the common case, not the exception).
var reviewDirs = []string{"", "docs", "documentation", "design", "architecture", "adr"}

var reviewExtensions = map[string]bool{".md": true, ".txt": true, ".adoc": true, ".rst": true}

// excludedNames are file/directory names §11 explicitly excludes from
// discovery, even inside a reviewed directory.
var excludedNames = map[string]bool{
	"LICENSE": true, "LICENSE.md": true, "LICENSE.txt": true,
	"CHANGELOG": true, "CHANGELOG.md": true,
	"node_modules": true, "vendor": true,
}

// discoveredDocument is one document found inside the workspace, before any
// AI review — Path is repo-relative, for display and location attribution.
type discoveredDocument struct {
	Path    string
	Content string
}

// discoverRepoDocuments finds every document §11's discovery rule matches
// inside the checkout. Unreadable files are skipped silently, same as every
// other engine's own discovery (cicdscan.discoverWorkflows, k8sscan's own).
func discoverRepoDocuments(workspaceDir string) ([]discoveredDocument, error) {
	seen := map[string]bool{}
	var out []discoveredDocument

	collect := func(path string, entry os.DirEntry) {
		rel := relDocPath(workspaceDir, path)
		if seen[rel] || excludedNames[entry.Name()] || looksGenerated(rel) {
			return
		}
		if !reviewExtensions[strings.ToLower(filepath.Ext(entry.Name()))] {
			return
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return
		}
		if len(content) > maxDocumentBytes {
			content = content[:maxDocumentBytes]
		}
		seen[rel] = true
		out = append(out, discoveredDocument{Path: rel, Content: string(content)})
	}

	// Repo root: its own files only — "root" is one directory, not the whole
	// repository walked recursively from there.
	rootEntries, err := os.ReadDir(workspaceDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range rootEntries {
		if entry.IsDir() {
			continue
		}
		collect(filepath.Join(workspaceDir, entry.Name()), entry)
	}

	// The named doc directories: walked recursively, since a real doc set is
	// commonly nested a level or two deep.
	for _, sub := range reviewDirs[1:] {
		dir := filepath.Join(workspaceDir, sub)
		walkErr := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) && path == dir {
					return nil // this doc directory doesn't exist in this repo — not an error
				}
				return err
			}
			if entry.IsDir() {
				if excludedNames[entry.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			collect(path, entry)
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	return out, nil
}

// looksGenerated is a light heuristic for §11's "generated API docs"
// exclusion — a path containing a "generated" segment or named after a
// common API-doc generator's own output is skipped. Not exhaustive by
// design: a false negative here just means one extra document gets
// reviewed, which is harmless; a false positive would silently skip a real
// document, which is the failure mode worth staying conservative about.
func looksGenerated(relPath string) bool {
	lower := strings.ToLower(relPath)
	if slices.Contains(strings.Split(lower, "/"), "generated") {
		return true
	}
	return strings.Contains(lower, "openapi") || strings.Contains(lower, "swagger")
}

func relDocPath(workspaceDir, path string) string {
	rel, err := filepath.Rel(workspaceDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
