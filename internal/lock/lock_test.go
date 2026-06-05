package lock_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	goflock "github.com/gofrs/flock"
	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/lock"
)

func TestWithRunsUnderLock(t *testing.T) {
	lp := filepath.Join(t.TempDir(), ".lock")
	ran := false
	require.NoError(t, lock.With(context.Background(), lp, func() error {
		ran = true
		return nil
	}))
	require.True(t, ran)
}

// TestWithTimesOutOnContention asserts the bounded acquisition returns a clear
// error instead of blocking forever when another holder keeps the lock.
func TestWithTimesOutOnContention(t *testing.T) {
	lp := filepath.Join(t.TempDir(), ".lock")

	// Hold the lock from a separate flock handle (simulating another process).
	holder := goflock.New(lp)
	locked, err := holder.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	defer func() { _ = holder.Unlock() }()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = lock.With(ctx, lp, func() error { return nil })
	require.Error(t, err)
	require.Contains(t, err.Error(), lp)
	require.Less(t, time.Since(start), 2*time.Second, "must not block past the bounded deadline")
}
