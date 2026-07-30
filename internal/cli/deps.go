package cli

import (
	"github.com/tinywysnight-collab/ops-cli/internal/config"
	"github.com/tinywysnight-collab/ops-cli/internal/creds"
	"github.com/tinywysnight-collab/ops-cli/internal/paths"
	"github.com/tinywysnight-collab/ops-cli/internal/state"
)

// loadConfig loads and validates the config from its default path.
func loadConfig() (*config.Config, error) {
	p, err := paths.ConfigFile()
	if err != nil {
		return nil, err
	}
	c, err := config.Load(p)
	if err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func configStore() (*config.Store, error) {
	cp, err := paths.ConfigFile()
	if err != nil {
		return nil, err
	}
	lp, err := paths.LockFile()
	if err != nil {
		return nil, err
	}
	return config.NewStore(cp, lp), nil
}

// credStore returns the credentials store wired to the default paths.
func credStore() (*creds.Store, error) {
	cp, err := paths.CredentialsFile()
	if err != nil {
		return nil, err
	}
	lp, err := paths.LockFile()
	if err != nil {
		return nil, err
	}
	return creds.NewStore(cp, lp), nil
}

// stateStore returns the state store wired to the default paths.
func stateStore() (*state.Store, error) {
	sp, err := paths.StateFile()
	if err != nil {
		return nil, err
	}
	lp, err := paths.LockFile()
	if err != nil {
		return nil, err
	}
	return state.NewStore(sp, lp), nil
}
