package config

import "fmt"

// Profile-name spelling is a single source of truth (see architecture). Master
// caches are master_admin / master_awsopr; citizen profiles are <alias>.<mode>.

// MasterProfile returns the cached master profile name for a mode.
func MasterProfile(mode string) (string, error) {
	switch mode {
	case ModeAdmin:
		return "master_admin", nil
	case ModeOpr:
		return "master_awsopr", nil
	default:
		return "", fmt.Errorf("invalid mode %q", mode)
	}
}

// CitizenProfile returns the citizen profile name for an account alias and mode.
func CitizenProfile(alias, mode string) string {
	return fmt.Sprintf("%s.%s", alias, mode)
}
