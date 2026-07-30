package creds_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/creds"
	"github.com/tinywysnight-collab/ops-cli/internal/state"
)

// TestCrossTerminalNoCollision simulates two terminals operating different
// accounts/modes against the SAME shared credentials file. Each writes its own
// [alias.mode] profile; neither overwrites the other (structural isolation).
func TestCrossTerminalNoCollision(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cs, ss := newCitizenStores(t)

	cfg := &config.Config{
		Regions: []string{"us-east-1"},
		Accounts: map[string]config.Account{
			"dev":  {AccountID: "111111111111", Region: "us-east-1"},
			"prod": {AccountID: "222222222222", Region: "us-east-1"},
		},
		Auth: config.Auth{
			MasterAccountID: "000000000000",
			Region:          "us-east-1",
			MasterRoles:     map[string]string{"admin": "master_admin", "opr": "master_AWSOpr"},
			CitizenRoles:    map[string]string{"admin": "Admin", "opr": "AWSOpr"},
		},
	}

	// Both master roles are logged in and valid.
	for _, p := range []string{"master_admin", "master_awsopr"} {
		require.NoError(t, cs.Write(context.Background(), p, creds.Credentials{AccessKeyID: "M-" + p, SecretAccessKey: "s", SessionToken: "t"}))
		require.NoError(t, ss.Put(context.Background(), p, state.Entry{Expiry: now.Add(time.Hour), Mode: "admin", UpdatedAt: now}))
	}

	newSvc := func(tag string) *creds.CitizenService {
		return &creds.CitizenService{
			Cfg: cfg, Creds: cs, State: ss, Now: func() time.Time { return now },
			Assume: func(_ context.Context, _ creds.Credentials, roleARN, _, _ string) (creds.Credentials, time.Time, error) {
				return creds.Credentials{AccessKeyID: tag, SecretAccessKey: "s", SessionToken: "t"}, now.Add(time.Hour), nil
			},
		}
	}

	// Terminal A: non-prod as admin. Terminal B: prod as opr.
	pa, err := newSvc("CRED-A").Use(context.Background(), "dev", "admin")
	require.NoError(t, err)
	pb, err := newSvc("CRED-B").Use(context.Background(), "prod", "opr")
	require.NoError(t, err)

	require.Equal(t, "dev.admin", pa)
	require.Equal(t, "prod.opr", pb)
	require.NotEqual(t, pa, pb)

	a, ok, _ := cs.Read("dev.admin")
	require.True(t, ok)
	require.Equal(t, "CRED-A", a.AccessKeyID)
	b, ok, _ := cs.Read("prod.opr")
	require.True(t, ok)
	require.Equal(t, "CRED-B", b.AccessKeyID, "prod.opr must not be overwritten by dev.admin")
}
