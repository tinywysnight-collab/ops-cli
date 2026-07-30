package cli

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

// renderLs writes configured accounts and clusters to w. Empty sections print a
// friendly "none configured" line rather than nothing.
func renderLs(w io.Writer, cfg *config.Config) error {
	fmt.Fprintln(w, "Accounts:")
	if len(cfg.Accounts) == 0 {
		fmt.Fprintln(w, "  (none configured)")
	} else {
		table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(table, "ALIAS\tACCOUNT ID\tREGION\tDESCRIPTION")
		for _, alias := range sortedKeys(accountAliases(cfg)) {
			a := cfg.Accounts[alias]
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", alias, a.AccountID, a.Region, displayOptional(a.Description))
		}
		if err := table.Flush(); err != nil {
			return fmt.Errorf("flush accounts table: %w", err)
		}
	}

	fmt.Fprintln(w, "Clusters:")
	if len(cfg.Clusters) == 0 {
		fmt.Fprintln(w, "  (none configured)")
		return nil
	}
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ALIAS\tACCOUNT\tACCOUNT ID\tREGION\tNAME")
	for _, alias := range sortedKeys(clusterAliases(cfg)) {
		c := cfg.Clusters[alias]
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", alias, c.Account, cfg.Accounts[c.Account].AccountID, c.Region, c.Name)
	}
	if err := table.Flush(); err != nil {
		return fmt.Errorf("flush clusters table: %w", err)
	}
	return nil
}

func accountAliases(cfg *config.Config) []string {
	out := make([]string, 0, len(cfg.Accounts))
	for k := range cfg.Accounts {
		out = append(out, k)
	}
	return out
}

func clusterAliases(cfg *config.Config) []string {
	out := make([]string, 0, len(cfg.Clusters))
	for k := range cfg.Clusters {
		out = append(out, k)
	}
	return out
}

func sortedKeys(keys []string) []string {
	sort.Strings(keys)
	return keys
}

func newLsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List configured account and cluster aliases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			return renderLs(cmd.OutOrStdout(), cfg)
		},
	}
}
