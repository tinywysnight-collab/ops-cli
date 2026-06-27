package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/creds"
)

// switchAccount assumes the citizen role for alias in the current mode, writes
// the [<alias>.<mode>] profile, and returns the profile name plus the account's
// resolved session region. It performs no MFA — it relies entirely on cached
// master credentials.
func switchAccount(ctx context.Context, alias, mode string) (profile, region string, err error) {
	cfg, err := loadConfig()
	if err != nil {
		return "", "", err
	}
	cs, err := credStore()
	if err != nil {
		return "", "", err
	}
	ss, err := stateStore()
	if err != nil {
		return "", "", err
	}
	svc := &creds.CitizenService{Cfg: cfg, Creds: cs, State: ss, Assume: citizenAssumer}
	profile, err = svc.Use(ctx, alias, mode)
	if err != nil {
		return "", "", err
	}
	region, err = cfg.ResolveCitizenRegion(alias)
	if err != nil {
		return "", "", err
	}
	return profile, region, nil
}

func newUseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "use <account-alias>",
		Short: "Switch the current terminal to a citizen account by alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolveMode(cmd)
			if err != nil {
				return err
			}
			profile, _, err := switchAccount(cmd.Context(), args[0], mode)
			if err != nil {
				return err
			}
			// Human summary to stderr; the parent shell is changed only when run
			// via the opsx shell function or `eval "$(opsx shell-switch use ...)"`.
			fmt.Fprintf(cmd.ErrOrStderr(), "switched account: %s (mode %s) → AWS_PROFILE=%s\n", args[0], mode, profile)
			return nil
		},
	}
}
