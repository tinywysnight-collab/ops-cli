package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegionCommand(t *testing.T) {
	t.Run("selects configured region without profile or persistence", func(t *testing.T) {
		setupFakeEnv(t, integrationConfig)
		t.Setenv("AWS_PROFILE", "")
		t.Setenv("AWS_REGION", "us-east-1")
		before, err := os.ReadFile(configPathForTest(t))
		require.NoError(t, err)

		stdout, stderr, err := runInteractive(t, "3\n", true, "shell-switch", "region")
		require.NoError(t, err)
		require.Equal(t, "us-west-2", exportValue(stdout, "AWS_REGION"))
		require.Equal(t, "us-west-2", exportValue(stdout, "AWS_DEFAULT_REGION"))
		require.Contains(t, stderr, "us-east-1 (active)")
		require.Less(t, indexFrom(stderr, "ap-southeast-2", 0), indexFrom(stderr, "us-east-1", 0))
		after, err := os.ReadFile(configPathForTest(t))
		require.NoError(t, err)
		require.Equal(t, before, after)
	})

	t.Run("non tty emits no assignments", func(t *testing.T) {
		setupFakeEnv(t, integrationConfig)
		stdout, _, err := runInteractive(t, "", false, "shell-switch", "region")
		require.Error(t, err)
		require.Empty(t, stdout)
		require.Contains(t, err.Error(), "interactive terminal")
	})

	t.Run("cancellation emits no assignments", func(t *testing.T) {
		setupFakeEnv(t, integrationConfig)
		stdout, stderr, err := runInteractive(t, "", true, "shell-switch", "region")
		require.NoError(t, err)
		require.Empty(t, stdout)
		require.Contains(t, stderr, "Cancelled.")
	})
}

func TestRegionShellDialects(t *testing.T) {
	tests := []struct {
		name   string
		shell  string
		prefix string
	}{
		{name: "powershell", shell: "powershell", prefix: `$env:AWS_REGION = "`},
		{name: "cmd", shell: "cmd", prefix: `set "AWS_REGION=`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupFakeEnv(t, integrationConfig)
			stdout, stderr, err := runInteractive(t, "1\n", true,
				"shell-switch", "--shell", tt.shell, "region")
			require.NoError(t, err)
			require.Contains(t, stdout, tt.prefix)
			require.NotContains(t, stdout, "Region:")
			require.Contains(t, stderr, "Region:")
		})
	}
}
