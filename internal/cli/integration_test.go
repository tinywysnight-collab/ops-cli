package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/auth"
	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/creds"
	"github.com/tinywysnight-collab/ops-cli/internal/kube"
	"github.com/tinywysnight-collab/ops-cli/internal/state"
)

const integrationConfig = `
regions:
  - ap-southeast-2
  - us-east-1
  - us-west-2
accounts:
  dev:
    account_id: "111111111111"
    description: "Dev"
    region: us-east-1
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

// fakeMasterSTS returns deterministic master creds via AssumeRoleWithSAML.
type fakeMasterSTS struct{ exp time.Time }

func (f fakeMasterSTS) AssumeRoleWithSAML(_ context.Context, _ *sts.AssumeRoleWithSAMLInput, _ ...func(*sts.Options)) (*sts.AssumeRoleWithSAMLOutput, error) {
	return &sts.AssumeRoleWithSAMLOutput{Credentials: &ststypes.Credentials{
		AccessKeyId: aws.String("MASTERAK"), SecretAccessKey: aws.String("MASTERSK"),
		SessionToken: aws.String("MASTERST"), Expiration: aws.Time(f.exp),
	}}, nil
}

// fakeProvider returns a canned assertion (the company seam stand-in).
type fakeProvider struct{}

func (fakeProvider) FetchAssertion(context.Context, auth.MasterRole) (string, error) {
	return "ASSERTION", nil
}

type providerFunc func(context.Context, auth.MasterRole) (string, error)

func (f providerFunc) FetchAssertion(ctx context.Context, role auth.MasterRole) (string, error) {
	return f(ctx, role)
}

// run executes the root command with args and isolated env, returning stdout.
func run(t *testing.T, args ...string) string {
	t.Helper()
	root := NewRootCommand()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	// reset persistent flags between invocations
	persistentOpr, persistentMode = false, ""
	require.NoError(t, root.Execute(), "args=%v stderr=%s", args, errOut.String())
	return out.String()
}

func TestLoginFetchesSAMLBeforeBuildingSTSClient(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "opsx")
	require.NoError(t, os.MkdirAll(cfgDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(integrationConfig), 0o600))
	t.Setenv("OPSX_CONFIG_DIR", cfgDir)
	t.Setenv("OPSX_CREDENTIALS_FILE", filepath.Join(dir, "aws", "credentials"))

	oldProviderFactory := samlProviderFactory
	oldMasterFactory := masterAssumeFactory
	t.Cleanup(func() {
		samlProviderFactory = oldProviderFactory
		masterAssumeFactory = oldMasterFactory
	})

	now := time.Now()
	fetchedSAML := false
	samlProviderFactory = func(config.Auth) auth.SAMLProvider {
		return providerFunc(func(context.Context, auth.MasterRole) (string, error) {
			fetchedSAML = true
			return "ASSERTION", nil
		})
	}
	masterAssumeFactory = func(context.Context, string) (auth.AssumeWithSAMLAPI, error) {
		require.True(t, fetchedSAML, "login must ask Entra for SAML before building the AWS STS client")
		return fakeMasterSTS{exp: now.Add(time.Hour)}, nil
	}

	run(t, "login")
}

func TestIntegrationLoginUseKubeStatus(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "opsx")
	require.NoError(t, os.MkdirAll(cfgDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(integrationConfig), 0o600))
	t.Setenv("OPSX_CONFIG_DIR", cfgDir)
	t.Setenv("OPSX_CREDENTIALS_FILE", filepath.Join(dir, "aws", "credentials"))

	now := time.Now()

	// Override the AWS/Entra/kube seams with fakes — real command wiring otherwise.
	samlProviderFactory = func(config.Auth) auth.SAMLProvider { return fakeProvider{} }
	masterAssumeFactory = func(context.Context, string) (auth.AssumeWithSAMLAPI, error) {
		return fakeMasterSTS{exp: now.Add(time.Hour)}, nil
	}
	citizenAssumer = func(_ context.Context, _ creds.Credentials, _, _, _ string) (creds.Credentials, time.Time, error) {
		return creds.Credentials{AccessKeyID: "CITIZENAK", SecretAccessKey: "CITIZENSK", SessionToken: "CITIZENST"}, now.Add(time.Hour), nil
	}
	var gotKubeArgs []string
	kubeServiceFactory = func() *kube.Service {
		return &kube.Service{
			LookPath: func(string) (string, error) { return "/usr/bin/x", nil },
			Exec: func(_ context.Context, _ []string, _ string, args ...string) error {
				gotKubeArgs = args
				// pretend aws wrote the kubeconfig
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

	// login → master cached
	run(t, "login")
	cred := filepath.Join(dir, "aws", "credentials")
	data, err := os.ReadFile(cred)
	require.NoError(t, err)
	require.Contains(t, string(data), "[master_admin]")
	require.Contains(t, string(data), "MASTERAK")

	// shell-switch use → export AWS_PROFILE + citizen profile written
	useOut := run(t, "shell-switch", "use", "dev")
	require.Equal(t, "dev.admin", exportValue(useOut, "AWS_PROFILE"))
	require.Equal(t, "us-east-1", exportValue(useOut, "AWS_REGION"))
	require.Equal(t, "us-east-1", exportValue(useOut, "AWS_DEFAULT_REGION"))
	data, _ = os.ReadFile(cred)
	require.Contains(t, string(data), "[dev.admin]")
	require.Contains(t, string(data), "CITIZENAK")

	// shell-switch kube → exports BOTH AWS_PROFILE and KUBECONFIG (13.2);
	// --profile reached update-kubeconfig (R2)
	kubeOut := run(t, "shell-switch", "kube", "dev-syd")
	require.True(t, strings.HasPrefix(kubeOut, "export AWS_PROFILE=dev.admin\n"), "kube must emit AWS_PROFILE first; got %q", kubeOut)
	require.Contains(t, kubeOut, "\nexport AWS_REGION=ap-southeast-2\n")
	require.Contains(t, kubeOut, "\nexport AWS_DEFAULT_REGION=ap-southeast-2\n")
	require.Contains(t, kubeOut, "\nexport KUBECONFIG=")
	require.Contains(t, gotKubeArgs, "--profile")
	require.Contains(t, gotKubeArgs, "dev.admin")

	// status → reflects active account, mode, and cluster. It reads AWS_PROFILE
	// from the env, so simulate what the eval'd shell would have set.
	t.Setenv("AWS_PROFILE", "dev.admin")
	statusOut := run(t, "status")
	require.Contains(t, statusOut, "dev.admin")
	require.Contains(t, statusOut, "Account:")
	require.Contains(t, statusOut, "dev-syd")
	// Valid, just-ensured credentials must never be reported as EXPIRED.
	require.NotContains(t, statusOut, "EXPIRED")
}

func TestIntegrationDefaultCommandWritesDefaultProfile(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "opsx")
	require.NoError(t, os.MkdirAll(cfgDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(integrationConfig), 0o600))
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

	run(t, "login")
	run(t, "default", "dev")

	cred := filepath.Join(dir, "aws", "credentials")
	data, err := os.ReadFile(cred)
	require.NoError(t, err)
	require.Contains(t, string(data), "[dev.admin]")
	require.Contains(t, string(data), "[default]")
	require.Contains(t, string(data), "CITIZENAK")
}

// TestIntegrationBareKubeSetsProfileAndStatus asserts that `opsx kube` with no
// prior `opsx use` exports AWS_PROFILE (not only KUBECONFIG) and that `opsx
// status` then shows the account, cluster, and a non-expired expiry (13.2/13.3).
func TestIntegrationBareKubeSetsProfileAndStatus(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "opsx")
	require.NoError(t, os.MkdirAll(cfgDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(integrationConfig), 0o600))
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

	run(t, "login")

	// Bare kube (no prior use): emits AWS_PROFILE, session region, and KUBECONFIG.
	kubeOut := run(t, "shell-switch", "kube", "dev-syd")
	var profile, region, defaultRegion, kubeconfig string
	for _, line := range strings.Split(strings.TrimSpace(kubeOut), "\n") {
		if v, ok := strings.CutPrefix(line, "export AWS_PROFILE="); ok {
			profile = v
		}
		if v, ok := strings.CutPrefix(line, "export AWS_REGION="); ok {
			region = v
		}
		if v, ok := strings.CutPrefix(line, "export AWS_DEFAULT_REGION="); ok {
			defaultRegion = v
		}
		if v, ok := strings.CutPrefix(line, "export KUBECONFIG="); ok {
			kubeconfig = v
		}
	}
	require.Equal(t, "dev.admin", profile, "bare kube must export AWS_PROFILE")
	require.Equal(t, "ap-southeast-2", region, "bare kube must export AWS_REGION")
	require.Equal(t, "ap-southeast-2", defaultRegion, "bare kube must export AWS_DEFAULT_REGION")
	require.NotEmpty(t, kubeconfig, "bare kube must export KUBECONFIG")

	// Simulate the shell having eval'd both exports.
	t.Setenv("AWS_PROFILE", profile)
	t.Setenv("KUBECONFIG", kubeconfig)
	statusOut := run(t, "status")
	require.NotContains(t, statusOut, "No active opsx context")
	require.Contains(t, statusOut, "Account:")
	require.Contains(t, statusOut, "dev-syd")
	require.NotContains(t, statusOut, "EXPIRED")
}

