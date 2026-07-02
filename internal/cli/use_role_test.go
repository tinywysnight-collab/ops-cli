package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUseRoleOverrideProfile(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	run(t, "login")
	out := run(t, "shell-switch", "use", "dev", "--role", "BAU")
	require.Equal(t, "dev.admin.BAU", exportValue(out, "AWS_PROFILE"))
}

func TestUseDefaultRoleProfile(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	run(t, "login")
	out := run(t, "shell-switch", "use", "dev")
	require.Equal(t, "dev.admin.Admin", exportValue(out, "AWS_PROFILE"))
}

func TestUseRoleRejectsUnsafe(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	run(t, "login")
	_, _, err := runOutErr(t, "shell-switch", "use", "dev", "--role", "bad;rm -rf ~")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid --role")
}
