package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

const validYAML = `
regions:
  - ap-southeast-2
  - us-east-1
accounts:
  dev:
    account_id: "111111111111"
    description: "Dev citizen account"
    region: ap-southeast-2
  prod:
    account_id: "222222222222"
    description: "Prod citizen account"
    region: us-east-1
clusters:
  dev-syd:
    account: dev
    region: ap-southeast-2
    name: dev-eks-cluster-01
auth:
  master_account_id: "000000000000"
  saml_provider_arn: "arn:aws:iam::000000000000:saml-provider/EntraID"
  entra:
    app_id: "a"
    username: "u"
    debug: true
    base_url: "https://login.example.com"
    ms_login_url: "https://ms.example.com/login"
    myapps_url: "https://apps.example.com/myapps"
  master_roles:
    admin: master_admin
    opr: master_AWSOpr
  citizen_roles:
    admin: Admin
    opr: AWSOpr
`

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestLoad(t *testing.T) {
	t.Run("valid config decodes all sections", func(t *testing.T) {
		c, err := config.Load(writeConfig(t, validYAML))
		require.NoError(t, err)
		require.NoError(t, c.Validate())

		require.Equal(t, []string{"ap-southeast-2", "us-east-1"}, c.Regions)
		require.Equal(t, "111111111111", c.Accounts["dev"].AccountID)
		require.Equal(t, "Dev citizen account", c.Accounts["dev"].Description)
		require.Equal(t, "ap-southeast-2", c.Clusters["dev-syd"].Region)
		require.Equal(t, "dev-eks-cluster-01", c.Clusters["dev-syd"].Name)
		require.Equal(t, "000000000000", c.Auth.MasterAccountID)
		require.True(t, c.Auth.Entra.Debug)
		require.Equal(t, "https://login.example.com", c.Auth.Entra.BaseURL)
		require.Equal(t, "https://ms.example.com/login", c.Auth.Entra.MSLoginURL)
		require.Equal(t, "https://apps.example.com/myapps", c.Auth.Entra.MyAppsURL)
		require.Equal(t, "master_admin", c.Auth.MasterRoles["admin"])
		require.Equal(t, "AWSOpr", c.Auth.CitizenRoles["opr"])
	})

	t.Run("missing file gives a clear, path-bearing error", func(t *testing.T) {
		_, err := config.Load(filepath.Join(t.TempDir(), "nope.yaml"))
		require.Error(t, err)
		require.ErrorIs(t, err, config.ErrConfigNotFound)
		require.Contains(t, err.Error(), "nope.yaml")
	})

	t.Run("deprecated entra fields are rejected", func(t *testing.T) {
		body := `
accounts: {}
clusters: {}
auth:
  master_account_id: "000000000000"
  saml_provider_arn: "arn:aws:iam::000000000000:saml-provider/EntraID"
  entra:
    tenant_id: "legacy"
    domain_map:
      example.com:
        app_id: "legacy-app"
  master_roles: {admin: master_admin, opr: master_AWSOpr}
  citizen_roles: {admin: Admin, opr: AWSOpr}
`
		_, err := config.Load(writeConfig(t, body))
		require.Error(t, err)
		require.Contains(t, err.Error(), "field tenant_id not found")
		require.Contains(t, err.Error(), "field domain_map not found")
	})
}

func TestExampleConfig(t *testing.T) {
	t.Run("sample omits optional entra endpoint overrides", func(t *testing.T) {
		c, err := config.Load(filepath.FromSlash("../../testdata/config.example.yaml"))
		require.NoError(t, err)
		require.NoError(t, c.Validate())
		require.Empty(t, c.Auth.Entra.BaseURL)
		require.Empty(t, c.Auth.Entra.MSLoginURL)
		require.Empty(t, c.Auth.Entra.MyAppsURL)
		require.False(t, c.Auth.Entra.Debug)
	})
}

func TestValidate(t *testing.T) {
	t.Run("cluster referencing unknown account is rejected", func(t *testing.T) {
		body := `
regions: [ap-southeast-2]
accounts:
  dev:
    account_id: "111111111111"
    region: ap-southeast-2
clusters:
  ghost:
    account: missing
    region: ap-southeast-2
    name: x
auth:
  master_account_id: "0"
  master_roles: {admin: a, opr: o}
  citizen_roles: {admin: a, opr: o}
`
		c, err := config.Load(writeConfig(t, body))
		require.NoError(t, err)
		err = c.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "ghost")
		require.Contains(t, err.Error(), "missing")
	})
}

func TestNormalizeMode(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"admin", "admin", false},
		{"opr", "opr", false},
		{"Admin", "", true},
		{"awsopr", "", true},
		{"", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := config.NormalizeMode(tc.in)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestARNComposition(t *testing.T) {
	c, err := config.Load(writeConfig(t, validYAML))
	require.NoError(t, err)

	t.Run("master and citizen ARNs are composed from config", func(t *testing.T) {
		master, err := c.MasterRoleARN("admin")
		require.NoError(t, err)
		require.Equal(t, "arn:aws:iam::000000000000:role/master_admin", master)

		citizen, err := c.CitizenRoleARN("prod", "opr")
		require.NoError(t, err)
		require.Equal(t, "arn:aws:iam::222222222222:role/AWSOpr", citizen)
	})

	t.Run("changing role names changes the ARNs (config-driven, no hardcoding)", func(t *testing.T) {
		c.Auth.MasterRoles["admin"] = "Renamed_Master"
		c.Auth.CitizenRoles["admin"] = "Renamed_Citizen"

		master, err := c.MasterRoleARN("admin")
		require.NoError(t, err)
		require.Equal(t, "arn:aws:iam::000000000000:role/Renamed_Master", master)

		citizen, err := c.CitizenRoleARN("dev", "admin")
		require.NoError(t, err)
		require.Equal(t, "arn:aws:iam::111111111111:role/Renamed_Citizen", citizen)
	})

	t.Run("unknown account alias errors", func(t *testing.T) {
		_, err := c.CitizenRoleARN("ghost", "admin")
		require.Error(t, err)
		require.Contains(t, err.Error(), "ghost")
	})
}
