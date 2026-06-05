// Package auth owns the only company-specific seam in opsx: fetching an Entra
// SAML assertion. Everything downstream (AssumeRoleWithSAML, caching) is native
// and stable. Nothing outside this package knows about Entra or HTTP.
package auth

import (
	"context"
	"fmt"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

// MasterRole identifies which master role a login targets.
type MasterRole int

const (
	// RoleAdmin is the master_admin role (mode "admin").
	RoleAdmin MasterRole = iota
	// RoleOpr is the master_AWSOpr role (mode "opr").
	RoleOpr
)

// Mode returns the mode token ("admin"/"opr") for the role.
func (r MasterRole) Mode() string {
	if r == RoleOpr {
		return config.ModeOpr
	}
	return config.ModeAdmin
}

// String implements fmt.Stringer.
func (r MasterRole) String() string { return r.Mode() }

// MasterRoleFromMode maps a mode token to a MasterRole.
func MasterRoleFromMode(mode string) (MasterRole, error) {
	switch mode {
	case config.ModeAdmin:
		return RoleAdmin, nil
	case config.ModeOpr:
		return RoleOpr, nil
	default:
		return 0, fmt.Errorf("invalid mode %q", mode)
	}
}

// SAMLProvider is the single auth seam. FetchAssertion performs Entra auth over
// HTTP with interactive MFA (proxy honored via system env) and returns a
// base64-encoded SAML assertion for the requested master role.
type SAMLProvider interface {
	FetchAssertion(ctx context.Context, role MasterRole) (assertion string, err error)
}
