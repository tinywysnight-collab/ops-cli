// Package lock provides the single advisory file lock shared by the credentials
// and state writers, so concurrent opsx processes serialize their
// read-modify-write cycles and never corrupt the shared files.
package lock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// DefaultTimeout bounds lock acquisition when the caller's context carries no
// deadline of its own, so the CLI never blocks forever on contention.
const DefaultTimeout = 5 * time.Second

// retryInterval is how often we re-attempt the advisory lock while waiting.
const retryInterval = 50 * time.Millisecond

// With acquires an exclusive advisory lock at lockPath, runs fn, then releases
// it. Acquisition is bounded: it honors ctx cancellation (e.g. Ctrl-C) and, if
// ctx has no deadline, applies DefaultTimeout. On contention past the deadline
// it returns a clear error naming the lock. The lock's parent directory is
// created (0700) if missing.
func With(ctx context.Context, lockPath string, fn func() error) (err error) {
	if mkErr := os.MkdirAll(filepath.Dir(lockPath), 0o700); mkErr != nil {
		return fmt.Errorf("create lock dir: %w", mkErr)
	}

	lockCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		lockCtx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}

	fl := flock.New(lockPath)
	locked, lockErr := fl.TryLockContext(lockCtx, retryInterval)
	if lockErr != nil {
		return fmt.Errorf("acquire lock %s: %w", lockPath, lockErr)
	}
	if !locked {
		return fmt.Errorf("acquire lock %s: timed out (another opsx process holds it)", lockPath)
	}
	defer func() {
		if unlockErr := fl.Unlock(); unlockErr != nil && err == nil {
			err = fmt.Errorf("release lock %s: %w", lockPath, unlockErr)
		}
	}()
	return fn()
}
