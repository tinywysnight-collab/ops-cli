package shell_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/shell"
)

// On Windows the per-cluster kubeconfig path uses backslash separators
// (filepath.Join), e.g. C:\Users\me\.config\opsx\kube\admin\dev-syd.yaml.
// In a POSIX/Bash shell (Git Bash) a backslash is an escape character, so the
// POSIX dialect must normalize separators to forward slashes — which Bash treats
// literally and Windows kubectl/aws accept — rather than rejecting the path as an
// unsafe value.
func TestEmitPosixNormalizesWindowsPathSeparators(t *testing.T) {
	winPath := `C:\Users\me\.config\opsx\kube\admin\dev-syd.yaml`

	lines, err := shell.EmitAssignments(shell.DialectPOSIX, []shell.Assignment{
		{Key: "KUBECONFIG", Value: winPath},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"export KUBECONFIG=C:/Users/me/.config/opsx/kube/admin/dev-syd.yaml"}, lines)
}

// Genuine shell metacharacters must still be rejected after separator
// normalization, so the safety guarantee is not weakened.
func TestEmitPosixStillRejectsMetacharacters(t *testing.T) {
	_, err := shell.EmitAssignments(shell.DialectPOSIX, []shell.Assignment{
		{Key: "KUBECONFIG", Value: `/tmp/x;rm -rf ~`},
	})
	require.Error(t, err)
}
