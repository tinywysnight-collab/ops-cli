package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

func newAccountCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Interactively manage account configuration",
	}
	cmd.AddCommand(newAccountAddCommand(), newAccountDeleteCommand())
	return cmd
}

func newAccountAddCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "add",
		Short: "Interactively add an account",
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
			alias, cancelled, err := p.text("Alias", false, func(value string) error {
				if err := config.ValidateAlias(value); err != nil {
					return err
				}
				if _, exists := cfg.Accounts[value]; exists {
					return fmt.Errorf("account alias %q already exists", value)
				}
				return nil
			})
			if err != nil || cancelled {
				return finishCancellation(cmd, cancelled, err)
			}
			accountID, cancelled, err := p.text("Account ID", false, func(value string) error {
				if err := config.ValidateAccountID(value); err != nil {
					return err
				}
				for existingAlias, account := range cfg.Accounts {
					if account.AccountID == value {
						return fmt.Errorf("account ID %s is already used by %q", value, existingAlias)
					}
				}
				return nil
			})
			if err != nil || cancelled {
				return finishCancellation(cmd, cancelled, err)
			}
			description, cancelled, err := p.text("Description (optional)", true, func(string) error { return nil })
			if err != nil || cancelled {
				return finishCancellation(cmd, cancelled, err)
			}
			region, cancelled, err := p.selectOne("Region", cfg.Regions, "")
			if err != nil || cancelled {
				return finishCancellation(cmd, cancelled, err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Account to add:\n  alias: %s\n  account ID: %s\n  region: %s\n  description: %s\n",
				alias, accountID, region, displayOptional(description))
			confirmed, cancelled, err := p.confirm()
			if err != nil || cancelled || !confirmed {
				return finishCancellation(cmd, cancelled || !confirmed, err)
			}
			store, err := configStore()
			if err != nil {
				return err
			}
			if err := store.AddAccount(cmd.Context(), alias, config.Account{
				AccountID: accountID, Description: description, Region: region,
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added account %q.\n", alias)
			return nil
		},
	}
}

func finishCancellation(cmd *cobra.Command, cancelled bool, err error) error {
	if err != nil {
		return err
	}
	if cancelled {
		fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
	}
	return nil
}

func displayOptional(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func newAccountDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete",
		Short: "Interactively delete an account",
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
				fmt.Fprintln(cmd.OutOrStdout(), "No accounts configured.")
				return nil
			}
			aliases := accountAliases(cfg)
			sort.Strings(aliases)
			alias, cancelled, err := p.selectOne("Account", aliases, "")
			if err != nil || cancelled {
				return finishCancellation(cmd, cancelled, err)
			}
			var refs []string
			for clusterAlias, cluster := range cfg.Clusters {
				if cluster.Account == alias {
					refs = append(refs, clusterAlias)
				}
			}
			if len(refs) > 0 {
				sort.Strings(refs)
				return fmt.Errorf("cannot delete account %q; referenced by clusters: %s; delete these clusters first with `opsx cluster delete`", alias, strings.Join(refs, ", "))
			}
			account := cfg.Accounts[alias]
			fmt.Fprintf(cmd.ErrOrStderr(), "Account to delete:\n  alias: %s\n  account ID: %s\n  region: %s\n  description: %s\n",
				alias, account.AccountID, account.Region, displayOptional(account.Description))
			if strings.HasPrefix(os.Getenv("AWS_PROFILE"), alias+".") {
				fmt.Fprintln(cmd.ErrOrStderr(), "WARNING: this account is active in the current terminal; its environment will not be reset.")
			}
			confirmed, cancelled, err := p.confirm()
			if err != nil || cancelled || !confirmed {
				return finishCancellation(cmd, cancelled || !confirmed, err)
			}
			store, err := configStore()
			if err != nil {
				return err
			}
			if err := store.DeleteAccount(cmd.Context(), alias); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted account %q; only config was removed and runtime artifacts were retained.\n", alias)
			return nil
		},
	}
}
