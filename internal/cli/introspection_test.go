package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/state"
)

func TestRenderLs(t *testing.T) {
	cfg := &config.Config{
		Accounts: map[string]config.Account{
			"prod": {AccountID: "222222222222", Region: "us-east-1"},
			"dev":  {AccountID: "111111111111", Description: "Dev citizen account", Region: "ap-southeast-2"},
		},
		Clusters: map[string]config.Cluster{
			"dev-syd": {Account: "dev", Region: "ap-southeast-2", Name: "dev-eks-cluster-01"},
		},
	}
	var b bytes.Buffer
	require.NoError(t, renderLs(&b, cfg))
	out := b.String()
	require.Contains(t, out, "ALIAS")
	require.Contains(t, out, "ACCOUNT ID")
	require.Contains(t, out, "DESCRIPTION")
	require.Contains(t, out, "111111111111")
	require.Contains(t, out, "Dev citizen account")
	require.Contains(t, out, "222222222222")
	require.Contains(t, out, "prod")
	require.Contains(t, out, " -")
	require.Contains(t, out, "NAME")
	require.Contains(t, out, "dev-eks-cluster-01")
	require.Less(t, strings.Index(out, "dev "), strings.Index(out, "prod "))
	require.NotContains(t, out, "REGIONS")
}

func TestRenderLsEmptySections(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, renderLs(&b, &config.Config{}))
	out := b.String()
	require.Contains(t, out, "Accounts:")
	require.Contains(t, out, "Clusters:")
	require.Equal(t, 2, bytesCount(out, "(none configured)"))
}

func TestRenderStatusNoContext(t *testing.T) {
	var b bytes.Buffer
	renderStatus(&b, terminalContext{}, map[string]state.Entry{}, time.Now())
	out := b.String()
	require.Contains(t, out, "No active opsx context")
	require.Contains(t, out, "opsx login")
}

func TestRenderStatusNoProfileWithRegion(t *testing.T) {
	var b bytes.Buffer
	renderStatus(&b, terminalContext{Region: "ap-southeast-2"}, map[string]state.Entry{}, time.Now())
	out := b.String()
	require.Contains(t, out, "No active opsx")
	require.Contains(t, out, "Region:         ap-southeast-2")
}

func TestRenderStatusActive(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	entries := map[string]state.Entry{
		"dev.admin": {Account: "dev", Mode: "admin", Cluster: "dev-syd", Expiry: now.Add(30 * time.Minute)},
	}
	var b bytes.Buffer
	renderStatus(&b, terminalContext{Profile: "dev.admin", Mode: "admin", KubeConfig: "/k/dev-syd.admin.yaml"}, entries, now)
	out := b.String()
	require.Contains(t, out, "Active profile: dev.admin")
	require.Contains(t, out, "Account:        dev")
	require.Contains(t, out, "Cluster:        dev-syd")
	require.NotContains(t, out, "EXPIRED")
}

// TestRenderStatusClusterReflectsThisTerminal asserts that when two terminals
// share the same <alias.mode> profile but have different KUBECONFIG clusters,
// each terminal's status reflects its own cluster (from KUBECONFIG), not the
// shared state entry's last-recorded cluster (13.3).
func TestRenderStatusClusterReflectsThisTerminal(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	entries := map[string]state.Entry{
		// Shared state recorded syd-2 last (e.g. by terminal B).
		"dev.admin": {Account: "dev", Mode: "admin", Cluster: "dev-syd-2", Expiry: now.Add(30 * time.Minute)},
	}

	var a bytes.Buffer
	renderStatus(&a, terminalContext{Profile: "dev.admin", Mode: "admin", Cluster: "dev-syd-1"}, entries, now)
	require.Contains(t, a.String(), "Cluster:        dev-syd-1")
	require.NotContains(t, a.String(), "dev-syd-2")

	var b bytes.Buffer
	renderStatus(&b, terminalContext{Profile: "dev.admin", Mode: "admin", Cluster: "dev-syd-2"}, entries, now)
	require.Contains(t, b.String(), "Cluster:        dev-syd-2")
}

// TestRenderStatusFallsBackToRecordedCluster asserts that with no per-terminal
// KUBECONFIG cluster, the state-derived cluster is shown but clearly labeled.
func TestRenderStatusFallsBackToRecordedCluster(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	entries := map[string]state.Entry{
		"dev.admin": {Account: "dev", Mode: "admin", Cluster: "dev-syd", Expiry: now.Add(30 * time.Minute)},
	}
	var b bytes.Buffer
	renderStatus(&b, terminalContext{Profile: "dev.admin", Mode: "admin"}, entries, now)
	require.Contains(t, b.String(), "Cluster:        dev-syd (last recorded for this profile)")
}

// TestRenderStatusModeReflectsActiveProfileNotStaleEnv pins that the Mode shown
// is the active profile's recorded mode, not a stale OPSX_MODE env default. A
// terminal can have OPSX_MODE=admin exported and then run `opsx use prod --opr`,
// which sets AWS_PROFILE=prod.opr but does NOT touch OPSX_MODE — status must
// report the credentials' real mode (opr), never the contradicting env value.
func TestRenderStatusModeReflectsActiveProfileNotStaleEnv(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	entries := map[string]state.Entry{
		"prod.opr": {Account: "prod", Mode: "opr", Expiry: now.Add(30 * time.Minute)},
	}
	var b bytes.Buffer
	renderStatus(&b, terminalContext{Profile: "prod.opr", Mode: "admin"}, entries, now)
	out := b.String()
	require.Contains(t, out, "Mode:           opr")
	require.NotContains(t, out, "Mode:           admin")
}

func TestRenderStatusExpired(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	entries := map[string]state.Entry{
		"dev.admin": {Account: "dev", Mode: "admin", Expiry: now.Add(-time.Minute)},
	}
	var b bytes.Buffer
	renderStatus(&b, terminalContext{Profile: "dev.admin"}, entries, now)
	out := b.String()
	require.Contains(t, out, "EXPIRED")
	require.Contains(t, out, "opsx login")
}

func TestRenderStatusForeignProfile(t *testing.T) {
	var b bytes.Buffer
	renderStatus(&b, terminalContext{Profile: "legacy"}, map[string]state.Entry{}, time.Now())
	out := b.String()
	require.Contains(t, out, "Active profile: legacy")
	require.Contains(t, out, "not opsx-managed")
	require.Contains(t, out, "no recorded expiry")
}

func bytesCount(s, sub string) int {
	count, idx := 0, 0
	for {
		i := indexFrom(s, sub, idx)
		if i < 0 {
			return count
		}
		count++
		idx = i + len(sub)
	}
}

func indexFrom(s, sub string, from int) int {
	if from > len(s) {
		return -1
	}
	i := bytes.Index([]byte(s[from:]), []byte(sub))
	if i < 0 {
		return -1
	}
	return from + i
}
