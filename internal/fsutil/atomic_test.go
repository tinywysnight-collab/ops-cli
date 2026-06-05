package fsutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/fsutil"
)

func TestAtomicWriteCreatesWithPerm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "file")
	require.NoError(t, fsutil.AtomicWrite(path, []byte("hello"), 0o600))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "hello", string(data))

	fi, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), fi.Mode().Perm())

	di, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), di.Mode().Perm())
}

// TestAtomicWritePreservesSymlink asserts that writing through a symlinked
// destination lands on the link's target and never replaces the link itself
// with a regular file (a user may intentionally symlink ~/.aws/credentials).
func TestAtomicWritePreservesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real", "credentials")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o700))
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))

	link := filepath.Join(dir, "credentials")
	require.NoError(t, os.Symlink(target, link))

	require.NoError(t, fsutil.AtomicWrite(link, []byte("new"), 0o600))

	fi, err := os.Lstat(link)
	require.NoError(t, err)
	require.NotZero(t, fi.Mode()&os.ModeSymlink, "symlink must be preserved, not replaced by a regular file")

	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "new", string(data), "write must land on the symlink target")
}

// TestAtomicWriteFollowsMultiHopSymlink asserts the full symlink chain is
// resolved (link → link → target), so a two-hop indirection lands on the final
// regular file and never replaces any link in the chain.
func TestAtomicWriteFollowsMultiHopSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real", "credentials")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o700))
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))

	hop1 := filepath.Join(dir, "hop1")
	require.NoError(t, os.Symlink(target, hop1))
	link := filepath.Join(dir, "credentials")
	require.NoError(t, os.Symlink(hop1, link))

	require.NoError(t, fsutil.AtomicWrite(link, []byte("new"), 0o600))

	for _, l := range []string{link, hop1} {
		fi, err := os.Lstat(l)
		require.NoError(t, err)
		require.NotZero(t, fi.Mode()&os.ModeSymlink, "symlink %s must be preserved", l)
	}
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "new", string(data), "write must land on the final chain target")
}

// TestAtomicWriteSymlinkTargetDirMissing asserts a symlink whose target
// directory does not yet exist resolves to the not-yet-existing target and the
// write creates it (intentional indirection into a vault/dotfiles location).
func TestAtomicWriteSymlinkTargetDirMissing(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "missing", "credentials")
	link := filepath.Join(dir, "credentials")
	require.NoError(t, os.Symlink(target, link))

	require.NoError(t, fsutil.AtomicWrite(link, []byte("new"), 0o600))

	fi, err := os.Lstat(link)
	require.NoError(t, err)
	require.NotZero(t, fi.Mode()&os.ModeSymlink, "symlink must be preserved")
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "new", string(data))
}

func TestAtomicWriteReplacesAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	require.NoError(t, fsutil.AtomicWrite(path, []byte("old"), 0o600))
	require.NoError(t, fsutil.AtomicWrite(path, []byte("new-content"), 0o600))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "new-content", string(data))

	// No leftover temp files in the directory.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "file", entries[0].Name())
}
