package kube_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/kube"
)

const validKubeconfig = "apiVersion: v1\nclusters:\n- cluster:\n  name: full\n"

// fakeSlowExec writes a partial kubeconfig to the --kubeconfig argument, then
// blocks until ctx is done (simulating a killed mid-write aws CLI) or finishes
// writing after a delay (simulating a slow concurrent aws CLI).
func fakeSlowExec(partial bool, delay time.Duration) kube.ExecFunc {
	return func(ctx context.Context, _ []string, name string, args ...string) error {
		var path string
		for i, a := range args {
			if a == "--kubeconfig" {
				path = args[i+1]
			}
		}
		if partial {
			_ = os.WriteFile(path, []byte("apiVersion: v1\nclusters:\n- clu"), 0o600)
			<-ctx.Done()
			return ctx.Err()
		}
		_ = os.WriteFile(path, []byte(validKubeconfig), 0o600)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		return nil
	}
}

func newAtomicService(t *testing.T, exec kube.ExecFunc) *kube.Service {
	t.Helper()
	dir := t.TempDir()
	return &kube.Service{
		Exec:     exec,
		LookPath: func(string) (string, error) { return "/usr/bin/true", nil },
		LockPath: filepath.Join(dir, ".opsx.lock"),
	}
}

func TestKubeCancelledWriteLeavesExistingKubeconfigIntact(t *testing.T) {
	svc := newAtomicService(t, fakeSlowExec(true, 0))
	dir := t.TempDir()
	final := filepath.Join(dir, "admin", "dev-syd.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(final), 0o700))
	require.NoError(t, os.WriteFile(final, []byte(validKubeconfig), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()
	err := svc.UpdateKubeconfig(ctx, "ap-southeast-2", "dev-eks", final, "dev.admin", "")
	require.Error(t, err)

	got, readErr := os.ReadFile(final)
	require.NoError(t, readErr)
	require.Equal(t, validKubeconfig, string(got), "cancelled switch must not corrupt the existing kubeconfig")
}

func TestKubeConcurrentSameClusterWritesYieldIntactKubeconfig(t *testing.T) {
	svc := newAtomicService(t, fakeSlowExec(false, 20*time.Millisecond))
	final := filepath.Join(t.TempDir(), "admin", "dev-syd.yaml")

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := svc.UpdateKubeconfig(context.Background(), "ap-southeast-2", "dev-eks", final, "dev.admin", "")
			if err != nil {
				t.Errorf("concurrent update failed: %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := os.ReadFile(final)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(got), "name: full"), "final kubeconfig must be a complete write, got: %q", got)
	// No staging leftovers.
	entries, err := os.ReadDir(filepath.Dir(final))
	require.NoError(t, err)
	for _, e := range entries {
		require.False(t, strings.Contains(e.Name(), ".tmp-"), "staging file leaked: %s", e.Name())
	}
}
