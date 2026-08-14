package dockerx

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func readTarNames(t *testing.T, r io.Reader) map[string][]byte {
	t.Helper()
	tr := tar.NewReader(r)
	out := make(map[string][]byte)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if hdr.Typeflag == tar.TypeDir {
			out[hdr.Name] = nil
			continue
		}
		content, err := io.ReadAll(tr)
		require.NoError(t, err)
		out[hdr.Name] = content
	}
	return out
}

func TestTarDirectory_IncludesFilesAndSubdirectories(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main\n"), 0o644))

	r, err := tarDirectory(dir)
	require.NoError(t, err)

	entries := readTarNames(t, r)
	require.Equal(t, []byte("FROM alpine\n"), entries["Dockerfile"])
	require.Equal(t, []byte("package main\n"), entries["src/main.go"])
	require.Contains(t, entries, "src/", "directories are archived too, not just files")
}

func TestTarDirectory_SkipsSymlinks(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.txt"), []byte("hi"), 0o644))
	err := os.Symlink(filepath.Join(dir, "real.txt"), filepath.Join(dir, "link.txt"))
	if err != nil {
		t.Skip("symlinks unsupported in this environment")
	}

	r, terr := tarDirectory(dir)
	require.NoError(t, terr)

	entries := readTarNames(t, r)
	require.Contains(t, entries, "real.txt")
	require.NotContains(t, entries, "link.txt", "a symlink must not be followed into the build context")
}
