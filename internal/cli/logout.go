package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/creds"
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
			profiles, err := runLogout(cmd.Context(), mode, all)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "logged out: removed %d opsx-managed profile(s)\n", len(profiles))
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "purge all opsx-managed profiles for every mode")
	return cmd
}

func runLogout(ctx context.Context, mode string, all bool) ([]string, error) {
	cs, err := credStore()
	if err != nil {
		return nil, err
	}
	ss, err := stateStore()
	if err != nil {
		return nil, err
	}
	entries, err := ss.Load()
	if err != nil {
		return nil, err
	}
	profiles, err := logoutProfiles(mode, all, entries)
	if err != nil {
		return nil, err
	}
	if err := cs.Delete(ctx, profiles); err != nil {
		return nil, err
	}
	if err := ss.Delete(ctx, profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

func logoutProfiles(mode string, all bool, entries map[string]state.Entry) ([]string, error) {
	mode, err := config.NormalizeMode(mode)
	if err != nil {
		return nil, err
	}
	targets := map[string]struct{}{}
	// `opsx use` overwrites the shared [default] profile, so logout clears it too.
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

// isOpsxManagedEntry reports whether a state entry is opsx-managed. state.json
// holds only opsx's own entries and opsx always records a Mode, so any entry
// with a non-empty Mode is ours (covers admin, opr, and any configured mode).
func isOpsxManagedEntry(entry state.Entry) bool {
	return entry.Mode != ""
}
