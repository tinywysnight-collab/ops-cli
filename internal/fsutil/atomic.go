// Package fsutil holds small filesystem helpers shared by the credential and
// state writers.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// syncFile / syncDir are seams so the durability contract is observable in
// tests; in production they perform real fsyncs.
var (
	syncFile = func(f *os.File) error { return f.Sync() }
	syncDir  = defaultSyncDir
)

// AtomicWrite writes data to path atomically AND durably: it writes a temp file
// in the destination's directory, fsyncs it, renames it into place, then fsyncs
// the parent directory. The rename gives atomicity (a crash mid-write never
// leaves a truncated destination); the fsyncs give durability (a write that
// returns success survives a power loss). The parent directory is created 0700;
// the destination ends up with perm.
//
// If path is a symlink, the write lands on the link's target so a user's
// intentional indirection (dotfiles, vault mount) is preserved rather than the
// link being silently replaced by a regular file.
func AtomicWrite(path string, data []byte, perm os.FileMode) (err error) {
	dest, err := resolveSymlink(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(dest)
	if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
		return fmt.Errorf("create dir %s: %w", dir, mkErr)
	}

	tmp, err := os.CreateTemp(dir, ".opsx-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err = tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err = syncFile(tmp); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsync temp file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err = os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("rename temp file into %s: %w", dest, err)
	}
	if err = syncDir(dir); err != nil {
		return fmt.Errorf("fsync dir %s: %w", dir, err)
	}
	return nil
}

// maxSymlinkHops bounds chain resolution so a symlink loop fails fast rather
// than spinning forever (mirrors the kernel's ELOOP guard).
const maxSymlinkHops = 40

// resolveSymlink returns the path the write should land on, following the full
// symlink chain (link → link → … → target) so every link in the chain is
// preserved rather than only the first. It returns path unchanged when it is
// not a symlink or does not yet exist; a dangling link resolves to its
// not-yet-existing target so intentional indirection (dotfiles, vault mount)
// is honored. A symlink cycle is reported as an error.
func resolveSymlink(path string) (string, error) {
	cur := path
	for hop := 0; hop < maxSymlinkHops; hop++ {
		fi, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return cur, nil
			}
			return "", fmt.Errorf("stat %s: %w", cur, err)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			return cur, nil
		}
		target, err := os.Readlink(cur)
		if err != nil {
			return "", fmt.Errorf("resolve symlink %s: %w", cur, err)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(cur), target)
		}
		cur = target
	}
	return "", fmt.Errorf("too many symlink levels resolving %s", path)
}
