package shell_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/shell"
)

func TestExport(t *testing.T) {
	cases := []struct {
		key, value, want string
	}{
		{"AWS_PROFILE", "dev.admin", "export AWS_PROFILE=dev.admin"},
		{"KUBECONFIG", "/home/u/.config/opsx/kube/dev-syd.admin.yaml", "export KUBECONFIG=/home/u/.config/opsx/kube/dev-syd.admin.yaml"},
		{"OPSX_MODE", "opr", "export OPSX_MODE=opr"},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			got, err := shell.Export(tc.key, tc.value)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestExportRejectsUnsafeValues(t *testing.T) {
	unsafe := []string{
		"dev;rm -rf ~",
		"dev$(whoami)",
		"dev`id`",
		"a b",
		"dev|cat",
		"dev&",
		"x\nY=1",
	}
	for _, v := range unsafe {
		t.Run(v, func(t *testing.T) {
			out, err := shell.Export("AWS_PROFILE", v)
			require.Error(t, err)
			require.Empty(t, out, "no export line may be produced for an unsafe value")
		})
	}
}

// TestExportAllowsPercentEncodedKubeconfig pins that the export emitter accepts
// the "%" produced by the kubeconfig-alias encoding (e.g. a dotted cluster alias
// "dev.syd" → "dev%2Esyd.yaml"). The alias encoding and the export charset must
// agree, so `opsx kube` never fails with an "unsafe value" error for a
// config-valid dotted alias.
func TestExportAllowsPercentEncodedKubeconfig(t *testing.T) {
	path := "/home/u/.config/opsx/kube/admin/dev%2Esyd.yaml"
	got, err := shell.Export("KUBECONFIG", path)
	require.NoError(t, err)
	require.Equal(t, "export KUBECONFIG="+path, got)
}

func TestEmitAssignmentsByDialect(t *testing.T) {
	updates := []shell.Assignment{
		{Key: "AWS_PROFILE", Value: "dev.admin"},
		{Key: "KUBECONFIG", Value: "/home/u/.config/opsx/kube/admin/dev%2Esyd.yaml"},
		{Key: "OPSX_MODE", Value: "opr"},
	}
	cases := []struct {
		name    string
		dialect shell.Dialect
		want    []string
	}{
		{
			name:    "posix",
			dialect: shell.DialectPOSIX,
			want: []string{
				"export AWS_PROFILE=dev.admin",
				"export KUBECONFIG=/home/u/.config/opsx/kube/admin/dev%2Esyd.yaml",
				"export OPSX_MODE=opr",
			},
		},
		{
			name:    "powershell",
			dialect: shell.DialectPowerShell,
			want: []string{
				`$env:AWS_PROFILE = "dev.admin"`,
				`$env:KUBECONFIG = "/home/u/.config/opsx/kube/admin/dev%2Esyd.yaml"`,
				`$env:OPSX_MODE = "opr"`,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := shell.EmitAssignments(tc.dialect, updates)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestPowerShellAssignmentRejectsUnsafeValues(t *testing.T) {
	updates := []shell.Assignment{{Key: "AWS_PROFILE", Value: `dev"; Remove-Item -Recurse ~`}}
	got, err := shell.EmitAssignments(shell.DialectPowerShell, updates)
	require.Error(t, err)
	require.Empty(t, got, "unsafe PowerShell assignments must never be emitted")
}

func TestInitScriptZshOnlyWrapsSwitchers(t *testing.T) {
	got, err := shell.InitScript("zsh")
	require.NoError(t, err)
	// Switching subcommands go through shell-switch...
	require.Contains(t, got, "use|kube|mode")
	require.Contains(t, got, `command opsx shell-switch "$@"`)
	require.Contains(t, got, `return $?`)
	// ...everything else runs the bare binary directly.
	require.Contains(t, got, `command opsx "$@"`)
	require.True(t, strings.HasPrefix(got, "opsx() {"))
}

func TestInitScriptZshRoutesLeadingGlobalFlagsAndPreservesFailure(t *testing.T) {
	dir := t.TempDir()
	fakeOpsx := filepath.Join(dir, "opsx")
	logPath := filepath.Join(dir, "opsx.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "` + logPath + `"
if [ "$1" = "shell-switch" ]; then
  shift
  case "$*" in
    *missing*) exit 23 ;;
    *) printf '%s\n' 'export OPSX_MODE=opr' ;;
  esac
fi
`
	require.NoError(t, os.WriteFile(fakeOpsx, []byte(script), 0o700))

	initScript, err := shell.InitScript("zsh")
	require.NoError(t, err)

	cmd := exec.Command("/bin/zsh", "-c", initScript+`
PATH="`+dir+`:$PATH"
opsx --mode opr use dev
printf 'mode=%s\n' "$OPSX_MODE"
opsx use missing
printf 'status=%d\n' $?
`)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	require.Contains(t, string(out), "mode=opr")
	require.Contains(t, string(out), "status=23")

	logData, err := os.ReadFile(logPath)
	require.NoError(t, err)
	log := string(logData)
	require.Contains(t, log, "shell-switch --mode opr use dev\n")
	require.Contains(t, log, "shell-switch use missing\n")
}

// TestInitScriptZshDoesNotEvalHelpOutput pins that a help flag on a switching
// subcommand (`opsx use --help`) is shown as help and NEVER eval'd. cobra prints
// `--help` text to stdout with exit 0; the prior wrapper captured that exit-0
// stdout and eval'd it, spewing "command not found: Usage:" into the live shell.
func TestInitScriptZshDoesNotEvalHelpOutput(t *testing.T) {
	dir := t.TempDir()
	fakeOpsx := filepath.Join(dir, "opsx")
	// Faithful cobra stand-in: ANY invocation carrying -h/--help prints help to
	// stdout and exits 0 (this is exactly what `opsx shell-switch use --help`
	// does); otherwise `shell-switch …` emits an export line.
	script := `#!/bin/sh
for a in "$@"; do
  case "$a" in
    -h|--help)
      printf '%s\n' 'Usage:'
      printf '%s\n' '  opsx use <account-alias> [flags]'
      exit 0 ;;
  esac
done
if [ "$1" = "shell-switch" ]; then
  printf '%s\n' 'export AWS_PROFILE=dev.admin'
fi
`
	require.NoError(t, os.WriteFile(fakeOpsx, []byte(script), 0o700))

	initScript, err := shell.InitScript("zsh")
	require.NoError(t, err)

	cmd := exec.Command("/bin/zsh", "-c", initScript+`
PATH="`+dir+`:$PATH"
opsx use --help
`)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "Usage:", "help text must reach the user")
	require.NotContains(t, s, "command not found", "help text must never be eval'd as commands")
	require.NotContains(t, s, "export AWS_PROFILE", "help must not go through shell-switch")
}

// TestInitScriptZshRefusesNonExportOutput pins the defense-in-depth contract: the
// wrapper eval's ONLY `export` lines, so a future exit-0 stdout leak from a
// switching subcommand can never run as an arbitrary shell command.
func TestInitScriptZshRefusesNonExportOutput(t *testing.T) {
	dir := t.TempDir()
	fakeOpsx := filepath.Join(dir, "opsx")
	marker := filepath.Join(dir, "pwned")
	// The bogus stdout is a real command with a filesystem side-effect, so the
	// test distinguishes "eval'd and executed" (marker created) from merely
	// echoed in a diagnostic.
	script := `#!/bin/sh
if [ "$1" = "shell-switch" ]; then
  printf '%s\n' 'touch ` + marker + `'
fi
`
	require.NoError(t, os.WriteFile(fakeOpsx, []byte(script), 0o700))

	initScript, err := shell.InitScript("zsh")
	require.NoError(t, err)

	cmd := exec.Command("/bin/zsh", "-c", initScript+`
PATH="`+dir+`:$PATH"
opsx use dev
printf 'status=%d\n' $?
`)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	require.NoFileExists(t, marker, "non-export stdout must never be eval'd/executed")
	require.Contains(t, string(out), "status=1", "wrapper must report failure when output is not export-only")
}

func TestInitScriptPowerShellEmitsWrapper(t *testing.T) {
	got, err := shell.InitScript("powershell")
	require.NoError(t, err)
	require.Contains(t, got, "function opsx")
	require.Contains(t, got, "use", "PowerShell wrapper must recognize switching subcommands")
	require.Contains(t, got, "kube", "PowerShell wrapper must recognize switching subcommands")
	require.Contains(t, got, "mode", "PowerShell wrapper must recognize switching subcommands")
	require.Contains(t, got, "shell-switch")
	require.Contains(t, got, "--shell")
	require.Contains(t, got, "powershell")
	require.Contains(t, got, "Invoke-Expression")
	require.Contains(t, got, "$env:")
}

func TestInitScriptPowerShellRoutesLeadingGlobalFlagsAndPreservesFailure(t *testing.T) {
	pwsh := requirePwsh(t)
	dir := t.TempDir()
	fakeOpsx := filepath.Join(dir, "opsx")
	logPath := filepath.Join(dir, "opsx.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "` + logPath + `"
if [ "$1" = "shell-switch" ]; then
  shift
  case "$*" in
    *missing*) exit 23 ;;
    *) printf '%s\n' '$env:OPSX_MODE = "opr"' ;;
  esac
fi
`
	require.NoError(t, os.WriteFile(fakeOpsx, []byte(script), 0o700))

	initScript, err := shell.InitScript("powershell")
	require.NoError(t, err)

	cmd := exec.Command(pwsh, "-NoProfile", "-Command", initScript+`
$env:PATH = "`+dir+`" + [IO.Path]::PathSeparator + $env:PATH
opsx --mode opr use dev
Write-Output "mode=$env:OPSX_MODE"
opsx use missing
Write-Output "status=$LASTEXITCODE"
`)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	require.Contains(t, string(out), "mode=opr")
	require.Contains(t, string(out), "status=23")

	logData, err := os.ReadFile(logPath)
	require.NoError(t, err)
	log := string(logData)
	require.Contains(t, log, "shell-switch --shell powershell --mode opr use dev\n")
	require.Contains(t, log, "shell-switch --shell powershell use missing\n")
}

func TestInitScriptPowerShellDoesNotEvaluateHelpOutput(t *testing.T) {
	pwsh := requirePwsh(t)
	dir := t.TempDir()
	fakeOpsx := filepath.Join(dir, "opsx")
	script := `#!/bin/sh
for a in "$@"; do
  case "$a" in
    -h|--help)
      printf '%s\n' 'Usage:'
      printf '%s\n' '  opsx use <account-alias> [flags]'
      exit 0 ;;
  esac
done
if [ "$1" = "shell-switch" ]; then
  printf '%s\n' '$env:AWS_PROFILE = "dev.admin"'
fi
`
	require.NoError(t, os.WriteFile(fakeOpsx, []byte(script), 0o700))

	initScript, err := shell.InitScript("powershell")
	require.NoError(t, err)

	cmd := exec.Command(pwsh, "-NoProfile", "-Command", initScript+`
$env:PATH = "`+dir+`" + [IO.Path]::PathSeparator + $env:PATH
opsx use --help
`)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "Usage:", "help text must reach the user")
	require.NotContains(t, s, "$env:AWS_PROFILE", "help must not go through shell-switch")
}

func TestInitScriptPowerShellRefusesNonAssignmentOutput(t *testing.T) {
	pwsh := requirePwsh(t)
	dir := t.TempDir()
	fakeOpsx := filepath.Join(dir, "opsx")
	marker := filepath.Join(dir, "pwned")
	script := `#!/bin/sh
if [ "$1" = "shell-switch" ]; then
  printf '%s\n' 'Set-Content -Path "` + marker + `" -Value pwned'
fi
`
	require.NoError(t, os.WriteFile(fakeOpsx, []byte(script), 0o700))

	initScript, err := shell.InitScript("powershell")
	require.NoError(t, err)

	cmd := exec.Command(pwsh, "-NoProfile", "-Command", initScript+`
$env:PATH = "`+dir+`" + [IO.Path]::PathSeparator + $env:PATH
opsx use dev
Write-Output "status=$LASTEXITCODE"
`)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	require.NoFileExists(t, marker, "non-assignment stdout must never be evaluated/executed")
	require.Contains(t, string(out), "status=1", "wrapper must report failure when output is not assignment-only")
}

func TestInitScriptUnsupported(t *testing.T) {
	_, err := shell.InitScript("fish")
	require.Error(t, err)
	require.Contains(t, err.Error(), "fish")
}

func requirePwsh(t *testing.T) string {
	t.Helper()
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not installed; skipping live PowerShell wrapper test")
	}
	return pwsh
}
