package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/auth"
)

func newLoginCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Authenticate via Entra (+MFA) and cache master STS credentials",
		Long:  "Authenticate via Entra SAML (with interactive MFA) and cache master STS credentials for this mode (admin by default, AWSOpr with --opr). Both can be cached simultaneously.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			mode, err := resolveMode(cmd)
			if err != nil {
				return err
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			cs, err := credStore()
			if err != nil {
				return err
			}
			ss, err := stateStore()
			if err != nil {
				return err
			}
			region, err := cfg.ResolveMasterRegion()
			if err != nil {
				return err
			}

			svc := &auth.LoginService{
				Cfg:      cfg,
				Provider: samlProviderFactory(cfg.Auth),
				STSFactory: func(ctx context.Context) (auth.AssumeWithSAMLAPI, error) {
					return masterAssumeFactory(ctx, region)
				},
				Creds: cs,
				State: ss,
			}
			if err := svc.Login(ctx, mode); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "logged in: master (%s) cached\n", mode)
			return nil
		},
	}
}
