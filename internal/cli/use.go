package cli

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/creds"
)

// regionOverridePattern is the charset allowed for an explicit `--region` value.
// It is a strict subset of the shell-safe export charset, so a validated region
// can never inject into the emitted `export AWS_REGION=<value>` line (which the
// bare `opsx use` path never reaches, so the check cannot live only in the shell
// emitter).
var regionOverridePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// switchAccount assumes the citizen role for alias in the current mode, writes
// the [<alias>.<mode>] profile, and returns the profile name plus the session
// region. When regionOverride is non-empty it is validated and used as the
// session region (the multi-region-per-account case); otherwise the account's
// config-resolved region is used (accounts.<alias>.region → auth.region → env).
// It performs no MFA — it relies entirely on cached master credentials.
func switchAccount(ctx context.Context, alias, mode, regionOverride string) (profile, region string, err error) {
	cfg, err := loadConfig()
	if err != nil {
		return "", "", err
	}
	if r := strings.TrimSpace(regionOverride); r != "" {
		if !regionOverridePattern.MatchString(r) {
			return "", "", fmt.Errorf("invalid --region %q: must contain only letters, digits, '.', '_', '-'", regionOverride)
		}
		if !cfg.IsRegionAllowed(r) {
			return "", "", fmt.Errorf("invalid --region %q: allowed regions are %s", r, strings.Join(cfg.Regions, ", "))
		}
		region = r
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
	if region != "" {
		return profile, region, nil
	}
	region, err = cfg.ResolveCitizenRegion(alias)
	if err != nil {
		return "", "", err
	}
	return profile, region, nil
}

func newUseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use <account-alias>",
		Short: "Switch the current terminal to a citizen account by alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolveMode(cmd)
			if err != nil {
				return err
			}
			regionOverride, _ := cmd.Flags().GetString("region")
			profile, region, err := switchAccount(cmd.Context(), args[0], mode, regionOverride)
			if err != nil {
				return err
			}
			// Human summary to stderr; the parent shell is changed only when run
			// via the opsx shell function or `eval "$(opsx shell-switch use ...)"`.
			fmt.Fprintf(cmd.ErrOrStderr(), "switched account: %s (mode %s) → AWS_PROFILE=%s AWS_REGION=%s\n", args[0], mode, profile, region)
			return nil
		},
	}
	cmd.Flags().String("region", "", "override the session AWS region for this account (default: the account's configured region)")
	return cmd
}
