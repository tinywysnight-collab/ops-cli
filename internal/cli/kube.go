package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/paths"
	"github.com/tinywysnight-collab/ops-cli/internal/state"
)

// switchCluster ensures the cluster's account credentials for the current mode,
// runs `aws eks update-kubeconfig` into a per-(cluster,mode) file, and returns
// both the account profile and the kubeconfig path. Because config binds each
// cluster to an account, `opsx kube` switches the account profile too — not
// only KUBECONFIG — so plain `aws` commands and `opsx status` work after a bare
// `opsx kube` with no prior `opsx use`.
func switchCluster(ctx context.Context, alias, mode string) (profile, kubeconfigPath, region string, err error) {
	cfg, err := loadConfig()
	if err != nil {
		return "", "", "", err
	}
	cluster, ok := cfg.Clusters[alias]
	if !ok {
		return "", "", "", fmt.Errorf("unknown cluster alias %q", alias)
	}

	// Auto-ensure the cluster's account credentials for this mode (no MFA). No
	// region override: the cluster's own region (below) is authoritative for the
	// exported session region, so `opsx kube` never takes a `--region`.
	profile, _, err = switchAccount(ctx, cluster.Account, mode, "")
	if err != nil {
		return "", "", "", err
	}

	// The just-ensured citizen credentials' expiry, recorded by switchAccount,
	// is the source of truth for the cluster annotation below.
	ss, err := stateStore()
	if err != nil {
		return "", "", "", err
	}
	entry, _, err := ss.Get(profile)
	if err != nil {
		return "", "", "", err
	}

	kubeconfigPath, err = paths.KubeConfig(alias, mode)
	if err != nil {
		return "", "", "", err
	}
	svc := kubeServiceFactory()
	// Per-(cluster,mode) file write: the isolated KUBECONFIG each terminal
	// exports. No friendly alias — this path is unchanged.
	if err := svc.UpdateKubeconfig(ctx, cluster.Region, cluster.Name, kubeconfigPath, profile, ""); err != nil {
		return "", "", "", err
	}
	// Additive merge into the default ~/.kube/config so `kubectl` works with no
	// KUBECONFIG set (locked-down PowerShell / Command Prompt). The real EKS
	// cluster name (not the friendly opsx alias) becomes the context name and
	// current-context; the AWS CLI's own merge preserves the user's unrelated
	// contexts. This always reflects the latest `opsx kube`. Two clusters that
	// share a real name (e.g. same name across accounts/regions) map to one
	// context here — the latest switch wins — which the operator accepts by
	// choosing real-name contexts over the collision-free ARN.
	defaultKubeconfig, err := paths.DefaultKubeConfig()
	if err != nil {
		return "", "", "", err
	}
	if err := svc.UpdateKubeconfig(ctx, cluster.Region, cluster.Name, defaultKubeconfig, profile, cluster.Name); err != nil {
		return "", "", "", err
	}

	// Record the active cluster on the profile's state entry so `opsx status`
	// can display it.
	if err := recordCluster(ctx, profile, cluster.Account, mode, alias, entry.Expiry); err != nil {
		return "", "", "", err
	}
	return profile, kubeconfigPath, cluster.Region, nil
}

// recordCluster sets the cluster on the profile's state entry, preserving the
// latest expiry/account/mode if another command refreshed the same profile.
// When no entry exists yet, it seeds one with the supplied credential expiry so
// `opsx status` never reports valid credentials as EXPIRED (a zero expiry would).
func recordCluster(ctx context.Context, profile, account, mode, cluster string, expiry time.Time) error {
	ss, err := stateStore()
	if err != nil {
		return err
	}
	return ss.Update(ctx, profile, func(entry state.Entry, ok bool) (state.Entry, error) {
		if !ok {
			entry.Account = account
			entry.Mode = mode
			entry.Expiry = expiry
			entry.UpdatedAt = time.Now()
		}
		entry.Cluster = cluster
		return entry, nil
	})
}

// printKubeConfirmation writes the two human-readable confirmation lines that
// `opsx kube` must show after a successful switch: one for the account/profile
// switch (so the operator sees their `aws` identity changed too, not only
// KUBECONFIG) and one for the EKS cluster switch. Both lines go to stderr so
// stdout stays export-only for `eval`. It is shared by the bare `opsx kube` path
// and the installed-shell-function `shell-switch kube` path.
func printKubeConfirmation(w io.Writer, alias, mode, profile, kubeconfigPath string) {
	fmt.Fprintf(w, "switched account → AWS_PROFILE=%s (mode %s)\n", profile, mode)
	fmt.Fprintf(w, "switched cluster → %s (KUBECONFIG=%s)\n", alias, kubeconfigPath)
}

func newKubeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "kube <cluster-alias>",
		Short: "Switch the current terminal to an EKS cluster by alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolveMode(cmd)
			if err != nil {
				return err
			}
			profile, kubeconfigPath, _, err := switchCluster(cmd.Context(), args[0], mode)
			if err != nil {
				return err
			}
			printKubeConfirmation(cmd.ErrOrStderr(), args[0], mode, profile, kubeconfigPath)
			return nil
		},
	}
}
