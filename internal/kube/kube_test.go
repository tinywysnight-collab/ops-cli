package kube_test

import (
	"context"
	"errors"
	"path/filepath"
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

	err := s.UpdateKubeconfig(context.Background(), "ap-southeast-2", "dev-eks-cluster-01", kubePath, "dev.admin")
	require.NoError(t, err)

	require.Equal(t, "aws", gotName)
	require.Equal(t, []string{
		"eks", "update-kubeconfig",
		"--region", "ap-southeast-2",
		"--name", "dev-eks-cluster-01",
		"--kubeconfig", kubePath,
		"--profile", "dev.admin",
	}, gotArgs)
	require.Contains(t, gotEnv, "AWS_PROFILE=dev.admin")
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

	err := s.UpdateKubeconfig(context.Background(), "r", "c", filepath.Join(t.TempDir(), "k.yaml"), "dev.admin")
	require.Error(t, err)
	require.Contains(t, err.Error(), "kubectl")
}
