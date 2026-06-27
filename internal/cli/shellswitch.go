package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/shell"
)

// newShellSwitchCommand is the eval target invoked by the installed opsx shell
// function. Its subcommands print ONLY environment assignment lines for the
// selected shell dialect to stdout; all human output goes to stderr, so parent
// shell evaluation is safe.
func newShellSwitchCommand() *cobra.Command {
	opts := &shellSwitchOptions{dialect: string(shell.DialectPOSIX)}
	cmd := &cobra.Command{
		Use:   "shell-switch",
		Short: "Emit environment assignment lines (internal target of the shell function)",
	}
	cmd.PersistentFlags().StringVar(&opts.dialect, "shell", opts.dialect, "assignment dialect: posix|bash|zsh|powershell|cmd")
	cmd.AddCommand(
		newShellSwitchUseCommand(opts),
		newShellSwitchKubeCommand(opts),
		newShellSwitchModeCommand(opts),
	)
	return cmd
}

type shellSwitchOptions struct {
	dialect string
}

func (o shellSwitchOptions) parseDialect() (shell.Dialect, error) {
	return shell.ParseDialect(o.dialect)
}

func newShellSwitchUseCommand(opts *shellSwitchOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "use <account-alias>",
		Short: "Switch account and emit AWS_PROFILE assignment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolveMode(cmd)
			if err != nil {
				return err
			}
			profile, region, err := switchAccount(cmd.Context(), args[0], mode)
			if err != nil {
				return err
			}
			dialect, err := opts.parseDialect()
			if err != nil {
				return err
			}
			if err := printAssignments(cmd, dialect, profileRegionAssignments(profile, region)); err != nil {
				return err
			}
			return nil
		},
	}
}

func newShellSwitchKubeCommand(opts *shellSwitchOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "kube <cluster-alias>",
		Short: "Switch cluster (account profile + KUBECONFIG) and emit assignments",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolveMode(cmd)
			if err != nil {
				return err
			}
			profile, kubeconfig, region, err := switchCluster(cmd.Context(), args[0], mode)
			if err != nil {
				return err
			}
			dialect, err := opts.parseDialect()
			if err != nil {
				return err
			}
			// Emit BOTH the account profile and the kubeconfig so a bare
			// `opsx kube` (no prior `opsx use`) leaves AWS_PROFILE set for plain
			// `aws` commands and `opsx status`.
			assignments := append(profileRegionAssignments(profile, region), shell.Assignment{Key: "KUBECONFIG", Value: kubeconfig})
			if err := printAssignments(cmd, dialect, assignments); err != nil {
				return err
			}
			// Human confirmation goes to stderr so stdout stays export-only for
			// eval; the operator must see that AWS_PROFILE changed too.
			printKubeConfirmation(cmd.ErrOrStderr(), args[0], mode, profile, kubeconfig)
			return nil
		},
	}
}

func newShellSwitchModeCommand(opts *shellSwitchOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "mode <admin|opr>",
		Short: "Emit OPSX_MODE assignment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := normalizeModeArg(args[0])
			if err != nil {
				return err
			}
			dialect, err := opts.parseDialect()
			if err != nil {
				return err
			}
			return printAssignments(cmd, dialect, []shell.Assignment{{Key: EnvMode, Value: mode}})
		},
	}
}

func profileRegionAssignments(profile, region string) []shell.Assignment {
	return []shell.Assignment{
		{Key: "AWS_PROFILE", Value: profile},
		{Key: "AWS_REGION", Value: region},
		{Key: "AWS_DEFAULT_REGION", Value: region},
	}
}

func printAssignments(cmd *cobra.Command, dialect shell.Dialect, updates []shell.Assignment) error {
	lines, err := shell.EmitAssignments(dialect, updates)
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Fprintln(cmd.OutOrStdout(), line)
	}
	return nil
}
