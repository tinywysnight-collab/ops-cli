package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/creds"
	"github.com/tinywysnight-collab/ops-cli/internal/state"
)

// LoginService orchestrates `opsx login`: fetch a SAML assertion through the
// seam, assume the master role, and cache the credentials with their expiry.
type LoginService struct {
	Cfg        *config.Config
	Provider   SAMLProvider
	STS        AssumeWithSAMLAPI
	STSFactory func(context.Context) (AssumeWithSAMLAPI, error)
	Creds      *creds.Store
	State      *state.Store
	Now        func() time.Time
}

// Login authenticates for the given mode and caches master credentials. Both
// master profiles (admin and opr) can be cached independently without one
// overwriting the other.
func (s *LoginService) Login(ctx context.Context, mode string) error {
	mode, err := config.NormalizeMode(mode)
	if err != nil {
		return err
	}
	role, err := MasterRoleFromMode(mode)
	if err != nil {
		return err
	}

	assertion, err := s.Provider.FetchAssertion(ctx, role)
	if err != nil {
		return fmt.Errorf("fetch SAML assertion: %w", err)
	}

	roleARN, err := s.Cfg.MasterRoleARN(mode)
	if err != nil {
		return err
	}
	principal := s.Cfg.Auth.SAMLProviderARN
	if principal == "" {
		return fmt.Errorf("auth.saml_provider_arn is not set")
	}

	stsAPI, err := s.sts(ctx)
	if err != nil {
		return err
	}
	c, expiry, err := assumeMaster(ctx, stsAPI, principal, roleARN, assertion)
	if err != nil {
		return err
	}

	profile, err := config.MasterProfile(mode)
	if err != nil {
		return err
	}
	if err := s.Creds.Write(ctx, profile, c); err != nil {
		return err
	}
	return s.State.Put(ctx, profile, state.Entry{
		Expiry:    expiry,
		Account:   "master",
		Mode:      mode,
		UpdatedAt: s.now(),
	})
}

func (s *LoginService) sts(ctx context.Context) (AssumeWithSAMLAPI, error) {
	if s.STSFactory != nil {
		stsAPI, err := s.STSFactory(ctx)
		if err != nil {
			return nil, err
		}
		if stsAPI == nil {
			return nil, fmt.Errorf("login STS factory returned nil")
		}
		return stsAPI, nil
	}
	if s.STS == nil {
		return nil, fmt.Errorf("login STS client is not configured")
	}
	return s.STS, nil
}

func (s *LoginService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
