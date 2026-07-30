package shell

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitScriptsRouteRegion(t *testing.T) {
	for _, dialect := range []string{"zsh", "bash", "powershell", "cmd"} {
		t.Run(dialect, func(t *testing.T) {
			script, err := InitScript(dialect)
			require.NoError(t, err)
			require.True(t,
				strings.Contains(script, "use|kube|mode|region") ||
					strings.Contains(script, "'region'") ||
					strings.Contains(script, `"region"`) ||
					strings.Contains(script, "use kube mode region"),
				"wrapper must route region through shell-switch:\n%s", script)
			require.Contains(t, script, "shell-switch")
		})
	}
}