func TestIntegrationKubeDoesNotMergeDefaultKubeconfig(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "opsx")
	require.NoError(t, os.MkdirAll(cfgDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(integrationConfig), 0o600))
	t.Setenv("OPSX_CONFIG_DIR", cfgDir)
	t.Setenv("OPSX_CREDENTIALS_FILE", filepath.Join(dir, "aws", "credentials"))

	defaultKube := filepath.Join(dir, "dotkube", "config")
	require.NoError(t, os.MkdirAll(filepath.Dir(defaultKube), 0o700))
	require.NoError(t, os.WriteFile(defaultKube, []byte("unrelated-user-context\n"), 0o600))

	now := time.Now()
	samlProviderFactory = func(config.Auth) auth.SAMLProvider { return fakeProvider{} }
	masterAssumeFactory = func(context.Context, string) (auth.AssumeWithSAMLAPI, error) {
		return fakeMasterSTS{exp: now.Add(time.Hour)}, nil
	}
	citizenAssumer = func(_ context.Context, _ creds.Credentials, _, _, _ string) (creds.Credentials, time.Time, error) {
		return creds.Credentials{AccessKeyID: "CITIZENAK", SecretAccessKey: "CITIZENSK", SessionToken: "CITIZENST"}, now.Add(time.Hour), nil
	}
	var calls [][]string
	kubeServiceFactory = func() *kube.Service {
		return &kube.Service{
			LookPath: func(string) (string, error) { return "/usr/bin/x", nil },
			Exec: func(_ context.Context, _ []string, _ string, args ...string) error {
				calls = append(calls, append([]string(nil), args...))
				for i, a := range args {
					if a == "--kubeconfig" {
						require.NotEqual(t, defaultKube, args[i+1], "kube must not write shared default kubeconfig")
						require.NoError(t, os.MkdirAll(filepath.Dir(args[i+1]), 0o700))
						require.NoError(t, os.WriteFile(args[i+1], []byte("kind: Config\n"), 0o600))
					}
				}
				return nil
			},
		}
	}

	run(t, "login")
	run(t, "shell-switch", "kube", "dev-syd")

	require.Len(t, calls, 1, "kube must only write the per-terminal kubeconfig by default")
	require.NotContains(t, calls[0], "--alias")
	require.Contains(t, calls[0], "--profile")
	require.Contains(t, calls[0], "dev.admin")

	data, err := os.ReadFile(defaultKube)
	require.NoError(t, err)
	require.Contains(t, string(data), "unrelated-user-context")
}

