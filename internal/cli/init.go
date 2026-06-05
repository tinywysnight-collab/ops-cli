package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/shell"
)

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init <shell>",
		Short: "Print one-time shell integration (zsh or powershell)",
		Long:  "Print the one-time shell function to add to your shell profile (e.g. `opsx init zsh >> ~/.zshrc` or `opsx init powershell >> $PROFILE`). After that, `opsx use`/`opsx kube`/`opsx mode` transparently update the current terminal.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			script, err := shell.InitScript(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), script)
			return nil
		},
	}
}
