// Package kube switches EKS clusters by shelling out to `aws eks
// update-kubeconfig`, writing a per-(cluster,mode) kubeconfig file so each
// terminal's KUBECONFIG is isolated. kubectl/helm consume that file directly.
package kube

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ExecFunc runs an external command with extra environment entries. Child
// stdout/stderr must never reach opsx stdout (it would corrupt eval output).
type ExecFunc func(ctx context.Context, env []string, name string, args ...string) error

// LookPathFunc resolves an executable in PATH (exec.LookPath in production).
type LookPathFunc func(file string) (string, error)

// Service runs update-kubeconfig with injectable exec/lookup for testing.
type Service struct {
	Exec     ExecFunc
	LookPath LookPathFunc
}

// NewService returns a Service wired to the real os/exec implementations.
func NewService() *Service {
	return &Service{Exec: defaultExec, LookPath: exec.LookPath}
}

// requiredTools must be present for `opsx kube` to work.
var requiredTools = []string{"aws", "kubectl"}

// UpdateKubeconfig writes the cluster's kubeconfig to kubeconfigPath using the
// given AWS profile for credentials. It verifies prerequisites first and
// returns a clear error naming any missing tool.
func (s *Service) UpdateKubeconfig(ctx context.Context, region, clusterName, kubeconfigPath, awsProfile string) error {
	for _, tool := range requiredTools {
		if _, err := s.LookPath(tool); err != nil {
			return fmt.Errorf("required tool %q not found in PATH — install it to use `opsx kube`: %w", tool, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(kubeconfigPath), 0o700); err != nil {
		return fmt.Errorf("create kube dir: %w", err)
	}
	// --profile makes the generated kubeconfig's exec block carry the profile,
	// so `kubectl`'s `aws eks get-token` authenticates without a separate
	// `opsx use`. AWS_PROFILE is also set for the update-kubeconfig call itself.
	args := []string{
		"eks", "update-kubeconfig",
		"--region", region,
		"--name", clusterName,
		"--kubeconfig", kubeconfigPath,
		"--profile", awsProfile,
	}
	env := []string{"AWS_PROFILE=" + awsProfile}
	if err := s.Exec(ctx, env, "aws", args...); err != nil {
		return fmt.Errorf("aws eks update-kubeconfig for cluster %s: %w", clusterName, err)
	}
	return nil
}

// defaultExec runs name with args, inheriting the environment plus extra, and
// routes child output to stderr so opsx stdout stays clean for eval.
func defaultExec(ctx context.Context, extra []string, name string, args ...string) error {
	c := exec.CommandContext(ctx, name, args...)
	c.Env = append(os.Environ(), extra...)
	c.Stdout = os.Stderr
	c.Stderr = os.Stderr
	return c.Run()
}
