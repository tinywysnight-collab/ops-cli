package auth_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/auth"
)

func TestReadPasswordEnvFallback(t *testing.T) {
	t.Setenv(auth.EnvPassword, "s3cret")
	var prompt bytes.Buffer

	pw, err := auth.ReadPassword(context.Background(), &prompt, "Password: ")
	require.NoError(t, err)
	require.Equal(t, []byte("s3cret"), pw)
	// With the env fallback, nothing is prompted.
	require.Empty(t, prompt.String())
}

func TestReadPasswordCancelledContext(t *testing.T) {
	// SIGINT during the login password prompt cancels the root context; the
	// prompt must fail fast instead of blocking in term.ReadPassword. A
	// cancelled context is detected before any TTY access, so this test is
	// safe to run without a terminal.
	t.Setenv(auth.EnvPassword, "") // empty value must NOT count as set
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var prompt bytes.Buffer

	done := make(chan struct{})
	var pw []byte
	var err error
	go func() {
		pw, err = auth.ReadPassword(ctx, &prompt, "Password: ")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ReadPassword did not return after context cancellation")
	}
	require.Error(t, err)
	require.Nil(t, pw)
}

func TestZeroize(t *testing.T) {
	b := []byte("topsecret")
	auth.Zeroize(b)
	require.Equal(t, make([]byte, len("topsecret")), b)
}
