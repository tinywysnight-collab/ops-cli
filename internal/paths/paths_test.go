package paths_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/paths"
)

func TestConfigDirEnvOverride(t *testing.T) {
	t.Setenv(paths.EnvConfigDir, "/tmp/opsx-test")
	dir, err := paths.ConfigDir()
	require.NoError(t, err)
	require.Equal(t, "/tmp/opsx-test", dir)
}

// TestDefaultKubeConfigHonorsHome asserts the default kubeconfig location is
// ~/.kube/config, derived from the home directory so the default-merge path has
// a single source of truth ($HOME-driven, overridable in tests).
func TestDefaultKubeConfigHonorsHome(t *testing.T) {
	// Clear the override so a developer's ambient OPSX_DEFAULT_KUBECONFIG cannot
	// mask the $HOME-derived default this test asserts.
	t.Setenv(paths.EnvDefaultKubeConfig, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := paths.DefaultKubeConfig()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".kube", "config"), got)
}

// TestDefaultKubeConfigEnvOverride lets tests redirect the default kubeconfig at
// a temp path without touching HOME (which other libraries also read).
func TestDefaultKubeConfigEnvOverride(t *testing.T) {
	t.Setenv(paths.EnvDefaultKubeConfig, "/tmp/opsx-default-kube/config")
	got, err := paths.DefaultKubeConfig()
	require.NoError(t, err)
	require.Equal(t, "/tmp/opsx-default-kube/config", got)
}

// TestKubeConfigPerClusterMode asserts each (cluster,mode) maps to a distinct
// kubeconfig file, which is what keeps two terminals' contexts from colliding.
func TestKubeConfigPerClusterMode(t *testing.T) {
	t.Setenv(paths.EnvConfigDir, "/tmp/opsx-test")

	devAdmin, err := paths.KubeConfig("dev-syd", "admin")
	require.NoError(t, err)
	require.Equal(t, "/tmp/opsx-test/kube/admin/dev-syd.yaml", devAdmin)

	devOpr, err := paths.KubeConfig("dev-syd", "opr")
	require.NoError(t, err)
	prodAdmin, err := paths.KubeConfig("prod-syd", "admin")
	require.NoError(t, err)

	require.NotEqual(t, devAdmin, devOpr, "same cluster, different mode → different file")
	require.NotEqual(t, devAdmin, prodAdmin, "different cluster → different file")
}

func TestKubeConfigEncodesDotAliasesUnambiguously(t *testing.T) {
	t.Setenv(paths.EnvConfigDir, "/tmp/opsx-test")

	withDot, err := paths.KubeConfig("dev.syd", "admin")
	require.NoError(t, err)
	withoutDot, err := paths.KubeConfig("dev", "syd.admin")
	require.Error(t, err)
	require.Empty(t, withoutDot)

	require.Equal(t, "/tmp/opsx-test/kube/admin/dev%2Esyd.yaml", withDot)
	require.False(t, strings.Contains(filepath.Base(withDot), ".."))
	require.True(t, strings.HasPrefix(withDot, "/tmp/opsx-test/kube/"))
}

func TestClusterFromKubeConfigRoundTrip(t *testing.T) {
	t.Setenv(paths.EnvConfigDir, "/tmp/opsx-test")

	cases := []string{"dev-syd", "dev.syd", "prod-eks-01"}
	for _, alias := range cases {
		t.Run(alias, func(t *testing.T) {
			for _, mode := range []string{"admin", "opr"} {
				p, err := paths.KubeConfig(alias, mode)
				require.NoError(t, err)
				got, ok := paths.ClusterFromKubeConfig(p)
				require.True(t, ok)
				require.Equal(t, alias, got)
			}
		})
	}
}

func TestClusterFromKubeConfigRejectsForeignPath(t *testing.T) {
	t.Setenv(paths.EnvConfigDir, "/tmp/opsx-test")

	for _, p := range []string{"", "/home/user/.kube/config", "/tmp/opsx-test/kube/admin/notyaml.txt"} {
		_, ok := paths.ClusterFromKubeConfig(p)
		require.False(t, ok, "foreign path %q must not decode to a cluster", p)
	}
}
