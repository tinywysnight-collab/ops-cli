package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

func TestMasterProfile(t *testing.T) {
	admin, err := config.MasterProfile("admin")
	require.NoError(t, err)
	require.Equal(t, "master_admin", admin)

	opr, err := config.MasterProfile("opr")
	require.NoError(t, err)
	require.Equal(t, "master_awsopr", opr)

	prod, err := config.MasterProfile("prod-admin")
	require.NoError(t, err)
	require.Equal(t, "master_prod-admin", prod)
}

func TestCitizenProfileIncludesRole(t *testing.T) {
	require.Equal(t, "dev.admin.Admin", config.CitizenProfile("dev", "admin", "Admin"))
	require.Equal(t, "dev.admin.BAU", config.CitizenProfile("dev", "admin", "BAU"))
}
