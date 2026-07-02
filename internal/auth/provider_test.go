package auth_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/auth"
)

func TestMasterRoleFromModeCarriesMode(t *testing.T) {
	r, err := auth.MasterRoleFromMode("prod-admin")
	require.NoError(t, err)
	require.Equal(t, "prod-admin", r.Mode())
	require.Equal(t, "prod-admin", r.String())
}
