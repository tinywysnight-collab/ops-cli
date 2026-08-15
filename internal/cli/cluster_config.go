package cli

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/paths"
)

func newClusterCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Interactively manage cluster configuration",
	}
	cmd.AddCommand(newClusterAddCommand(), newClusterDeleteCommand())
	return cmd
}

func newClusterAddCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "add",
		Short: "Interactively add a cluster",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p := newPromptSession(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), interactiveTerminal(cmd.InOrStdin()))
			if err := p.requireTTY(); err != nil {
				return err
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if len(cfg.Accounts) == 0 {
				return fmt.Errorf("no accounts configured; run `opsx account add` first")
			}
			alias, cancelled, err := p.text("Alias", false, func(value string) error {
				if err := config.ValidateAlias(value); err != nil {
					return err
				}
				if _, exists := cfg.Clusters[value]; exists {
					return fmt.Errorf("cluster alias %q already exists", value)
				}
				return nil
			})
			if err != nil || cancelled {
				return finishCancellation(cmd, cancelled, err)
			}
			accounts := accountAliases(cfg)
			sort.Strings(accounts)
			account, cancelled, err := p.selectOne("Account", accounts, "")
			if err != nil || cancelled {
				return finishCancellation(cmd, cancelled, err)
			}
			region, cancelled, err := p.selectOne("Region", cfg.Regions, "")
			if err != nil || cancelled {
				return finishCancellation(cmd, cancelled, err)
			}
			name, cancelled, err := p.text("EKS cluster name", false, func(value string) error {
				if value == "" {
					return fmt.Errorf("cluster name is required")
				}
				return nil
			})
			if err != nil || cancelled {
				return finishCancellation(cmd, cancelled, err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Cluster to add:\n  alias: %s\n  account: %s\n  region: %s\n  name: %s\n",
				alias, account, region, name)
			for existingAlias, existing := range cfg.Clusters {
				if existing.Account == account && existing.Region == region && existing.Name == name {
					fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: duplicate real cluster identity already configured as %q.\n", existingAlias)
				}
			}
			confirmed, cancelled, err := p.confirm()
			if err != nil || cancelled || !confirmed {
				return finishCancellation(cmd, cancelled || !confirmed, err)
			}
			store, err := configStore()
			if err != nil {
				return err
			}
			if err := store.AddCluster(cmd.Context(), alias, config.Cluster{
				Account: account, Region: region, Name: name,
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added cluster %q.\n", alias)
			return nil
		},
	}
}

func newClusterDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete",
		Short: "Interactively delete a cluster",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p := newPromptSession(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), interactiveTerminal(cmd.InOrStdin()))
			if err := p.requireTTY(); err != nil {
				return err
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if len(cfg.Clusters) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No clusters configured.")
				return nil
			}
			aliases := clusterAliases(cfg)
			sort.Strings(aliases)
			alias, cancelled, err := p.selectOne("Cluster", aliases, "")
			if err != nil || cancelled {
				return finishCancellation(cmd, cancelled, err)
			}
			cluster := cfg.Clusters[alias]
			fmt.Fprintf(cmd.ErrOrStderr(), "Cluster to delete:\n  alias: %s\n  account: %s\n  region: %s\n  name: %s\n",
				alias, cluster.Account, cluster.Region, cluster.Name)
			if active, ok := paths.ClusterFromKubeConfig(os.Getenv("KUBECONFIG")); ok && active == alias {
				fmt.Fprintln(cmd.ErrOrStderr(), "WARNING: this cluster is active in the current terminal; its environment will not be reset.")
			}
			confirmed, cancelled, err := p.confirm()
			if err != nil || cancelled || !confirmed {
				return finishCancellation(cmd, cancelled || !confirmed, err)
			}
			store, err := configStore()
			if err != nil {
				return err
			}
			if err := store.DeleteCluster(cmd.Context(), alias); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted cluster %q; only config was removed and runtime artifacts were retained.\n", alias)
			return nil
		},
	}
}
