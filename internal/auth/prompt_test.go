package auth_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/auth"
)

func TestReadPasswordEnvFallback(t *testing.T) {
	t.Setenv(auth.EnvPassword, "s3cret")
	var prompt bytes.Buffer

	pw, err := auth.ReadPassword(&prompt, "Password: ")
	require.NoError(t, err)
	require.Equal(t, []byte("s3cret"), pw)
	// With the env fallback, nothing is prompted.
	require.Empty(t, prompt.String())
}

func TestZeroize(t *testing.T) {
	b := []byte("topsecret")
	auth.Zeroize(b)
	require.Equal(t, make([]byte, len("topsecret")), b)
}
