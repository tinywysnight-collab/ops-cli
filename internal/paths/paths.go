// Package paths centralizes the on-disk locations opsx reads and writes.
//
// Defaults follow the PRD (~/.config/opsx, ~/.aws/credentials) but every
// location can be overridden via environment variables so tests can run
// against temporary directories with no dependency on the real home dir.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

// Environment overrides (primarily for tests and unusual setups).
const (
	EnvConfigDir       = "OPSX_CONFIG_DIR"       // overrides ~/.config/opsx
	EnvCredentialsFile = "OPSX_CREDENTIALS_FILE" // overrides ~/.aws/credentials
)

// ConfigDir returns the opsx configuration directory (default ~/.config/opsx).
func ConfigDir() (string, error) {
	if v := os.Getenv(EnvConfigDir); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "opsx"), nil
}

// ConfigFile returns the path to config.yaml.
func ConfigFile() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// StateFile returns the path to the expiry/state sidecar (state.json).
func StateFile() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

// LockFile returns the shared advisory-lock path guarding credential/state writes.
func LockFile() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".opsx.lock"), nil
}

// KubeDir returns the directory holding per-(cluster,mode) kubeconfig files.
func KubeDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "kube"), nil
}

// KubeConfig returns the kubeconfig path for a given cluster alias and mode.
func KubeConfig(cluster, mode string) (string, error) {
	mode, err := config.NormalizeMode(mode)
	if err != nil {
		return "", err
	}
	dir, err := KubeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, mode, fmt.Sprintf("%s.yaml", encodeKubeAlias(cluster))), nil
}

// encodeKubeAlias percent-encodes the characters that could let an alias escape
// or alias another kubeconfig file name. Mode now lives in its own subdirectory
// (kube/<mode>/<alias>.yaml) and config validation rejects path separators in
// aliases, so the original "<cluster>.<mode>" filename collision is already
// structurally impossible — only "." and "%" remain reachable here. The
// encoding is retained deliberately as belt-and-suspenders: it keeps the file
// name a single unambiguous path segment regardless of future alias-charset or
// layout changes, and "%" must be escaped first so the encoding stays
// reversible. See task 12.3 and the "Unambiguous kubeconfig file naming"
// rationale in the cluster-switching spec.
func encodeKubeAlias(alias string) string {
	replacer := strings.NewReplacer(
		"%", "%25",
		".", "%2E",
		"/", "%2F",
		"\\", "%5C",
	)
	return replacer.Replace(alias)
}

// ClusterFromKubeConfig reverses KubeConfig: given a KUBECONFIG path it returns
// the cluster alias it encodes, but only when the path is inside the opsx kube
// directory (kube/<mode>/<alias>.yaml). A foreign KUBECONFIG (one opsx did not
// create) yields ok=false so `opsx status` never mislabels it as a cluster.
func ClusterFromKubeConfig(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	kubeDir, err := KubeDir()
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(kubeDir, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	// rel must be exactly <mode>/<encoded-alias>.yaml.
	dir, base := filepath.Split(rel)
	mode := strings.TrimSuffix(dir, string(filepath.Separator))
	if mode == "" || strings.ContainsRune(mode, filepath.Separator) {
		return "", false
	}
	name, ok := strings.CutSuffix(base, ".yaml")
	if !ok || name == "" {
		return "", false
	}
	return decodeKubeAlias(name), true
}

// decodeKubeAlias reverses encodeKubeAlias. The longer "%2x" sequences are
// listed before "%25" so the single-pass replacer consumes them intact.
func decodeKubeAlias(s string) string {
	replacer := strings.NewReplacer(
		"%2E", ".",
		"%2F", "/",
		"%5C", "\\",
		"%25", "%",
	)
	return replacer.Replace(s)
}

// CredentialsFile returns the AWS shared credentials file (default ~/.aws/credentials).
func CredentialsFile() (string, error) {
	if v := os.Getenv(EnvCredentialsFile); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".aws", "credentials"), nil
}
