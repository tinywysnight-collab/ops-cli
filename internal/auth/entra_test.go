package auth_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/auth"
)

func TestEntraProviderReturnsProvidedSAMLAssertionFromEnv(t *testing.T) {
	t.Setenv(auth.EnvSAMLAssertion, "  ASSERTION-FROM-ENV\n")

	got, err := auth.NewEntraSAMLProvider(testConfig().Auth).FetchAssertion(context.Background(), auth.RoleAdmin)

	require.NoError(t, err)
	require.Equal(t, "ASSERTION-FROM-ENV", got)
}

func TestEntraProviderReturnsProvidedSAMLAssertionFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saml.txt")
	require.NoError(t, os.WriteFile(path, []byte("\nASSERTION-FROM-FILE\n"), 0o600))
	t.Setenv(auth.EnvSAMLAssertionFile, path)

	got, err := auth.NewEntraSAMLProvider(testConfig().Auth).FetchAssertion(context.Background(), auth.RoleOpr)

	require.NoError(t, err)
	require.Equal(t, "ASSERTION-FROM-FILE", got)
}

func TestEntraProviderRequiresConfiguredUsernameWithoutProvidedAssertion(t *testing.T) {
	cases := []struct {
		name string
		// prep configures the env: "" sets the var to empty, "unset" removes it.
		setEmpty bool
	}{
		{name: "vars set to empty string", setEmpty: true},
		{name: "vars genuinely unset", setEmpty: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv records the original value for restoration; we then either
			// leave it empty or remove it entirely so a regression that treats
			// "set-but-empty" differently from "unset" is caught.
			t.Setenv(auth.EnvSAMLAssertion, "")
			t.Setenv(auth.EnvSAMLAssertionFile, "")
			if !tc.setEmpty {
				require.NoError(t, os.Unsetenv(auth.EnvSAMLAssertion))
				require.NoError(t, os.Unsetenv(auth.EnvSAMLAssertionFile))
			}

			_, err := auth.NewEntraSAMLProvider(testConfig().Auth).FetchAssertion(context.Background(), auth.RoleAdmin)

			require.Error(t, err)
			require.Contains(t, err.Error(), "username")
		})
	}
}
