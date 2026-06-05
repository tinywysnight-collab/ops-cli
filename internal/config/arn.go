package config

import "fmt"

// MasterRoleARN composes the master role ARN for a mode entirely from config:
//
//	arn:aws:iam::{master_account_id}:role/{master_roles[mode]}
func (c *Config) MasterRoleARN(mode string) (string, error) {
	mode, err := NormalizeMode(mode)
	if err != nil {
		return "", err
	}
	if c.Auth.MasterAccountID == "" {
		return "", fmt.Errorf("auth.master_account_id is not set")
	}
	role, ok := c.Auth.MasterRoles[mode]
	if !ok || role == "" {
		return "", fmt.Errorf("auth.master_roles has no entry for mode %q", mode)
	}
	return fmt.Sprintf("arn:aws:iam::%s:role/%s", c.Auth.MasterAccountID, role), nil
}

// CitizenRoleARN composes the citizen role ARN for an account alias and mode
// entirely from config:
//
//	arn:aws:iam::{accounts[alias].account_id}:role/{citizen_roles[mode]}
func (c *Config) CitizenRoleARN(alias, mode string) (string, error) {
	mode, err := NormalizeMode(mode)
	if err != nil {
		return "", err
	}
	acct, ok := c.Accounts[alias]
	if !ok {
		return "", fmt.Errorf("unknown account alias %q", alias)
	}
	if acct.AccountID == "" {
		return "", fmt.Errorf("account %q has no account_id", alias)
	}
	role, ok := c.Auth.CitizenRoles[mode]
	if !ok || role == "" {
		return "", fmt.Errorf("auth.citizen_roles has no entry for mode %q", mode)
	}
	return fmt.Sprintf("arn:aws:iam::%s:role/%s", acct.AccountID, role), nil
}