func TestRecordClusterCreatesMissingEntry(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "opsx")
	require.NoError(t, os.MkdirAll(cfgDir, 0o700))
	t.Setenv("OPSX_CONFIG_DIR", cfgDir)

	expiry := time.Now().Add(time.Hour)
	require.NoError(t, recordCluster(context.Background(), "dev.admin", "dev", "admin", "dev-syd", expiry))

	ss, err := stateStore()
	require.NoError(t, err)
	entry, ok, err := ss.Get("dev.admin")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "dev", entry.Account)
	require.Equal(t, "admin", entry.Mode)
	require.Equal(t, "dev-syd", entry.Cluster)
	// A seeded entry must carry the real credential expiry, not a zero value,
	// so `opsx status` does not misreport valid credentials as EXPIRED.
	require.True(t, expiry.Equal(entry.Expiry))
	require.False(t, entry.UpdatedAt.IsZero())
	require.False(t, creds.IsExpired(entry.Expiry, time.Now()))
}

func TestRecordClusterPreservesLatestStateFields(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "opsx")
	require.NoError(t, os.MkdirAll(cfgDir, 0o700))
	t.Setenv("OPSX_CONFIG_DIR", cfgDir)
	ss, err := stateStore()
	require.NoError(t, err)
	latest := time.Date(2026, 5, 30, 13, 0, 0, 0, time.UTC)
	require.NoError(t, ss.Put(context.Background(), "dev.admin", state.Entry{Expiry: latest, Account: "dev", Mode: "admin", UpdatedAt: latest}))

	require.NoError(t, recordCluster(context.Background(), "dev.admin", "dev", "admin", "dev-syd", time.Now()))

	entry, ok, err := ss.Get("dev.admin")
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, latest.Equal(entry.Expiry))
	require.Equal(t, "dev", entry.Account)
	require.Equal(t, "admin", entry.Mode)
	require.Equal(t, "dev-syd", entry.Cluster)
}
