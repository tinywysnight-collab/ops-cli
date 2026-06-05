package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/auth"
	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/creds"
	"github.com/tinywysnight-collab/ops-cli/internal/kube"
	"github.com/tinywysnight-collab/ops-cli/internal/paths"
)

// runOutErr executes the root command with isolated streams and returns both
// stdout and stderr, so tests can assert that human confirmations land on stderr
// while stdout stays export-only.
func runOutErr(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := NewRootCommand()
	var out, errOut strings.Builder
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	persistentOpr, persistentMode = false, ""
	err = root.Execute()
	return out.String(), errOut.String(), err
}

// setupFakeEnv wires the AWS/Entra/kube seams with fakes and points opsx at a
// temp config/credentials dir, exercising the real command wiring otherwise.
func setupFakeEnv(t *testing.T, cfgYAML string) {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "opsx")
	require.NoError(t, os.MkdirAll(cfgDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfgYAML), 0o600))
	t.Setenv("OPSX_CONFIG_DIR", cfgDir)
	t.Setenv("OPSX_CREDENTIALS_FILE", filepath.Join(dir, "aws", "credentials"))

	now := time.Now()
	samlProviderFactory = func(config.Auth) auth.SAMLProvider { return fakeProvider{} }
	masterAssumeFactory = func(context.Context, string) (auth.AssumeWithSAMLAPI, error) {
		return fakeMasterSTS{exp: now.Add(time.Hour)}, nil
	}
	citizenAssumer = func(_ context.Context, _ creds.Credentials, _, _, _ string) (creds.Credentials, time.Time, error) {
		return creds.Credentials{AccessKeyID: "CITIZENAK", SecretAccessKey: "CITIZENSK", SessionToken: "CITIZENST"}, now.Add(time.Hour), nil
	}
	kubeServiceFactory = func() *kube.Service {
		return &kube.Service{
			LookPath: func(string) (string, error) { return "/usr/bin/x", nil },
			Exec: func(_ context.Context, _ []string, _ string, args ...string) error {
				for i, a := range args {
					if a == "--kubeconfig" {
						require.NoError(t, os.MkdirAll(filepath.Dir(args[i+1]), 0o700))
						require.NoError(t, os.WriteFile(args[i+1], []byte("kind: Config\n"), 0o600))
					}
				}
				return nil
			},
		}
	}
}

func exportValue(out, key string) string {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if v, ok := strings.CutPrefix(line, "export "+key+"="); ok {
			return v
		}
	}
	return ""
}

func powershellEnvValue(out, key string) string {
	prefix := "$env:" + key + ` = "`
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if v, ok := strings.CutPrefix(line, prefix); ok {
			return strings.TrimSuffix(v, `"`)
		}
	}
	return ""
}

// dottedAliasConfig binds a cluster alias containing a dot, which is config-valid
// (aliasPattern allows '.') and exercises the kubeconfig percent-encoding.
const dottedAliasConfig = `
accounts:
  dev:
    account_id: "111111111111"
    description: "Dev"
    region: ap-southeast-2
clusters:
  dev.syd:
    account: dev
    region: ap-southeast-2
    name: dev-eks-cluster-01
auth:
  master_account_id: "000000000000"
  saml_provider_arn: "arn:aws:iam::000000000000:saml-provider/EntraID"
  region: us-east-1
  master_roles: {admin: master_admin, opr: master_AWSOpr}
  citizen_roles: {admin: Admin, opr: AWSOpr}
`

// TestShellSwitchKubeDottedAliasEmitsEvalSafeExport pins task 14.1: a dotted
// cluster alias must not make `opsx kube` fail with an "unsafe value" error, and
// the emitted KUBECONFIG must round-trip back to the dotted alias.
func TestShellSwitchKubeDottedAliasEmitsEvalSafeExport(t *testing.T) {
	setupFakeEnv(t, dottedAliasConfig)
	_, _, err := runOutErr(t, "login")
	require.NoError(t, err)

	out, _, err := runOutErr(t, "shell-switch", "kube", "dev.syd")
	require.NoError(t, err, "a dotted alias must not be rejected as an unsafe export value")

	kubeconfig := exportValue(out, "KUBECONFIG")
	require.NotEmpty(t, kubeconfig, "shell-switch kube must emit export KUBECONFIG")
	require.Contains(t, kubeconfig, "dev%2Esyd", "the dot must be percent-encoded in the file name")

	got, ok := paths.ClusterFromKubeConfig(kubeconfig)
	require.True(t, ok)
	require.Equal(t, "dev.syd", got, "the encoded path must round-trip to the dotted alias")
}

// TestShellSwitchKubeConfirmsBothSwitches pins task 14.2 for the installed shell
// function path: stdout stays export-only and stderr carries one account/profile
// confirmation and one cluster confirmation.
func TestShellSwitchKubeConfirmsBothSwitches(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	_, _, err := runOutErr(t, "login")
	require.NoError(t, err)

	out, errOut, err := runOutErr(t, "shell-switch", "kube", "dev-syd")
	require.NoError(t, err)

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		require.True(t, strings.HasPrefix(line, "export "), "stdout must be export-only, got %q", line)
	}
	require.Regexp(t, `account.*AWS_PROFILE=dev\.admin`, errOut)
	require.Regexp(t, `cluster.*dev-syd`, errOut)
}

