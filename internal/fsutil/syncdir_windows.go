//go:build windows

package fsutil

func defaultSyncDir(string) error {
	// Windows does not support opening and fsyncing a directory with os.Open in
	// the same portable way as Unix. AtomicWrite still fsyncs the temp file
	// before rename; the parent-directory fsync durability step is a no-op on
	// Windows rather than making every write fail.
	return nil
}
