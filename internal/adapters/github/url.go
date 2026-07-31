// Package github is the one place GuardPipe speaks to GitHub: parsing
// repository URLs, calling the REST API for metadata, and shallow-cloning a
// repository to local disk (documentation/04-backend-architecture.md §3,
// "github: REST client + clone"). It translates every failure into either a
// plain sentinel error (for input the caller should re-validate) or a
// *platform/errors.Error (for a failure of the external system itself).
package github

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ErrInvalidRepositoryURL means the input isn't a well-formed
// "https://github.com/<owner>/<repo>" URL (FR-PRJ-005).
var ErrInvalidRepositoryURL = errors.New("github: not a valid HTTPS GitHub repository URL")

// RepoRef identifies a repository by owner/name, plus the normalised HTTPS
// URL GuardPipe stores and clones from
// (documentation/06-database-design.md §4.5).
type RepoRef struct {
	Owner         string
	Name          string
	NormalizedURL string
	CloneURL      string
}

// ParseRepoURL accepts "https://github.com/<owner>/<repo>", tolerating a
// trailing ".git", a trailing slash, and a leading "www.", and rejects
// anything else — other hosts, non-HTTPS schemes, or extra path segments
// like "/tree/main" (FR-PRJ-005: "validate a repository URL … before saving
// it").
func ParseRepoURL(raw string) (RepoRef, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return RepoRef{}, fmt.Errorf("%w: %s", ErrInvalidRepositoryURL, raw)
	}

	if u.Scheme != "https" {
		return RepoRef{}, fmt.Errorf("%w: scheme must be https", ErrInvalidRepositoryURL)
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" && host != "www.github.com" {
		return RepoRef{}, fmt.Errorf("%w: host must be github.com", ErrInvalidRepositoryURL)
	}

	segments := splitPath(u.Path)
	if len(segments) != 2 {
		return RepoRef{}, fmt.Errorf("%w: path must be /<owner>/<repo>", ErrInvalidRepositoryURL)
	}

	owner := segments[0]
	name := strings.TrimSuffix(segments[1], ".git")
	if owner == "" || name == "" {
		return RepoRef{}, fmt.Errorf("%w: owner and repository name are required", ErrInvalidRepositoryURL)
	}

	normalized := fmt.Sprintf("https://github.com/%s/%s", owner, name)
	return RepoRef{
		Owner:         owner,
		Name:          name,
		NormalizedURL: normalized,
		CloneURL:      normalized + ".git",
	}, nil
}

func splitPath(p string) []string {
	return strings.FieldsFunc(p, func(r rune) bool { return r == '/' })
}
