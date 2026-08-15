package state_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/state"
)

func newTempStore(t *testing.T) (*state.Store, string) {
	t.Helper()
	dir := t.TempDir()
	statePath := filepath.Join(dir, "opsx", "state.json")
	lockPath := filepath.Join(dir, "opsx", ".opsx.lock")
	return state.NewStore(statePath, lockPath), statePath
}

func TestPutGetRecordsAllFields(t *testing.T) {
	ctx := context.Background()
	s, _ := newTempStore(t)
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	e := state.Entry{
		Expiry:    now.Add(time.Hour),
		Account:   "dev",
		Mode:      "admin",
		Cluster:   "dev-syd",
		UpdatedAt: now,
	}
	require.NoError(t, s.Put(ctx, "dev.admin", e))

	got, ok, err := s.Get("dev.admin")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, e.Account, got.Account)
	require.Equal(t, e.Mode, got.Mode)
	require.Equal(t, e.Cluster, got.Cluster)
	require.True(t, e.Expiry.Equal(got.Expiry))
	require.True(t, e.UpdatedAt.Equal(got.UpdatedAt))
}

func TestGetMissing(t *testing.T) {
	s, _ := newTempStore(t)
	_, ok, err := s.Get("nope")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestLoadEmptyWhenAbsent(t *testing.T) {
	s, _ := newTempStore(t)
	m, err := s.Load()
	require.NoError(t, err)
	require.Empty(t, m)
}

func TestPutPreservesOtherEntries(t *testing.T) {
	ctx := context.Background()
	s, _ := newTempStore(t)
	now := time.Now().UTC()
	require.NoError(t, s.Put(ctx, "dev.admin", state.Entry{Account: "dev", Mode: "admin", UpdatedAt: now}))
	require.NoError(t, s.Put(ctx, "prod.opr", state.Entry{Account: "prod", Mode: "opr", UpdatedAt: now}))

	m, err := s.Load()
	require.NoError(t, err)
	require.Len(t, m, 2)
	require.Equal(t, "dev", m["dev.admin"].Account)
	require.Equal(t, "prod", m["prod.opr"].Account)
}

func TestUpdateMergesUnderLock(t *testing.T) {
	ctx := context.Background()
	s, _ := newTempStore(t)
	oldExpiry := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	newExpiry := oldExpiry.Add(time.Hour)
	require.NoError(t, s.Put(ctx, "dev.admin", state.Entry{Expiry: oldExpiry, Account: "dev", Mode: "admin", UpdatedAt: oldExpiry}))

	require.NoError(t, s.Update(ctx, "dev.admin", func(e state.Entry, ok bool) (state.Entry, error) {
		require.True(t, ok)
		e.Expiry = newExpiry
		e.Cluster = "dev-syd"
		return e, nil
	}))

	got, ok, err := s.Get("dev.admin")
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, newExpiry.Equal(got.Expiry))
	require.Equal(t, "dev", got.Account)
	require.Equal(t, "admin", got.Mode)
	require.Equal(t, "dev-syd", got.Cluster)
}

func TestDeleteProfiles(t *testing.T) {
	ctx := context.Background()
	s, _ := newTempStore(t)
	now := time.Now()
	require.NoError(t, s.Put(ctx, "master_admin", state.Entry{Account: "master", Mode: "admin", UpdatedAt: now}))
	require.NoError(t, s.Put(ctx, "dev.admin", state.Entry{Account: "dev", Mode: "admin", UpdatedAt: now}))
	require.NoError(t, s.Put(ctx, "legacy", state.Entry{Account: "legacy", UpdatedAt: now}))

	require.NoError(t, s.Delete(ctx, []string{"master_admin", "dev.admin"}))

	entries, err := s.Load()
	require.NoError(t, err)
	require.NotContains(t, entries, "master_admin")
	require.NotContains(t, entries, "dev.admin")
	require.Contains(t, entries, "legacy")
}

func TestStatePermissions(t *testing.T) {
	ctx := context.Background()
	s, statePath := newTempStore(t)
	require.NoError(t, s.Put(ctx, "dev.admin", state.Entry{Account: "dev", Mode: "admin", UpdatedAt: time.Now()}))

	fi, err := os.Stat(statePath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), fi.Mode().Perm())

	di, err := os.Stat(filepath.Dir(statePath))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), di.Mode().Perm())
}

func TestLoadTreatsJSONNullAsEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "state.json")
	require.NoError(t, os.WriteFile(p, []byte("null"), 0o600))
	s := state.NewStore(p, filepath.Join(dir, ".opsx.lock"))
	m, err := s.Load()
	require.NoError(t, err)
	require.NotNil(t, m, "JSON null must not yield a nil map")
	// The nil-map panic this guards against happened on the next Put.
	require.NoError(t, s.Put(context.Background(), "dev.admin", state.Entry{Mode: "admin"}))
}
