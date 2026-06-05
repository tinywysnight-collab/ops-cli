package fsutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAtomicWriteFsyncsFileThenDir pins the durability contract: the temp file's
// contents are fsync'd before the rename, and the parent directory entry is
// fsync'd after it. Without both, a crash after rename can lose or truncate the
// destination, so "crash-safe" would be an overclaim.
func TestAtomicWriteFsyncsFileThenDir(t *testing.T) {
	var order []string
	origFile, origDir := syncFile, syncDir
	t.Cleanup(func() { syncFile, syncDir = origFile, origDir })
	syncFile = func(f *os.File) error { order = append(order, "file"); return origFile(f) }
	syncDir = func(name string) error { order = append(order, "dir"); return origDir(name) }

	dir := t.TempDir()
	require.NoError(t, AtomicWrite(filepath.Join(dir, "f"), []byte("x"), 0o600))
	require.Equal(t, []string{"file", "dir"}, order, "temp file must be fsync'd before the dir is fsync'd after rename")
}
