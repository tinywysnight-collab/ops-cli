package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectiveCitizenRole(t *testing.T) {
	c := fullValid() // has citizen_roles admin:Admin, opr:AWSOpr
	got, err := c.EffectiveCitizenRole("admin", "")
	require.NoError(t, err)
	require.Equal(t, "Admin", got)

	got, err = c.EffectiveCitizenRole("admin", "BAU")
	require.NoError(t, err)
	require.Equal(t, "BAU", got)
}

func TestCitizenRoleARNWithOverride(t *testing.T) {
	c := fullValid()
	arn, err := c.CitizenRoleARN("dev", "admin", "BAU")
	require.NoError(t, err)
	require.Equal(t, "arn:aws:iam::111111111111:role/BAU", arn)

	arn, err = c.CitizenRoleARN("dev", "admin", "")
	require.NoError(t, err)
	require.Equal(t, "arn:aws:iam::111111111111:role/Admin", arn)
}
