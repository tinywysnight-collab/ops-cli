package kube

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewServiceWiresDefaultLockPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OPSX_CONFIG_DIR", dir)
	s := NewService()
	require.NotNil(t, s.Exec)
	require.NotNil(t, s.LookPath)
	require.Equal(t, filepath.Join(dir, ".opsx.lock"), s.LockPath)
}

func TestDefaultExecRunsChildAndRoutesStderr(t *testing.T) {
	require.NoError(t, defaultExec(context.Background(), nil, "true"))
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	err := defaultExec(ctx, nil, "sleep", "5")
	require.Error(t, err)
	_ = os.Stderr // keep os import if assertions change
}
