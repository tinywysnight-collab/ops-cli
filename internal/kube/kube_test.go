package kube_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/kube"
)

func TestUpdateKubeconfigInvokesAWS(t *testing.T) {
	var (
		gotEnv  []string
		gotName string
		gotArgs []string
	)
	kubePath := filepath.Join(t.TempDir(), "kube", "dev-syd.admin.yaml")

	s := &kube.Service{
		LookPath: func(string) (string, error) { return "/usr/bin/x", nil },
		Exec: func(_ context.Context, env []string, name string, args ...string) error {
			gotEnv, gotName, gotArgs = env, name, args
			return nil
		},
	}

	// Empty alias = the per-(cluster,mode) file write, which must be unchanged
	// (no --alias, so the AWS CLI picks its default context name).
	err := s.UpdateKubeconfig(context.Background(), "ap-southeast-2", "dev-eks-cluster-01", kubePath, "dev.admin", "")
	require.NoError(t, err)

	require.Equal(t, "aws", gotName)
	// The aws CLI writes a staging file (same directory) that is renamed over
	// the target; assert everything except the generated staging file name.
	stagingIdx := 0
	for i, a := range gotArgs {
		if a == "--kubeconfig" {
			stagingIdx = i + 1
		}
	}
	require.Equal(t, filepath.Join(filepath.Dir(kubePath), filepath.Base(gotArgs[stagingIdx])), gotArgs[stagingIdx])
	require.True(t, strings.HasPrefix(filepath.Base(gotArgs[stagingIdx]), ".opsx-kube-tmp-"))
	require.Equal(t, []string{
		"eks", "update-kubeconfig",
		"--region", "ap-southeast-2",
		"--name", "dev-eks-cluster-01",
		"--kubeconfig", gotArgs[stagingIdx],
		"--profile", "dev.admin",
	}, gotArgs)
	require.NotContains(t, gotArgs, "--alias")
	require.Contains(t, gotEnv, "AWS_PROFILE=dev.admin")
}

// TestUpdateKubeconfigAliasOption covers the optional --alias argument: when an
// explicit caller supplies one, the AWS CLI sets that context name as
// current-context, while --profile keeps the exec block self-authenticating.
func TestUpdateKubeconfigAliasOption(t *testing.T) {
	var gotArgs []string
	// Point at a not-yet-existing ~/.kube so we also prove the dir is created
	// (a missing ~/.kube must not make the merge fail — task 20.3).
	defaultPath := filepath.Join(t.TempDir(), "kube", "config")

	s := &kube.Service{
		LookPath: func(string) (string, error) { return "/usr/bin/x", nil },
		Exec: func(_ context.Context, _ []string, _ string, args ...string) error {
			gotArgs = args
			require.DirExists(t, filepath.Dir(defaultPath), "missing ~/.kube must be created before merge")
			return nil
		},
	}

	err := s.UpdateKubeconfig(context.Background(), "ap-southeast-2", "dev-eks-cluster-01", defaultPath, "dev.admin", "dev-syd")
	require.NoError(t, err)

	require.Equal(t, []string{
		"eks", "update-kubeconfig",
		"--region", "ap-southeast-2",
		"--name", "dev-eks-cluster-01",
		"--kubeconfig", gotArgs[indexAfter(gotArgs, "--kubeconfig")],
		"--profile", "dev.admin",
		"--alias", "dev-syd",
	}, gotArgs)
	// No destructive flag may be passed.
	require.NotContains(t, gotArgs, "--dry-run")
}

func TestUpdateKubeconfigMissingTool(t *testing.T) {
	s := &kube.Service{
		LookPath: func(file string) (string, error) {
			if file == "kubectl" {
				return "", errors.New("not found")
			}
			return "/usr/bin/aws", nil
		},
		Exec: func(context.Context, []string, string, ...string) error {
			t.Fatal("exec must not run when a prerequisite is missing")
			return nil
		},
	}

	err := s.UpdateKubeconfig(context.Background(), "r", "c", filepath.Join(t.TempDir(), "k.yaml"), "dev.admin", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "kubectl")
}

func indexAfter(args []string, flag string) int {
	for i, a := range args {
		if a == flag {
			return i + 1
		}
	}
	return 0
}
