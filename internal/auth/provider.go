// Package auth owns the only company-specific seam in opsx: fetching an Entra
// SAML assertion. Everything downstream (AssumeRoleWithSAML, caching) is native
// and stable. Nothing outside this package knows about Entra or HTTP.
package auth

import (
	"context"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

// MasterRole identifies which master role a login targets. It carries the mode
// token; the concrete role ARN is composed by config.MasterRoleARN(mode).
type MasterRole struct{ mode string }

// Mode returns the mode token for the role.
func (r MasterRole) Mode() string { return r.mode }

// String implements fmt.Stringer.
func (r MasterRole) String() string { return r.mode }

// MasterRoleFromMode wraps a validated mode token as a MasterRole.
func MasterRoleFromMode(mode string) (MasterRole, error) {
	mode, err := config.NormalizeMode(mode)
	if err != nil {
		return MasterRole{}, err
	}
	return MasterRole{mode: mode}, nil
}

// SAMLProvider is the single auth seam. FetchAssertion performs Entra auth over
// HTTP with interactive MFA (proxy honored via system env) and returns a
// base64-encoded SAML assertion for the requested master role.
type SAMLProvider interface {
	FetchAssertion(ctx context.Context, role MasterRole) (assertion string, err error)
}
