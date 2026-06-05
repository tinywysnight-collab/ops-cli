package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

func TestResolveModeSelectionRejectsConflictingFlags(t *testing.T) {
	_, err := resolveModeSelection(config.ModeAdmin, true, true, true, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "--opr")
	require.Contains(t, err.Error(), "--mode")
}

func TestResolveModeSelectionAcceptsRedundantAgreeingFlags(t *testing.T) {
	mode, err := resolveModeSelection(config.ModeOpr, true, true, true, config.ModeAdmin)
	require.NoError(t, err)
	require.Equal(t, config.ModeOpr, mode)
}

func TestResolveModeSelectionPrecedence(t *testing.T) {
	cases := []struct {
		name     string
		modeFlag string
		oprFlag  bool
		modeSet  bool
		oprSet   bool
		envMode  string
		want     string
	}{
		{name: "mode flag forces admin over env", modeFlag: config.ModeAdmin, modeSet: true, envMode: config.ModeOpr, want: config.ModeAdmin},
		{name: "opr shorthand forces opr over env", oprFlag: true, oprSet: true, envMode: config.ModeAdmin, want: config.ModeOpr},
		{name: "env wins over default", envMode: config.ModeOpr, want: config.ModeOpr},
		{name: "default admin", want: config.ModeAdmin},
		// An explicitly-negated --opr=false means "opr not selected": it neither
		// forces opr nor conflicts with anything.
		{name: "opr=false alone falls through to default admin", oprFlag: false, oprSet: true, want: config.ModeAdmin},
		{name: "opr=false lets env opr win", oprFlag: false, oprSet: true, envMode: config.ModeOpr, want: config.ModeOpr},
		{name: "mode admin with opr=false is admin, no conflict", modeFlag: config.ModeAdmin, modeSet: true, oprFlag: false, oprSet: true, want: config.ModeAdmin},
		{name: "mode opr with opr=false is opr", modeFlag: config.ModeOpr, modeSet: true, oprFlag: false, oprSet: true, want: config.ModeOpr},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mode, err := resolveModeSelection(tc.modeFlag, tc.oprFlag, tc.modeSet, tc.oprSet, tc.envMode)
			require.NoError(t, err)
			require.Equal(t, tc.want, mode)
		})
	}
}
