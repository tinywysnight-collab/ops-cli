package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/creds"
)

func setDefaultProfile(ctx context.Context, alias, mode string) (string, error) {
	profile, _, err := switchAccount(ctx, alias, mode, "")
	if err != nil {
		return "", err
	}
	cs, err := credStore()
	if err != nil {
		return "", err
	}
	c, ok, err := cs.Read(profile)
	if err != nil {
		return "", err
	}
	if !ok || !c.HasSessionToken() {
		return "", fmt.Errorf("cannot set default profile from incomplete profile %q", profile)
	}
	if err := cs.Write(ctx, creds.DefaultProfile, c); err != nil {
		return "", err
	}
	return profile, nil
}

func newDefaultCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "default <account-alias>",
		Short: "Set AWS [default] credentials explicitly from an account alias",
		Long:  "Set the shared AWS [default] profile explicitly from an opsx account alias. This is an opt-in latest-wins action for shells or tools that do not use AWS_PROFILE.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolveMode(cmd)
			if err != nil {
				return err
			}
			profile, err := setDefaultProfile(cmd.Context(), args[0], mode)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "set default profile: [default] copied from [%s]\n", profile)
			return nil
		},
	}
}
