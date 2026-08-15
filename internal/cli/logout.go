package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/creds"
	"github.com/tinywysnight-collab/ops-cli/internal/lock"
	"github.com/tinywysnight-collab/ops-cli/internal/paths"
	"github.com/tinywysnight-collab/ops-cli/internal/state"
)

func newLogoutCommand() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Purge opsx-managed cached credentials and state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolveMode(cmd)
			if err != nil {
				return err
			}
			removed, preserved, err := runLogout(cmd.Context(), mode, all)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "logged out: removed %d opsx-managed profile(s)\n", len(removed))
			if len(preserved) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "preserved %d user-maintained profile(s) not written by opsx: %v\n", len(preserved), preserved)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "purge all opsx-managed profiles for every mode")
	return cmd
}

// runLogout plans and executes the purge as ONE transaction under the shared
// advisory lock: reload state, plan targets, verify each existing target holds
// a complete opsx STS session, then delete credentials and state in the same
// window. A target present in the credentials file without a session token is
// user-maintained (for example long-term keys in [default]) and is preserved
// and reported instead of deleted.
func runLogout(ctx context.Context, mode string, all bool) (removed, preserved []string, err error) {
	cs, err := credStore()
	if err != nil {
		return nil, nil, err
	}
	ss, err := stateStore()
	if err != nil {
		return nil, nil, err
	}
	lp, err := paths.LockFile()
	if err != nil {
		return nil, nil, err
	}
	err = lock.With(ctx, lp, func() error {
		entries, err := ss.Load()
		if err != nil {
			return err
		}
		targets, err := logoutProfiles(mode, all, entries)
		if err != nil {
			return err
		}
		removed, preserved = []string{}, []string{}
		for _, profile := range targets {
			c, ok, err := cs.Read(profile)
			if err != nil {
				return err
			}
			if ok && !c.HasSessionToken() {
				preserved = append(preserved, profile)
				continue
			}
			removed = append(removed, profile)
		}
		if err := cs.DeleteSharedLockHeld(removed); err != nil {
			return err
		}
		return ss.DeleteSharedLockHeld(removed)
	})
	if err != nil {
		return nil, nil, err
	}
	return removed, preserved, nil
}

func logoutProfiles(mode string, all bool, entries map[string]state.Entry) ([]string, error) {
	mode, err := config.NormalizeMode(mode)
	if err != nil {
		return nil, err
	}
	targets := map[string]struct{}{}
	// Older opsx versions wrote the shared [default] profile; keep clearing it
	// as compatibility cleanup even though switches no longer write it.
	targets[creds.DefaultProfile] = struct{}{}
	addMaster := func(m string) error {
		p, err := config.MasterProfile(m)
		if err != nil {
			return err
		}
		targets[p] = struct{}{}
		return nil
	}
	if all {
		if err := addMaster(config.ModeAdmin); err != nil {
			return nil, err
		}
		if err := addMaster(config.ModeOpr); err != nil {
			return nil, err
		}
		for profile, entry := range entries {
			if isOpsxManagedEntry(entry) {
				targets[profile] = struct{}{}
			}
		}
	} else {
		if err := addMaster(mode); err != nil {
			return nil, err
		}
		for profile, entry := range entries {
			if entry.Mode == mode && isOpsxManagedEntry(entry) {
				targets[profile] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(targets))
	for profile := range targets {
		out = append(out, profile)
	}
	return out, nil
}

func isOpsxManagedEntry(entry state.Entry) bool {
	return entry.Mode == config.ModeAdmin || entry.Mode == config.ModeOpr
}
