package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/creds"
	"github.com/tinywysnight-collab/ops-cli/internal/state"
)

func TestLogoutPlanDefaultMode(t *testing.T) {
	entries := map[string]state.Entry{
		"master_admin":  {Account: "master", Mode: "admin"},
		"dev.admin":     {Account: "dev", Mode: "admin"},
		"prod.opr":      {Account: "prod", Mode: "opr"},
		"master_awsopr": {Account: "master", Mode: "opr"},
		"legacy":        {Account: "", Mode: ""},
	}
	got, err := logoutProfiles("admin", false, entries)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"master_admin", "dev.admin"}, got)
}

func TestLogoutPlanAll(t *testing.T) {
	entries := map[string]state.Entry{
		"master_admin":  {Account: "master", Mode: "admin"},
		"dev.admin":     {Account: "dev", Mode: "admin"},
		"prod.opr":      {Account: "prod", Mode: "opr"},
		"master_awsopr": {Account: "master", Mode: "opr"},
	}
	got, err := logoutProfiles("admin", true, entries)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"master_admin", "master_awsopr", "dev.admin", "prod.opr"}, got)
}

func TestRunLogoutPurgesCredentialsAndState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OPSX_CONFIG_DIR", filepath.Join(dir, "opsx"))
	t.Setenv("OPSX_CREDENTIALS_FILE", filepath.Join(dir, "aws", "credentials"))
	cs, err := credStore()
	require.NoError(t, err)
	ss, err := stateStore()
	require.NoError(t, err)
	now := time.Now()
	require.NoError(t, cs.Write(context.Background(), "master_admin", creds.Credentials{AccessKeyID: "M", SecretAccessKey: "S", SessionToken: "T"}))
	require.NoError(t, cs.Write(context.Background(), "dev.admin", creds.Credentials{AccessKeyID: "D", SecretAccessKey: "S", SessionToken: "T"}))
	require.NoError(t, cs.Write(context.Background(), "legacy", creds.Credentials{AccessKeyID: "L", SecretAccessKey: "S", SessionToken: "T"}))
	require.NoError(t, ss.Put(context.Background(), "master_admin", state.Entry{Account: "master", Mode: "admin", UpdatedAt: now}))
	require.NoError(t, ss.Put(context.Background(), "dev.admin", state.Entry{Account: "dev", Mode: "admin", UpdatedAt: now}))

	_, err = runLogout(context.Background(), "admin", false)
	require.NoError(t, err)

	_, ok, err := cs.Read("master_admin")
	require.NoError(t, err)
	require.False(t, ok)
	_, ok, err = cs.Read("dev.admin")
	require.NoError(t, err)
	require.False(t, ok)
	_, ok, err = cs.Read("legacy")
	require.NoError(t, err)
	require.True(t, ok)
	entries, err := ss.Load()
	require.NoError(t, err)
	require.NotContains(t, entries, "master_admin")
	require.NotContains(t, entries, "dev.admin")

	data, err := os.ReadFile(filepath.Join(dir, "aws", "credentials"))
	require.NoError(t, err)
	require.Contains(t, string(data), "[legacy]")
}
