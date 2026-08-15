package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUseRegionOverride: `opsx use <account> --region <r>` exports the given
// region for plain `aws` commands in that terminal, overriding the account's
// config-resolved (home) region — the multi-region-per-account case.
func TestUseRegionOverride(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	run(t, "login")

	out := run(t, "shell-switch", "use", "dev", "--region", "us-west-2")
	require.Equal(t, "dev.admin", exportValue(out, "AWS_PROFILE"))
	require.Equal(t, "us-west-2", exportValue(out, "AWS_REGION"))
	require.Equal(t, "us-west-2", exportValue(out, "AWS_DEFAULT_REGION"))
}

// TestUseRegionDefaultsToConfig: without --region, the account's
// config-resolved region is used (here auth.region=us-east-1, since
// accounts.dev has no region). Guards the default against regression.
func TestUseRegionDefaultsToConfig(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	run(t, "login")

	out := run(t, "shell-switch", "use", "dev")
	require.Equal(t, "us-east-1", exportValue(out, "AWS_REGION"))
	require.Equal(t, "us-east-1", exportValue(out, "AWS_DEFAULT_REGION"))
}

// TestUseRegionRejectsUnsafe: a --region carrying shell metacharacters is
// rejected with a clear error before anything is exported for eval.
func TestUseRegionRejectsUnsafe(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	run(t, "login")

	_, _, err := runOutErr(t, "shell-switch", "use", "dev", "--region", "bad;rm -rf ~")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid --region")
}

func TestUseRegionRejectsValueOutsideAllowlist(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	run(t, "login")

	stdout, _, err := runOutErr(t, "shell-switch", "use", "dev", "--region", "eu-central-1")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Contains(t, err.Error(), "eu-central-1")
	require.Contains(t, err.Error(), "allowed")
}

// TestStatusPrefersEnvRegion: `opsx status` shows the terminal's actual
// AWS_REGION as authoritative, so a `--region` override (or any manual
// AWS_REGION) is reflected rather than the config-derived region.
func TestStatusPrefersEnvRegion(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	t.Setenv("AWS_PROFILE", "dev.admin")
	t.Setenv("AWS_REGION", "eu-central-1") // not any configured region

	out := run(t, "status")
	require.Contains(t, out, "eu-central-1")
}

func TestUseRegionWhitespaceIsRejected(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	run(t, "login")
	// Whitespace-padded values must be rejected outright, not trimmed into
	// validity; pure whitespace must not be treated as "not provided".
	for _, bad := range []string{" us-west-2 ", "\tus-west-2", "   "} {
		t.Run(bad, func(t *testing.T) {
			stdout, _, err := runOutErr(t, "shell-switch", "use", "dev", "--region", bad)
			require.Error(t, err, "value %q must be rejected", bad)
			require.Empty(t, stdout)
		})
	}
}
