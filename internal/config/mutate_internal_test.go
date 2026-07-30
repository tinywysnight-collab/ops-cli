package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseEditableYAML(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "conventional document",
			body: "regions: [us-east-1]\naccounts: {}\nclusters: {}\nauth: {}\n",
		},
		{
			name:    "multiple documents",
			body:    "accounts: {}\n---\nclusters: {}\n",
			wantErr: "multiple",
		},
		{
			name:    "anchor",
			body:    "accounts: &accounts {}\nclusters: {}\n",
			wantErr: "anchor",
		},
		{
			name:    "alias",
			body:    "accounts: &a {}\nclusters: *a\n",
			wantErr: "anchor",
		},
		{
			name:    "merge key",
			body:    "base: &base {region: us-east-1}\naccounts:\n  dev:\n    <<: *base\n",
			wantErr: "anchor",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parseEditableYAML([]byte(tt.body))
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, doc)
		})
	}
}

const editableConfig = `# top comment
regions:
  - us-east-1 # primary
  - us-west-2
accounts:
  dev: # keep account comment
    account_id: "111111111111"
    description: "Dev"
    region: us-east-1
clusters:
  dev-east:
    account: dev
    region: us-east-1
    name: "dev-eks"
auth:
  master_account_id: "000000000000"
  saml_provider_arn: "arn:aws:iam::000000000000:saml-provider/EntraID"
  region: us-east-1
  master_roles: {admin: master_admin, opr: master_AWSOpr}
  citizen_roles: {admin: Admin, opr: AWSOpr}
`

func TestEditableYAMLMutations(t *testing.T) {
	t.Run("append account preserves untouched content and validates", func(t *testing.T) {
		doc, err := parseEditableYAML([]byte(editableConfig))
		require.NoError(t, err)
		require.NoError(t, addAccountNode(doc, "prod", Account{
			AccountID:   "222222222222",
			Description: "Prod",
			Region:      "us-west-2",
		}))

		data, cfg, err := encodeAndValidate(doc)
		require.NoError(t, err)
		require.Equal(t, "222222222222", cfg.Accounts["prod"].AccountID)
		out := string(data)
		require.Contains(t, out, "# top comment")
		require.Contains(t, out, "# keep account comment")
		require.Contains(t, out, `account_id: "111111111111"`)
		require.Less(t, strings.Index(out, "dev:"), strings.Index(out, "prod:"))
		require.Less(t, strings.Index(out, "accounts:"), strings.Index(out, "clusters:"))
	})

	t.Run("append and target-delete cluster", func(t *testing.T) {
		doc, err := parseEditableYAML([]byte(editableConfig))
		require.NoError(t, err)
		require.NoError(t, addClusterNode(doc, "dev-west", Cluster{
			Account: "dev",
			Region:  "us-west-2",
			Name:    "dev-west-eks",
		}))
		require.NoError(t, deleteMappingEntry(doc, "clusters", "dev-east"))

		_, cfg, err := encodeAndValidate(doc)
		require.NoError(t, err)
		require.NotContains(t, cfg.Clusters, "dev-east")
		require.Contains(t, cfg.Clusters, "dev-west")
		require.Contains(t, cfg.Accounts, "dev")
	})

	t.Run("delete last entry leaves explicit empty mapping", func(t *testing.T) {
		doc, err := parseEditableYAML([]byte(editableConfig))
		require.NoError(t, err)
		require.NoError(t, deleteMappingEntry(doc, "clusters", "dev-east"))

		data, _, err := encodeAndValidate(doc)
		require.NoError(t, err)
		require.Contains(t, string(data), "clusters: {}")
	})

	t.Run("candidate validation rejects invalid account", func(t *testing.T) {
		doc, err := parseEditableYAML([]byte(editableConfig))
		require.NoError(t, err)
		require.NoError(t, addAccountNode(doc, "broken", Account{
			AccountID: "123",
			Region:    "us-east-1",
		}))

		_, _, err = encodeAndValidate(doc)
		require.Error(t, err)
		require.Contains(t, err.Error(), "broken")
	})
}
