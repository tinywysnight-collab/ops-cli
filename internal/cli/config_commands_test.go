package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/paths"
)

func runInteractive(t *testing.T, input string, tty bool, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	old := interactiveTerminal
	interactiveTerminal = func(_ io.Reader) bool { return tty }
	t.Cleanup(func() { interactiveTerminal = old })

	root := NewRootCommand()
	var out, errOut strings.Builder
	root.SetIn(strings.NewReader(input))
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	persistentOpr, persistentMode = false, ""
	err = root.Execute()
	return out.String(), errOut.String(), err
}

func configPathForTest(t *testing.T) string {
	t.Helper()
	path, err := paths.ConfigFile()
	require.NoError(t, err)
	return path
}

func TestAccountAddCommand(t *testing.T) {
	t.Run("help exposes add and delete only", func(t *testing.T) {
		root := NewRootCommand()
		root.SetArgs([]string{"account", "--help"})
		var out strings.Builder
		root.SetOut(&out)
		require.NoError(t, root.Execute())
		require.Contains(t, out.String(), "add")
		require.Contains(t, out.String(), "delete")
		require.NotContains(t, out.String(), "edit")
		require.NotContains(t, out.String(), "update")
	})

	t.Run("adds after retries and confirmation", func(t *testing.T) {
		setupFakeEnv(t, integrationConfig)
		stdout, stderr, err := runInteractive(t,
			"bad alias\nqa\n111111111111\n333333333333\n\n2\ny\n",
			true, "account", "add")
		require.NoError(t, err)
		require.Contains(t, stdout, "Added account")
		require.Contains(t, stderr, "disallowed")
		require.Contains(t, stderr, "already used")
		require.Contains(t, stderr, "us-east-1")
		require.Contains(t, stderr, "us-west-2")
		cfg, err := config.Load(configPathForTest(t))
		require.NoError(t, err)
		require.Equal(t, "333333333333", cfg.Accounts["qa"].AccountID)
		require.Equal(t, "us-east-1", cfg.Accounts["qa"].Region)
		require.Empty(t, cfg.Accounts["qa"].Description)
	})

	t.Run("blank confirmation cancels without write", func(t *testing.T) {
		setupFakeEnv(t, integrationConfig)
		before, err := os.ReadFile(configPathForTest(t))
		require.NoError(t, err)
		stdout, _, err := runInteractive(t,
			"qa\n333333333333\nQA\n1\n\n",
			true, "account", "add")
		require.NoError(t, err)
		require.Contains(t, stdout, "Cancelled.")
		after, err := os.ReadFile(configPathForTest(t))
		require.NoError(t, err)
		require.Equal(t, before, after)
	})

	t.Run("non tty fails", func(t *testing.T) {
		setupFakeEnv(t, integrationConfig)
		_, _, err := runInteractive(t, "", false, "account", "add")
		require.Error(t, err)
		require.Contains(t, err.Error(), "interactive terminal")
		_, statErr := os.Stat(filepath.Join(filepath.Dir(configPathForTest(t)), "config.yaml.bak"))
		require.True(t, os.IsNotExist(statErr))
	})
}

func TestAccountDeleteCommand(t *testing.T) {
	t.Run("referenced account is blocked with cluster list", func(t *testing.T) {
		setupFakeEnv(t, integrationConfig)
		_, _, err := runInteractive(t, "1\n", true, "account", "delete")
		require.Error(t, err)
		require.Contains(t, err.Error(), "dev-syd")
		require.Contains(t, err.Error(), "opsx cluster delete")
	})

	t.Run("active unreferenced account warns and deletes config only", func(t *testing.T) {
		cfgYAML := strings.Replace(integrationConfig, `clusters:
  dev-syd:
    account: dev
    region: ap-southeast-2
    name: dev-eks-cluster-01`, "clusters: {}", 1)
		setupFakeEnv(t, cfgYAML)
		t.Setenv("AWS_PROFILE", "dev.admin")
		stdout, stderr, err := runInteractive(t, "1\ny\n", true, "account", "delete")
		require.NoError(t, err)
		require.Contains(t, stderr, "active")
		require.Contains(t, stdout, "runtime artifacts were retained")
		cfg, err := config.Load(configPathForTest(t))
		require.NoError(t, err)
		require.Empty(t, cfg.Accounts)
		data, err := os.ReadFile(configPathForTest(t))
		require.NoError(t, err)
		require.Contains(t, string(data), "accounts: {}")
		require.Equal(t, "dev.admin", os.Getenv("AWS_PROFILE"))
	})

	t.Run("empty accounts is a normal result", func(t *testing.T) {
		cfgYAML := strings.Replace(integrationConfig, `accounts:
  dev:
    account_id: "111111111111"
    description: "Dev"
    region: us-east-1`, "accounts: {}", 1)
		cfgYAML = strings.Replace(cfgYAML, `clusters:
  dev-syd:
    account: dev
    region: ap-southeast-2
    name: dev-eks-cluster-01`, "clusters: {}", 1)
		setupFakeEnv(t, cfgYAML)
		stdout, _, err := runInteractive(t, "", true, "account", "delete")
		require.NoError(t, err)
		require.Contains(t, stdout, "No accounts configured.")
	})

	t.Run("cancellation retains account", func(t *testing.T) {
		cfgYAML := strings.Replace(integrationConfig, `clusters:
  dev-syd:
    account: dev
    region: ap-southeast-2
    name: dev-eks-cluster-01`, "clusters: {}", 1)
		setupFakeEnv(t, cfgYAML)
		stdout, _, err := runInteractive(t, "1\n\n", true, "account", "delete")
		require.NoError(t, err)
		require.Contains(t, stdout, "Cancelled.")
		cfg, err := config.Load(configPathForTest(t))
		require.NoError(t, err)
		require.Contains(t, cfg.Accounts, "dev")
	})
}

