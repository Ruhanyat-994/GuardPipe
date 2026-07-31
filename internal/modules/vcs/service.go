// Package vcs is the shared git/GitHub capability documented in
// documentation/03-architecture-overview.md's module map: "git clone,
// GitHub client." It has no state and no dependents beyond the adapter it
// wraps (documentation/05-module-specifications.md §1) — `project` uses
// ValidateRepository when a repository is attached, and the orchestrator
// (from Phase 6) uses ShallowClone at scan time. Neither business rule
// lives here; this package only translates between adapters/github and the
// modules that call it.
package vcs

import (
	"context"
	"fmt"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/github"
)

// RepoInfo is what a caller learns about a repository after validation —
// enough to populate the `repositories` row
// (documentation/06-database-design.md §4.5).
type RepoInfo struct {
	Owner         string
	Name          string
	NormalizedURL string
	DefaultBranch string
	IsPrivate     bool
	SizeKB        int64
}

// GitHubClient is the subset of adapters/github.Client this package needs.
// Defined here (the consumer) per documentation/04-backend-architecture.md
// §5.1, so tests substitute a fake instead of making a real HTTP call.
type GitHubClient interface {
	GetRepository(ctx context.Context, owner, name, token string) (*github.RepoMetadata, error)
}

// CloneFunc matches adapters/github.ShallowClone's signature. It's a func
// type rather than a one-method interface because the real implementation
// is already a free function, not something worth wrapping in a struct.
type CloneFunc func(ctx context.Context, cloneURL, token, destDir string, maxBytes int64) error

// Service validates repository URLs/credentials and clones repositories to
// local disk.
type Service interface {
	// ValidateRepository parses rawURL, confirms it (and, if private, token)
	// actually resolves against GitHub, and returns the metadata `project`
	// needs to store (FR-PRJ-005: "validate a repository URL and confirm
	// reachability before saving it").
	ValidateRepository(ctx context.Context, rawURL, token string) (*RepoInfo, error)

	// ShallowClone clones rawURL into destDir, capped at maxRepoMB
	// (documentation/05-module-specifications.md §5, "Workspace
	// preparation"). Not exercised end-to-end until the orchestrator lands
	// in Phase 6; built now so that phase has nothing left to design here.
	ShallowClone(ctx context.Context, rawURL, token, destDir string) error
}

type service struct {
	client    GitHubClient
	clone     CloneFunc
	maxRepoMB int
}

// NewService wires the vcs module. maxRepoMB is GUARDPIPE_MAX_REPO_MB
// (documentation/13-devops-and-environments.md §5.4).
func NewService(client GitHubClient, clone CloneFunc, maxRepoMB int) Service {
	return &service{client: client, clone: clone, maxRepoMB: maxRepoMB}
}

func (s *service) ValidateRepository(ctx context.Context, rawURL, token string) (*RepoInfo, error) {
	ref, err := github.ParseRepoURL(rawURL)
	if err != nil {
		return nil, err
	}

	meta, err := s.client.GetRepository(ctx, ref.Owner, ref.Name, token)
	if err != nil {
		return nil, err
	}

	return &RepoInfo{
		Owner:         ref.Owner,
		Name:          ref.Name,
		NormalizedURL: ref.NormalizedURL,
		DefaultBranch: meta.DefaultBranch,
		IsPrivate:     meta.IsPrivate,
		SizeKB:        meta.SizeKB,
	}, nil
}

func (s *service) ShallowClone(ctx context.Context, rawURL, token, destDir string) error {
	ref, err := github.ParseRepoURL(rawURL)
	if err != nil {
		return err
	}
	maxBytes := int64(s.maxRepoMB) * 1024 * 1024
	if err := s.clone(ctx, ref.CloneURL, token, destDir, maxBytes); err != nil {
		return fmt.Errorf("vcs: clone %s: %w", ref.NormalizedURL, err)
	}
	return nil
}
