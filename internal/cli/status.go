package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/creds"
	"github.com/tinywysnight-collab/ops-cli/internal/paths"
	"github.com/tinywysnight-collab/ops-cli/internal/state"
)

// terminalContext is the env-derived view of the current terminal.
type terminalContext struct {
	Profile    string
	KubeConfig string
	Mode       string
	// Cluster is the cluster alias derived from this terminal's KUBECONFIG. It
	// is authoritative for THIS terminal, unlike the shared, profile-keyed
	// state.json entry which another terminal may have overwritten.
	Cluster string
	// Region is the region of the active context: the active cluster's region
	// when a cluster is active, otherwise the account's resolved STS region. It
	// is resolved best-effort from config and left empty when unknown.
	Region string
}

// renderStatus writes the current terminal's active context. It is read-only:
// it never mutates credentials or state.
func renderStatus(w io.Writer, tc terminalContext, entries map[string]state.Entry, now time.Time) {
	if tc.Profile == "" {
		fmt.Fprintln(w, "No active opsx context in this terminal.")
		fmt.Fprintln(w, "  Run `opsx login` then `opsx use <account-alias>` to get started.")
		return
	}

	entry, ok := entries[tc.Profile]
	fmt.Fprintf(w, "Active profile: %s\n", tc.Profile)
	// The active profile's recorded mode is authoritative: it is the mode the
	// active credentials were actually minted under (the profile name encodes it).
	// OPSX_MODE is only a per-terminal default for the NEXT command and may
	// disagree with the active profile (e.g. `opsx use prod --opr` sets
	// AWS_PROFILE=prod.opr but never touches OPSX_MODE), so it must not override
	// the profile's real mode. It is shown only as a fallback for a profile opsx
	// has no state for, and is labeled as such.
	switch {
	case ok && entry.Mode != "":
		fmt.Fprintf(w, "Mode:           %s\n", entry.Mode)
	case tc.Mode != "":
		fmt.Fprintf(w, "Mode:           %s (OPSX_MODE default; profile not opsx-managed)\n", tc.Mode)
	}
	if ok && entry.Account != "" {
		fmt.Fprintf(w, "Account:        %s\n", entry.Account)
	}
	if tc.KubeConfig != "" {
		fmt.Fprintf(w, "KUBECONFIG:     %s\n", tc.KubeConfig)
	}
	// Prefer the cluster derived from THIS terminal's KUBECONFIG. Fall back to
	// the shared state entry only when this terminal has no opsx KUBECONFIG,
	// labeling it so it is never mistaken for this terminal's definitive cluster.
	switch {
	case tc.Cluster != "":
		fmt.Fprintf(w, "Cluster:        %s\n", tc.Cluster)
	case ok && entry.Cluster != "":
		fmt.Fprintf(w, "Cluster:        %s (last recorded for this profile)\n", entry.Cluster)
	}
	if tc.Region != "" {
		fmt.Fprintf(w, "Region:         %s\n", tc.Region)
	}

	switch {
	case !ok:
		fmt.Fprintln(w, "Managed by opsx: no (profile is not opsx-managed or has no recorded expiry)")
		fmt.Fprintln(w, "Expiry:         unknown (no recorded expiry — run `opsx use` for opsx-managed profiles)")
	case creds.IsExpired(entry.Expiry, now):
		fmt.Fprintf(w, "Expiry:         EXPIRED at %s — run: opsx login [--opr]\n", entry.Expiry.Format(time.RFC3339))
	default:
		fmt.Fprintf(w, "Expiry:         %s\n", entry.Expiry.Format(time.RFC3339))
	}
}

// resolveStatusRegion resolves the region of the active context, best-effort:
// the active cluster's region wins, otherwise the account's resolved STS region.
// It is read-only and never fails — a missing/invalid config or an unresolvable
// region simply yields "" so `opsx status` keeps working.
func resolveStatusRegion(tc terminalContext, entries map[string]state.Entry) string {
	cfg, err := loadConfig()
	if err != nil {
		return ""
	}
	cluster := tc.Cluster
	account := ""
	if entry, ok := entries[tc.Profile]; ok {
		if cluster == "" {
			cluster = entry.Cluster
		}
		account = entry.Account
	}
	if cluster != "" {
		if c, ok := cfg.Clusters[cluster]; ok {
			return c.Region
		}
	}
	if account != "" {
		if r, err := cfg.ResolveCitizenRegion(account); err == nil {
			return r
		}
	}
	return ""
}

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show this terminal's active account, mode, cluster, and expiry",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			tc := terminalContext{
				Profile:    os.Getenv("AWS_PROFILE"),
				KubeConfig: os.Getenv("KUBECONFIG"),
				Mode:       os.Getenv(EnvMode),
			}
			if cluster, ok := paths.ClusterFromKubeConfig(tc.KubeConfig); ok {
				tc.Cluster = cluster
			}
			entries := map[string]state.Entry{}
			if tc.Profile != "" {
				ss, err := stateStore()
				if err != nil {
					return err
				}
				entries, err = ss.Load()
				if err != nil {
					return err
				}
				// The terminal's actual AWS_REGION is authoritative: `opsx use
				// --region` (or any manual override) must be reflected here rather
				// than the config-derived region. Fall back to config only when the
				// terminal exported no region.
				if r := strings.TrimSpace(os.Getenv("AWS_REGION")); r != "" {
					tc.Region = r
				} else {
					tc.Region = resolveStatusRegion(tc, entries)
				}
			}
			renderStatus(cmd.OutOrStdout(), tc, entries, time.Now())
			return nil
		},
	}
}
