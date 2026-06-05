package cli

import (
	"fmt"
	"io"
	"sort"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

// renderLs writes configured accounts and clusters to w. Empty sections print a
// friendly "none configured" line rather than nothing.
func renderLs(w io.Writer, cfg *config.Config) {
	fmt.Fprintln(w, "Accounts:")
	if len(cfg.Accounts) == 0 {
		fmt.Fprintln(w, "  (none configured)")
	} else {
		for _, alias := range sortedKeys(accountAliases(cfg)) {
			a := cfg.Accounts[alias]
			line := "  " + alias
			if a.Description != "" {
				line += " — " + a.Description
			}
			// Show the account's configured STS/home region alongside its
			// description. When unset it falls back to auth.region, so it is
			// omitted here rather than shown as empty.
			if a.Region != "" {
				line += fmt.Sprintf(" (region %s)", a.Region)
			}
			fmt.Fprintln(w, line)
		}
	}

	fmt.Fprintln(w, "Clusters:")
	if len(cfg.Clusters) == 0 {
		fmt.Fprintln(w, "  (none configured)")
		return
	}
	for _, alias := range sortedKeys(clusterAliases(cfg)) {
		c := cfg.Clusters[alias]
		fmt.Fprintf(w, "  %s — account %s, region %s\n", alias, c.Account, c.Region)
	}
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
			renderLs(cmd.OutOrStdout(), cfg)
			return nil
		},
	}
}