func TestClusterAddCommand(t *testing.T) {
	t.Run("help exposes add and delete only", func(t *testing.T) {
		root := NewRootCommand()
		root.SetArgs([]string{"cluster", "--help"})
		var out strings.Builder
		root.SetOut(&out)
		require.NoError(t, root.Execute())
		require.Contains(t, out.String(), "add")
		require.Contains(t, out.String(), "delete")
		require.NotContains(t, out.String(), "edit")
	})

	t.Run("adds alias and warns for duplicate real identity", func(t *testing.T) {
		setupFakeEnv(t, integrationConfig)
		stdout, stderr, err := runInteractive(t,
			"dev-syd\nstage-syd\n1\n1\ndev-eks-cluster-01\ny\n",
			true, "cluster", "add")
		require.NoError(t, err)
		require.Contains(t, stderr, "already exists")
		require.Contains(t, stderr, "duplicate")
		require.Contains(t, stderr, "dev-syd")
		require.Contains(t, stdout, "Added cluster")
		cfg, err := config.Load(configPathForTest(t))
		require.NoError(t, err)
		require.Equal(t, config.Cluster{
			Account: "dev", Region: "ap-southeast-2", Name: "dev-eks-cluster-01",
		}, cfg.Clusters["stage-syd"])
	})

	t.Run("no account fails before selection", func(t *testing.T) {
		cfgYAML := strings.Replace(integrationConfig, `accounts:
  dev:
    account_id: "111111111111"
    description: "Dev"
    region: us-east-1`, "accounts: {}", 1)
		cfgYAML = strings.Replace(cfgYAML, `clusters:
  dev-syd:
    account: dev
    region: ap-southeast-2
    name: dev-eks-cluster-01`, "clusters: {}", 1)
		setupFakeEnv(t, cfgYAML)
		_, _, err := runInteractive(t, "", true, "cluster", "add")
		require.Error(t, err)
		require.Contains(t, err.Error(), "opsx account add")
	})

	t.Run("non tty fails", func(t *testing.T) {
		setupFakeEnv(t, integrationConfig)
		_, _, err := runInteractive(t, "", false, "cluster", "add")
		require.Error(t, err)
		require.Contains(t, err.Error(), "interactive terminal")
	})
}

func TestClusterDeleteCommand(t *testing.T) {
	t.Run("active cluster warns and target-only deletion succeeds", func(t *testing.T) {
		setupFakeEnv(t, integrationConfig)
		kubePath, err := paths.KubeConfig("dev-syd", "admin")
		require.NoError(t, err)
		t.Setenv("KUBECONFIG", kubePath)
		stdout, stderr, err := runInteractive(t, "1\ny\n", true, "cluster", "delete")
		require.NoError(t, err)
		require.Contains(t, stderr, "active")
		require.Contains(t, stdout, "runtime artifacts were retained")
		cfg, err := config.Load(configPathForTest(t))
		require.NoError(t, err)
		require.Empty(t, cfg.Clusters)
		require.Contains(t, cfg.Accounts, "dev")
		data, err := os.ReadFile(configPathForTest(t))
		require.NoError(t, err)
		require.Contains(t, string(data), "clusters: {}")
		require.Equal(t, kubePath, os.Getenv("KUBECONFIG"))
	})

	t.Run("empty clusters is a normal result", func(t *testing.T) {
		cfgYAML := strings.Replace(integrationConfig, `clusters:
  dev-syd:
    account: dev
    region: ap-southeast-2
    name: dev-eks-cluster-01`, "clusters: {}", 1)
		setupFakeEnv(t, cfgYAML)
		stdout, _, err := runInteractive(t, "", true, "cluster", "delete")
		require.NoError(t, err)
		require.Contains(t, stdout, "No clusters configured.")
	})

	t.Run("cancellation retains cluster", func(t *testing.T) {
		setupFakeEnv(t, integrationConfig)
		stdout, _, err := runInteractive(t, "1\nno\n", true, "cluster", "delete")
		require.NoError(t, err)
		require.Contains(t, stdout, "Cancelled.")
		cfg, err := config.Load(configPathForTest(t))
		require.NoError(t, err)
		require.Contains(t, cfg.Clusters, "dev-syd")
	})
}
