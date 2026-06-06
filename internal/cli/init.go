package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/shell"
)

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init <shell>",
		Short: "Print one-time shell integration (zsh, bash, powershell, or cmd)",
		Long:  "Print the one-time shell function or wrapper to add to your shell profile/path (e.g. `opsx init zsh >> ~/.zshrc`, `opsx init bash >> ~/.bashrc`, `opsx init powershell >> $PROFILE`, or `opsx init cmd > opsx.cmd`). After that, `opsx use`/`opsx kube`/`opsx mode` transparently update the current terminal.",
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
