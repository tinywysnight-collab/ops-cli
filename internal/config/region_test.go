package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

func regionConfig() *config.Config {
	return &config.Config{
		Regions: []string{"ap-southeast-2", "us-east-1", "eu-west-1", "ap-south-1"},
		Accounts: map[string]config.Account{
			"dev":  {AccountID: "111111111111", Region: "ap-southeast-2"},
			"prod": {AccountID: "222222222222", Region: "us-east-1"},
		},
		Auth: config.Auth{
			MasterAccountID: "000000000000",
			SAMLProviderARN: "arn:aws:iam::000000000000:saml-provider/EntraID",
			Region:          "us-east-1",
			MasterRoles:     map[string]string{"admin": "master_admin", "opr": "master_AWSOpr"},
			CitizenRoles:    map[string]string{"admin": "Admin", "opr": "AWSOpr"},
		},
	}
}

func TestResolveMasterRegion(t *testing.T) {
	t.Run("auth.region wins over env", func(t *testing.T) {
		t.Setenv("AWS_REGION", "eu-west-1")
		r, err := regionConfig().ResolveMasterRegion()
		require.NoError(t, err)
		require.Equal(t, "us-east-1", r)
	})

	t.Run("falls back to AWS_REGION", func(t *testing.T) {
		t.Setenv("AWS_REGION", "eu-west-1")
		t.Setenv("AWS_DEFAULT_REGION", "")
		c := regionConfig()
		c.Auth.Region = ""
		r, err := c.ResolveMasterRegion()
		require.NoError(t, err)
		require.Equal(t, "eu-west-1", r)
	})

	t.Run("falls back to AWS_DEFAULT_REGION", func(t *testing.T) {
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "ap-south-1")
		c := regionConfig()
		c.Auth.Region = ""
		r, err := c.ResolveMasterRegion()
		require.NoError(t, err)
		require.Equal(t, "ap-south-1", r)
	})

	t.Run("no region resolves yields clear error", func(t *testing.T) {
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")
		c := regionConfig()
		c.Auth.Region = ""
		_, err := c.ResolveMasterRegion()
		require.Error(t, err)
		require.Contains(t, err.Error(), "auth.region")
	})
}

func TestResolveCitizenRegion(t *testing.T) {
	t.Run("per-account region wins", func(t *testing.T) {
		t.Setenv("AWS_REGION", "eu-west-1")
		r, err := regionConfig().ResolveCitizenRegion("dev")
		require.NoError(t, err)
		require.Equal(t, "ap-southeast-2", r)
	})

	t.Run("uses required account region", func(t *testing.T) {
		t.Setenv("AWS_REGION", "eu-west-1")
		r, err := regionConfig().ResolveCitizenRegion("prod")
		require.NoError(t, err)
		require.Equal(t, "us-east-1", r)
	})

	t.Run("missing account region is rejected", func(t *testing.T) {
		c := regionConfig()
		c.Accounts["prod"] = config.Account{AccountID: "222222222222"}
		_, err := c.ResolveCitizenRegion("prod")
		require.Error(t, err)
		require.Contains(t, err.Error(), "accounts.prod.region")
	})
}

func TestValidateRejectsWhitespaceRegions(t *testing.T) {
	t.Run("account region with whitespace", func(t *testing.T) {
		c := regionConfig()
		c.Accounts["dev"] = config.Account{AccountID: "111111111111", Region: "ap southeast 2"}
		err := c.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "dev")
	})

	t.Run("auth region with whitespace", func(t *testing.T) {
		c := regionConfig()
		c.Auth.Region = " us east 1 "
		err := c.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "auth.region")
	})
}
