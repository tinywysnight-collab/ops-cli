package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/state"
)

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	require.NoError(t, os.WriteFile(statePath, []byte("{not json"), 0o600))

	s := state.NewStore(statePath, filepath.Join(dir, ".lock"))
	_, err := s.Load()
	require.ErrorContains(t, err, "parse state")
}

func TestLoadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	require.NoError(t, os.WriteFile(statePath, []byte(""), 0o600))

	s := state.NewStore(statePath, filepath.Join(dir, ".lock"))
	m, err := s.Load()
	require.NoError(t, err)
	require.Empty(t, m)
}