// TestBareKubeConfirmsBothSwitches pins task 14.2 for the bare `opsx kube` path:
// no stdout, two stderr confirmation lines.
func TestBareKubeConfirmsBothSwitches(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	_, _, err := runOutErr(t, "login")
	require.NoError(t, err)

	out, errOut, err := runOutErr(t, "kube", "dev-syd")
	require.NoError(t, err)
	require.Empty(t, strings.TrimSpace(out), "bare kube must not write to stdout")
	require.Regexp(t, `account.*AWS_PROFILE=dev\.admin`, errOut)
	require.Regexp(t, `cluster.*dev-syd`, errOut)
}

// TestStatusShowsClusterRegion pins task 14.3: with a cluster active, status
// shows the cluster's region.
func TestStatusShowsClusterRegion(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	_, _, err := runOutErr(t, "login")
	require.NoError(t, err)

	out, _, err := runOutErr(t, "shell-switch", "kube", "dev-syd")
	require.NoError(t, err)
	t.Setenv("AWS_PROFILE", exportValue(out, "AWS_PROFILE"))
	t.Setenv("KUBECONFIG", exportValue(out, "KUBECONFIG"))

	statusOut, _, err := runOutErr(t, "status")
	require.NoError(t, err)
	require.Contains(t, statusOut, "Region:")
	require.Contains(t, statusOut, "ap-southeast-2", "should show the active cluster's region")
}

// TestStatusShowsAccountRegionWhenNoCluster pins task 14.3: with no cluster
// active, status falls back to the account's resolved STS region.
func TestStatusShowsAccountRegionWhenNoCluster(t *testing.T) {
	const cfg = `
accounts:
  dev:
    account_id: "111111111111"
    description: "Dev"
    region: ap-south-1
clusters:
  dev-syd:
    account: dev
    region: ap-southeast-2
    name: dev-eks-cluster-01
auth:
  master_account_id: "000000000000"
  saml_provider_arn: "arn:aws:iam::000000000000:saml-provider/EntraID"
  region: us-east-1
  master_roles: {admin: master_admin, opr: master_AWSOpr}
  citizen_roles: {admin: Admin, opr: AWSOpr}
`
	setupFakeEnv(t, cfg)
	_, _, err := runOutErr(t, "login")
	require.NoError(t, err)

	useOut, _, err := runOutErr(t, "shell-switch", "use", "dev")
	require.NoError(t, err)
	t.Setenv("AWS_PROFILE", exportValue(useOut, "AWS_PROFILE"))

	statusOut, _, err := runOutErr(t, "status")
	require.NoError(t, err)
	require.Contains(t, statusOut, "Region:")
	require.Contains(t, statusOut, "ap-south-1", "should fall back to the account's resolved STS region")
}

// TestLsShowsAccountRegion pins task 14.3: `opsx ls` shows each account's
// configured region alongside its description.
func TestLsShowsAccountRegion(t *testing.T) {
	const cfg = `
accounts:
  dev:
    account_id: "111111111111"
    description: "Dev"
    region: ap-south-1
clusters:
  dev-syd:
    account: dev
    region: ap-southeast-2
    name: dev-eks-cluster-01
auth:
  master_account_id: "000000000000"
  saml_provider_arn: "arn:aws:iam::000000000000:saml-provider/EntraID"
  region: us-east-1
  master_roles: {admin: master_admin, opr: master_AWSOpr}
  citizen_roles: {admin: Admin, opr: AWSOpr}
`
	setupFakeEnv(t, cfg)
	out, _, err := runOutErr(t, "ls")
	require.NoError(t, err)
	require.Regexp(t, `dev — Dev \(region ap-south-1\)`, out)
	require.Contains(t, out, "dev-syd — account dev, region ap-southeast-2")
}

func TestShellSwitchPowerShellUseKubeAndModeEmitAssignments(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	_, _, err := runOutErr(t, "login")
	require.NoError(t, err)

	useOut, _, err := runOutErr(t, "shell-switch", "--shell", "powershell", "use", "dev")
	require.NoError(t, err)
	require.Equal(t, "dev.admin", powershellEnvValue(useOut, "AWS_PROFILE"))
	require.NotContains(t, useOut, "export ")

	kubeOut, errOut, err := runOutErr(t, "shell-switch", "--shell", "powershell", "kube", "dev-syd")
	require.NoError(t, err)
	require.Equal(t, "dev.admin", powershellEnvValue(kubeOut, "AWS_PROFILE"))
	require.NotEmpty(t, powershellEnvValue(kubeOut, "KUBECONFIG"))
	require.NotContains(t, kubeOut, "export ")
	require.Regexp(t, `account.*AWS_PROFILE=dev\.admin`, errOut)
	require.Regexp(t, `cluster.*dev-syd`, errOut)

	modeOut, _, err := runOutErr(t, "shell-switch", "--shell", "powershell", "mode", "opr")
	require.NoError(t, err)
	require.Equal(t, "opr", powershellEnvValue(modeOut, "OPSX_MODE"))
	require.NotContains(t, modeOut, "export ")
}
