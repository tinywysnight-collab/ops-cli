package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/shell"
)

// EnvMode is the per-terminal mode environment variable.
const EnvMode = "OPSX_MODE"

// resolveMode returns the effective mode for a command. Precedence is explicit
// and symmetric: explicit flags > OPSX_MODE env > default admin. --opr is a
// shorthand for --mode opr; disagreeing explicit selectors are rejected.
// Mode is purely runtime state and is never persisted to disk.
func resolveMode(cmd *cobra.Command) (string, error) {
	modeSet := flagChanged(cmd, "mode")
	oprSet := flagChanged(cmd, "opr")
	return resolveModeSelection(persistentMode, persistentOpr, modeSet, oprSet, os.Getenv(EnvMode))
}

func resolveModeSelection(modeFlag string, oprFlag, modeSet, oprSet bool, envMode string) (string, error) {
	// --opr selects opr only when explicitly set to true. An explicitly-negated
	// --opr=false (oprSet && !oprFlag) means "opr not selected": it neither
	// forces opr nor conflicts with --mode.
	oprSelected := oprSet && oprFlag
	if modeSet {
		mode, err := config.NormalizeMode(modeFlag)
		if err != nil {
			return "", err
		}
		if oprSelected && mode != config.ModeOpr {
			return "", fmt.Errorf("conflicting mode selectors: --opr implies %q but --mode is %q", config.ModeOpr, mode)
		}
		return mode, nil
	}
	if oprSelected {
		return config.ModeOpr, nil
	}
	if envMode != "" {
		return config.NormalizeMode(envMode)
	}
	return config.ModeAdmin, nil
}

func flagChanged(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	if flag := cmd.Flags().Lookup(name); flag != nil {
		return flag.Changed
	}
	if root := cmd.Root(); root != nil {
		if flag := root.PersistentFlags().Lookup(name); flag != nil {
			return flag.Changed
		}
	}
	return false
}

// emitMode validates the requested mode and writes its export line to out.
func emitMode(out io.Writer, arg string) error {
	mode, err := normalizeModeArg(arg)
	if err != nil {
		return err
	}
	line, err := shell.Export(EnvMode, mode)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, line)
	return nil
}

func normalizeModeArg(arg string) (string, error) {
	return config.NormalizeMode(arg)
}

func newModeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "mode <admin|opr>",
		Short: "Set this terminal's default mode (emits an export; never persisted)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return emitMode(cmd.OutOrStdout(), args[0])
		},
	}
}
