package creds

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/state"
)

// TestUseUnderSharedLockSingleFlightsAcrossServiceInstances simulates two opsx
// PROCESSES switching to the same profile simultaneously: two CitizenService
// instances share the same files but are driven through the locked transaction
// directly, bypassing the in-process per-profile mutex that would otherwise
// mask the cross-process path. The credential-store "Single-flight citizen
// switch" scenario requires at most one AssumeRole; the second arrival must
// re-check reuse under the lock and take what the first process wrote.
func TestUseUnderSharedLockSingleFlightsAcrossServiceInstances(t *testing.T) {
	now := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".opsx.lock")
	cs := NewStore(filepath.Join(dir, "credentials"), lockPath)
	ss := state.NewStore(filepath.Join(dir, "state.json"), lockPath)

	cfg := &config.Config{
		Regions:  []string{"us-east-1"},
		Accounts: map[string]config.Account{"dev": {AccountID: "111111111111", Region: "us-east-1"}},
		Auth: config.Auth{
			MasterAccountID: "000000000000",
			Region:          "us-east-1",
			MasterRoles:     map[string]string{"admin": "master_admin", "opr": "master_AWSOpr"},
			CitizenRoles:    map[string]string{"admin": "Admin", "opr": "AWSOpr"},
		},
	}
	require.NoError(t, cs.Write(context.Background(), "master_admin", Credentials{AccessKeyID: "MASTER", SecretAccessKey: "ms", SessionToken: "mt"}))
	require.NoError(t, ss.Put(context.Background(), "master_admin", state.Entry{Expiry: now.Add(30 * time.Minute), Account: "master", Mode: "admin", UpdatedAt: now}))

	var mu sync.Mutex
	assumeCalls := 0
	newService := func() *CitizenService {
		return &CitizenService{
			Cfg: cfg, Creds: cs, State: ss, Now: func() time.Time { return now },
			Assume: func(context.Context, Credentials, string, string, string) (Credentials, time.Time, error) {
				mu.Lock()
				assumeCalls++
				mu.Unlock()
				time.Sleep(20 * time.Millisecond)
				return Credentials{AccessKeyID: "FRESH", SecretAccessKey: "cs", SessionToken: "ct"}, now.Add(time.Hour), nil
			},
		}
	}
	services := []*CitizenService{newService(), newService()}

	var wg sync.WaitGroup
	errs := make(chan error, len(services))
	for _, svc := range services {
		wg.Add(1)
		go func(svc *CitizenService) {
			defer wg.Done()
			if err := svc.useUnderSharedLock(context.Background(), "dev.admin", "dev", "admin"); err != nil {
				errs <- err
			}
		}(svc)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, 1, assumeCalls)

	got, ok, err := cs.Read("dev.admin")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "FRESH", got.AccessKeyID)
}
