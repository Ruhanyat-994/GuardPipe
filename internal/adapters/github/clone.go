package github

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// ErrRepoTooLarge means the checkout exceeded the configured byte cap.
// destDir has already been removed by the time this is returned.
var ErrRepoTooLarge = errors.New("github: repository exceeds the configured size limit")

// ErrCloneFailed wraps any other clone failure (auth, network, not found).
var ErrCloneFailed = errors.New("github: clone failed")

// ShallowClone clones cloneURL into destDir with `--depth 1
// --single-branch --no-tags` semantics (documentation/05-module-specifications.md
// §5, "Workspace preparation") — go-git rather than a shelled-out `git`
// binary, so there is no shell command string to inject into at all
// (documentation/12-security-and-threat-model.md, finding E4). token is the
// caller's decrypted PAT and may be empty for a public repository.
//
// maxBytes is enforced *during* the clone, not after: every byte go-git
// writes — working-tree files and the packed/loose objects under .git
// alike — passes through a counting billy.Filesystem that fails the write
// once the cap is crossed, so an oversized repository never fully lands on
// disk (documentation/05-module-specifications.md §5's "Size guard …
// checked during clone, not after").
func ShallowClone(ctx context.Context, cloneURL, token, destDir string, maxBytes int64) error {
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return fmt.Errorf("github: create workspace directory: %w", err)
	}

	counter := new(int64)
	worktree := newLimitedFS(osfs.New(destDir), counter, maxBytes)
	dotGit, err := worktree.Chroot(".git")
	if err != nil {
		_ = os.RemoveAll(destDir)
		return fmt.Errorf("github: chroot .git directory: %w", err)
	}
	storer := filesystem.NewStorage(dotGit, cache.NewObjectLRUDefault())

	var auth transport.AuthMethod
	if token != "" {
		auth = &githttp.BasicAuth{Username: "x-access-token", Password: token}
	}

	_, err = git.CloneContext(ctx, storer, worktree, &git.CloneOptions{
		URL:          cloneURL,
		Auth:         auth,
		Depth:        1,
		SingleBranch: true,
		Tags:         git.NoTags,
	})
	if err != nil {
		_ = os.RemoveAll(destDir)
		if errors.Is(err, ErrRepoTooLarge) {
			return ErrRepoTooLarge
		}
		return fmt.Errorf("%w: %v", ErrCloneFailed, err)
	}
	return nil
}

// limitedFS wraps a billy.Filesystem so every byte written through Create
// or OpenFile counts against a shared budget, including writes made after
// Chroot (go-git chroots into ".git" for its object store, so the wrapping
// must survive that hop or the size guard would only cover working-tree
// files).
type limitedFS struct {
	billy.Filesystem
	counter *int64
	max     int64
}

func newLimitedFS(fs billy.Filesystem, counter *int64, max int64) *limitedFS {
	return &limitedFS{Filesystem: fs, counter: counter, max: max}
}

func (fs *limitedFS) Create(filename string) (billy.File, error) {
	f, err := fs.Filesystem.Create(filename)
	if err != nil {
		return nil, err
	}
	return &limitedFile{File: f, counter: fs.counter, max: fs.max}, nil
}

func (fs *limitedFS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	f, err := fs.Filesystem.OpenFile(filename, flag, perm)
	if err != nil {
		return nil, err
	}
	return &limitedFile{File: f, counter: fs.counter, max: fs.max}, nil
}

func (fs *limitedFS) Chroot(path string) (billy.Filesystem, error) {
	sub, err := fs.Filesystem.Chroot(path)
	if err != nil {
		return nil, err
	}
	return newLimitedFS(sub, fs.counter, fs.max), nil
}

// limitedFile wraps a billy.File so Write fails once the shared byte budget
// is exceeded, aborting whatever go-git operation was mid-write.
type limitedFile struct {
	billy.File
	counter *int64
	max     int64
}

func (f *limitedFile) Write(p []byte) (int, error) {
	if *f.counter > f.max {
		return 0, ErrRepoTooLarge
	}
	n, err := f.File.Write(p)
	*f.counter += int64(n)
	if err == nil && *f.counter > f.max {
		return n, ErrRepoTooLarge
	}
	return n, err
}
