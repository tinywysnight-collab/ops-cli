//go:build darwin || linux

package fsutil

import "os"

func defaultSyncDir(name string) error {
	d, err := os.Open(name)
	if err != nil {
		return err
	}
	if serr := d.Sync(); serr != nil {
		_ = d.Close()
		return serr
	}
	return d.Close()
}
