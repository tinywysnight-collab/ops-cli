package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/shell"
)

func newRegionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "region",
		Short: "Interactively set this terminal's AWS region",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			region, cancelled, err := selectTerminalRegion(cmd)
			if err != nil || cancelled {
				return err
			}
			return printAssignments(cmd, shell.DialectPOSIX, regionAssignments(region))
		},
	}
}

func selectTerminalRegion(cmd *cobra.Command) (string, bool, error) {
	p := newPromptSession(cmd.InOrStdin(), cmd.ErrOrStderr(), interactiveTerminal(cmd.InOrStdin()))
	if err := p.requireTTY(); err != nil {
		return "", false, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return "", false, err
	}
	region, cancelled, err := p.selectOne("Region", cfg.Regions, os.Getenv("AWS_REGION"))
	if err != nil {
		return "", false, err
	}
	if cancelled {
		fmt.Fprintln(cmd.ErrOrStderr(), "Cancelled.")
		return "", true, nil
	}
	return region, false, nil
}
