package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

func TestMasterRoleARNErrors(t *testing.T) {
	t.Run("missing master_account_id", func(t *testing.T) {
		c := &config.Config{Auth: config.Auth{MasterRoles: map[string]string{"admin": "r"}}}
		_, err := c.MasterRoleARN("admin")
		require.ErrorContains(t, err, "master_account_id")
	})
	t.Run("no role for mode", func(t *testing.T) {
		c := &config.Config{Auth: config.Auth{MasterAccountID: "1", MasterRoles: map[string]string{}}}
		_, err := c.MasterRoleARN("admin")
		require.ErrorContains(t, err, "master_roles")
	})
	t.Run("invalid mode", func(t *testing.T) {
		c := &config.Config{Auth: config.Auth{MasterAccountID: "1"}}
		_, err := c.MasterRoleARN("nope")
		require.Error(t, err)
	})
}

func TestCitizenRoleARNErrors(t *testing.T) {
	base := &config.Config{
		Accounts: map[string]config.Account{"dev": {AccountID: ""}, "ok": {AccountID: "111"}},
		Auth:     config.Auth{CitizenRoles: map[string]string{"admin": "Admin"}},
	}
	t.Run("account without account_id", func(t *testing.T) {
		_, err := base.CitizenRoleARN("dev", "admin", "")
		require.ErrorContains(t, err, "account_id")
	})
	t.Run("no citizen role for mode", func(t *testing.T) {
		_, err := base.CitizenRoleARN("ok", "opr", "")
		require.ErrorContains(t, err, "citizen_roles")
	})
	t.Run("invalid mode", func(t *testing.T) {
		_, err := base.CitizenRoleARN("ok", "nope", "")
		require.Error(t, err)
	})
}

func TestProfileNaming(t *testing.T) {
	admin, err := config.MasterProfile("admin")
	require.NoError(t, err)
	require.Equal(t, "master_admin", admin)

	opr, err := config.MasterProfile("opr")
	require.NoError(t, err)
	require.Equal(t, "master_awsopr", opr)

	// "nope" is a well-formed token, not a configured mode: MasterProfile only
	// validates format (NormalizeMode), so it falls back to master_<mode>.
	// Whether "nope" is actually configured is enforced by Validate/*RoleARN.
	nope, err := config.MasterProfile("nope")
	require.NoError(t, err)
	require.Equal(t, "master_nope", nope)

	_, err = config.MasterProfile("has.dot")
	require.Error(t, err)

	require.Equal(t, "dev.admin.Admin", config.CitizenProfile("dev", "admin", "Admin"))
	require.Equal(t, "prod.opr.BAU", config.CitizenProfile("prod", "opr", "BAU"))
}

func TestLoadParseError(t *testing.T) {
	p := writeConfig(t, "accounts: [this is not a map")
	_, err := config.Load(p)
	require.ErrorContains(t, err, "parse config")
}
