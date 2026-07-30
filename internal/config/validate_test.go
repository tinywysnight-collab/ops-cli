package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

func fullValid() *config.Config {
	return &config.Config{
		Regions: []string{"ap-southeast-2", "us-east-1"},
		Accounts: map[string]config.Account{
			"dev":  {AccountID: "111111111111", Description: "dev", Region: "ap-southeast-2"},
			"prod": {AccountID: "222222222222", Region: "us-east-1"},
		},
		Clusters: map[string]config.Cluster{
			"dev-syd": {Account: "dev", Region: "ap-southeast-2", Name: "dev-eks"},
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

func TestValidateFull(t *testing.T) {
	require.NoError(t, fullValid().Validate())
}

func TestValidateRules(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*config.Config)
		wantSub string
	}{
		{"missing regions", func(c *config.Config) { c.Regions = nil }, "regions"},
		{"duplicate region", func(c *config.Config) {
			c.Regions = append(c.Regions, "ap-southeast-2")
		}, "duplicate"},
		{"whitespace allowlisted region", func(c *config.Config) {
			c.Regions = []string{"ap southeast 2", "us-east-1"}
		}, "whitespace"},
		{"account missing id", func(c *config.Config) { c.Accounts["dev"] = config.Account{} }, "dev"},
		{"account missing region", func(c *config.Config) {
			c.Accounts["dev"] = config.Account{AccountID: "111111111111"}
		}, "region"},
		{"account malformed id", func(c *config.Config) {
			c.Accounts["dev"] = config.Account{AccountID: "123"}
		}, "12 digits"},
		{"account id with whitespace", func(c *config.Config) {
			c.Accounts["dev"] = config.Account{AccountID: "1234 5678 9012"}
		}, "dev"},
		{"duplicate account id", func(c *config.Config) {
			c.Accounts["prod"] = config.Account{AccountID: "111111111111", Region: "us-east-1"}
		}, "same account_id"},
		{"hostile account alias", func(c *config.Config) {
			c.Accounts["dev;rm -rf ~"] = config.Account{AccountID: "999999999999", Region: "us-east-1"}
		}, "disallowed characters"},
		{"account region outside allowlist", func(c *config.Config) {
			c.Accounts["dev"] = config.Account{AccountID: "111111111111", Region: "eu-west-1"}
		}, "allowlist"},
		{"cluster missing region", func(c *config.Config) {
			c.Clusters["dev-syd"] = config.Cluster{Account: "dev", Name: "x"}
		}, "region"},
		{"cluster blank region", func(c *config.Config) {
			c.Clusters["dev-syd"] = config.Cluster{Account: "dev", Region: " ", Name: "x"}
		}, "region"},
		{"cluster whitespace region", func(c *config.Config) {
			c.Clusters["dev-syd"] = config.Cluster{Account: "dev", Region: "ap southeast 2", Name: "x"}
		}, "whitespace"},
		{"cluster region outside allowlist", func(c *config.Config) {
			c.Clusters["dev-syd"] = config.Cluster{Account: "dev", Region: "eu-west-1", Name: "x"}
		}, "allowlist"},
		{"cluster missing name", func(c *config.Config) {
			c.Clusters["dev-syd"] = config.Cluster{Account: "dev", Region: "r"}
		}, "name"},
		{"cluster unknown account", func(c *config.Config) {
			c.Clusters["dev-syd"] = config.Cluster{Account: "ghost", Region: "r", Name: "n"}
		}, "ghost"},
		{"hostile cluster alias", func(c *config.Config) {
			c.Clusters["c;evil"] = config.Cluster{Account: "dev", Region: "r", Name: "n"}
		}, "disallowed characters"},
		{"missing master account id", func(c *config.Config) { c.Auth.MasterAccountID = "" }, "master_account_id"},
		{"malformed master account id", func(c *config.Config) { c.Auth.MasterAccountID = "abc" }, "master_account_id"},
		{"missing saml provider", func(c *config.Config) { c.Auth.SAMLProviderARN = "" }, "saml_provider_arn"},
		{"auth region outside allowlist", func(c *config.Config) { c.Auth.Region = "eu-west-1" }, "allowlist"},
		{"missing master role for opr", func(c *config.Config) { delete(c.Auth.MasterRoles, "opr") }, "master_roles"},
		{"missing citizen role for admin", func(c *config.Config) { delete(c.Auth.CitizenRoles, "admin") }, "citizen_roles"},
		{"blank master role", func(c *config.Config) { c.Auth.MasterRoles["admin"] = " " }, "master_roles"},
		{"blank citizen role", func(c *config.Config) { c.Auth.CitizenRoles["opr"] = " " }, "citizen_roles"},
		{"blank entra app", func(c *config.Config) { c.Auth.Entra.AppID = " " }, "app_id"},
		{"blank entra username", func(c *config.Config) { c.Auth.Entra.Username = " " }, "username"},
		{"blank entra base url", func(c *config.Config) { c.Auth.Entra.BaseURL = " " }, "base_url"},
		{"malformed entra ms login url", func(c *config.Config) { c.Auth.Entra.MSLoginURL = "not a url" }, "ms_login_url"},
		{"unsupported entra myapps url scheme", func(c *config.Config) { c.Auth.Entra.MyAppsURL = "ftp://apps.example.com" }, "myapps_url"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := fullValid()
			tc.mutate(c)
			err := c.Validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantSub)
		})
	}
}
