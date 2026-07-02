package config

import "fmt"

// Profile-name spelling is a single source of truth (see architecture). Master
// caches are master_admin / master_awsopr / master_<mode>; citizen profiles are
// <alias>.<mode>.<role>.

// MasterProfile returns the cached master profile name for a mode. admin/opr
// keep their historical names for backward compatibility; any other configured
// mode uses master_<mode>.
func MasterProfile(mode string) (string, error) {
	if _, err := NormalizeMode(mode); err != nil {
		return "", err
	}
	switch mode {
	case ModeAdmin:
		return "master_admin", nil
	case ModeOpr:
		return "master_awsopr", nil
	default:
		return "master_" + mode, nil
	}
}

// CitizenProfile returns the citizen profile name for an account alias, mode,
// and effective citizen role: <alias>.<mode>.<role>. Encoding the role isolates
// distinct (alias, mode, citizen-role) triples so a --role override never
// overwrites another switch's cached credentials.
func CitizenProfile(alias, mode, role string) string {
	return fmt.Sprintf("%s.%s.%s", alias, mode, role)
}
