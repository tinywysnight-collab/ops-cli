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

	"github.com/tinywysnight-collab/ops-cli/internal/lock"
	"github.com/tinywysnight-collab/ops-cli/internal/paths"
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
	// LockPath serializes concurrent writers of the same kubeconfig through
	// the shared opsx advisory lock. Empty disables locking (focused tests).
	LockPath string
}

// NewService returns a Service wired to the real os/exec implementations.
func NewService() *Service {
	lockPath, err := paths.LockFile()
	if err != nil {
		lockPath = ""
	}
	return &Service{Exec: defaultExec, LookPath: exec.LookPath, LockPath: lockPath}
}

// requiredTools must be present for `opsx kube` to work.
var requiredTools = []string{"aws", "kubectl"}

// UpdateKubeconfig writes the cluster's kubeconfig to kubeconfigPath using the
// given AWS profile for credentials. It verifies prerequisites first and
// returns a clear error naming any missing tool.
//
// When alias is non-empty, `--alias <alias>` is passed so the AWS CLI names the
// generated context with that friendly alias and sets it as current-context.
// The production switch path writes isolated per-(cluster,mode) files and
// passes an empty alias; the alias parameter remains available for explicit
// callers and focused tests.
func (s *Service) UpdateKubeconfig(ctx context.Context, region, clusterName, kubeconfigPath, awsProfile, alias string) error {
	for _, tool := range requiredTools {
		if _, err := s.LookPath(tool); err != nil {
			return fmt.Errorf("required tool %q not found in PATH — install it to use `opsx kube`: %w", tool, err)
		}
	}
	// MkdirAll handles a missing parent for first-time per-cluster kubeconfig
	// generation.
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
	if alias != "" {
		args = append(args, "--alias", alias)
	}
	env := []string{"AWS_PROFILE=" + awsProfile}

	// The aws CLI is an external writer with no atomicity of its own, so it
	// writes a staging file in the same directory that is fsynced and renamed
	// over the target: concurrent same-cluster switches serialize under the
	// shared lock, and a cancelled or failed run can never leave a torn or
	// partially-truncated kubeconfig that a live terminal would then consume.
	update := func() error {
		staging, err := os.CreateTemp(filepath.Dir(kubeconfigPath), ".opsx-kube-tmp-*")
		if err != nil {
			return fmt.Errorf("create staging kubeconfig: %w", err)
		}
		stagingPath := staging.Name()
		stagingErr := staging.Close()
		if stagingErr != nil {
			_ = os.Remove(stagingPath)
			return fmt.Errorf("close staging kubeconfig: %w", stagingErr)
		}
		defer func() { _ = os.Remove(stagingPath) }() // no-op after successful rename

		stageArgs := make([]string, len(args))
		copy(stageArgs, args)
		for i, a := range stageArgs {
			if a == "--kubeconfig" {
				stageArgs[i+1] = stagingPath
				break
			}
		}
		if err := s.Exec(ctx, env, "aws", stageArgs...); err != nil {
			return fmt.Errorf("aws eks update-kubeconfig for cluster %s: %w", clusterName, err)
		}
		f, err := os.Open(stagingPath)
		if err != nil {
			return fmt.Errorf("open staging kubeconfig: %w", err)
		}
		syncErr := f.Sync()
		closeErr := f.Close()
		if syncErr != nil {
			return fmt.Errorf("sync staging kubeconfig: %w", syncErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close staging kubeconfig: %w", closeErr)
		}
		if err := os.Chmod(stagingPath, 0o600); err != nil {
			return fmt.Errorf("chmod staging kubeconfig: %w", err)
		}
		if err := os.Rename(stagingPath, kubeconfigPath); err != nil {
			return fmt.Errorf("publish kubeconfig: %w", err)
		}
		return nil
	}
	if s.LockPath == "" {
		return update()
	}
	return lock.With(ctx, s.LockPath, update)
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
