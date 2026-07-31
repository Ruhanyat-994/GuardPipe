package github_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/github"
)

// newFixtureRepo creates a tiny local git repository with one commit, so
// clone tests never touch the network — documentation's testing philosophy
// says explicitly "tests never call … GitHub live."
func newFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello, guardpipe\n"), 0o644))

	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("README.md")
	require.NoError(t, err)

	sig := &object.Signature{Name: "Fixture", Email: "fixture@example.com", When: time.Now()}
	_, err = wt.Commit("initial commit", &git.CommitOptions{Author: sig, Committer: sig})
	require.NoError(t, err)

	return dir
}

func TestShallowClone_SucceedsWithinSizeCap(t *testing.T) {
	src := newFixtureRepo(t)
	dest := filepath.Join(t.TempDir(), "clone")

	err := github.ShallowClone(context.Background(), src, "", dest, 10*1024*1024)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dest, "README.md"))
	require.NoError(t, err)
	require.Equal(t, "hello, guardpipe\n", string(data))

	// Shallow: exactly the one commit should have landed, not any history
	// beyond it (documentation/05-module-specifications.md §5, "git clone
	// --depth 1").
	clone, err := git.PlainOpen(dest)
	require.NoError(t, err)
	head, err := clone.Head()
	require.NoError(t, err)
	commits, err := clone.Log(&git.LogOptions{From: head.Hash()})
	require.NoError(t, err)
	count := 0
	require.NoError(t, commits.ForEach(func(*object.Commit) error { count++; return nil }))
	require.Equal(t, 1, count)
}

func TestShallowClone_AbortsOverSizeCap(t *testing.T) {
	src := newFixtureRepo(t)
	dest := filepath.Join(t.TempDir(), "clone")

	// Near-miss the other direction from the success case above: same
	// repository, but a cap so small even one loose object exceeds it —
	// the clone must not silently succeed just because "some" content fit.
	err := github.ShallowClone(context.Background(), src, "", dest, 5)
	require.ErrorIs(t, err, github.ErrRepoTooLarge)

	_, statErr := os.Stat(dest)
	require.True(t, os.IsNotExist(statErr), "workspace directory must be removed after an aborted clone")
}

func TestShallowClone_NonexistentSourceFails(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "clone")
	err := github.ShallowClone(context.Background(), filepath.Join(t.TempDir(), "does-not-exist"), "", dest, 10*1024*1024)
	require.ErrorIs(t, err, github.ErrCloneFailed)
}
