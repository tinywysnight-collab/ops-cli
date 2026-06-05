// Package cli holds the Cobra command adapters. Commands are thin: they parse
// flags/args and delegate to the domain packages (config, auth, creds, state,
// kube, shell). Commands return errors via RunE; only cmd/opsx/main.go exits.
package cli

import (
	"github.com/spf13/cobra"
)

// Version is overridden at build time via -ldflags "-X ...Version=...".
var Version = "dev"

// persistentOpr backs the global --opr flag (shorthand for --mode opr).
var persistentOpr bool

// persistentMode backs the global --mode flag (symmetric override: admin|opr).
var persistentMode string

// NewRootCommand builds the opsx root command with all subcommands registered.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "opsx",
		Short:         "Fast AWS account & EKS cluster switcher",
		Long:          "opsx authenticates once per master role per hour, then switches between AWS citizen accounts and EKS clusters by short alias with full multi-terminal isolation.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true, // main.go owns error formatting + exit code
	}

	root.PersistentFlags().BoolVar(&persistentOpr, "opr", false, "shorthand for --mode opr (operator/AWSOpr)")
	root.PersistentFlags().StringVar(&persistentMode, "mode", "", "force mode for this command: admin|opr (overrides OPSX_MODE)")

	root.AddCommand(
		newLoginCommand(),
		newUseCommand(),
		newKubeCommand(),
		newModeCommand(),
		newStatusCommand(),
		newLsCommand(),
		newLogoutCommand(),
		newInitCommand(),
		newShellSwitchCommand(),
	)

	return root
}
