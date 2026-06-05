package shell

import "fmt"

// ErrUnsupportedShell is returned by InitScript for shells without a generator.
type ErrUnsupportedShell struct{ Shell string }

func (e ErrUnsupportedShell) Error() string {
	return fmt.Sprintf("unsupported shell %q: supported shells are zsh and powershell", e.Shell)
}

// InitScript returns the one-time shell-integration snippet for a shell.
func InitScript(shell string) (string, error) {
	switch shell {
	case "zsh":
		return zshFunction, nil
	case "powershell", "pwsh":
		return powerShellFunction, nil
	default:
		return "", ErrUnsupportedShell{Shell: shell}
	}
}

// zshFunction wraps opsx so that ONLY the switching subcommands (use/kube/mode)
// have their export output applied to the current shell via eval; every other
// subcommand (login, status, ls, init, --help, --version, …) runs directly and
// behaves exactly as the bare binary. Installed once by appending to the rc file.
const zshFunction = `opsx() {
  local _opsx_arg
  local _opsx_output
  local _opsx_line
  local _opsx_skip_next=0
  local _opsx_subcmd=""

  for _opsx_arg in "$@"; do
    if (( _opsx_skip_next )); then
      _opsx_skip_next=0
      continue
    fi
    case "$_opsx_arg" in
      --mode) _opsx_skip_next=1 ;;
      --mode=*|--opr) ;;
      --*) ;;
      *) _opsx_subcmd="$_opsx_arg"; break ;;
    esac
  done

  case "$_opsx_subcmd" in
    use|kube|mode)
      # A help flag makes the switching subcommand print help text to stdout with
      # exit 0. That text is NOT an export line and must never be eval'd, so route
      # any -h/--help invocation straight to the bare binary, which also shows the
      # real subcommand help rather than shell-switch's internal help.
      for _opsx_arg in "$@"; do
        case "$_opsx_arg" in
          -h|--help) command opsx "$@"; return $? ;;
        esac
      done
      _opsx_output="$(command opsx shell-switch "$@")" || return $?
      # Defense-in-depth: only ever eval 'export' lines. Any other stdout (a
      # future non-export leak) is refused rather than executed as a shell command.
      while IFS= read -r _opsx_line; do
        case "$_opsx_line" in
          ""|export\ *) ;;
          *)
            print -r -- "opsx: refusing to eval unexpected shell-switch output: $_opsx_line" >&2
            return 1
            ;;
        esac
      done <<< "$_opsx_output"
      eval "$_opsx_output"
      ;;
    *) command opsx "$@" ;;
  esac
}`

// powerShellFunction mirrors the zsh wrapper for PowerShell. It routes only the
// switching subcommands through shell-switch, asks for the PowerShell dialect,
// validates every stdout line before Invoke-Expression, and preserves the
// underlying native command exit code in $LASTEXITCODE.
const powerShellFunction = `function opsx {
  $opsxArgs = @($args)
  $opsxSubcmd = $null
  $opsxSkipNext = $false

  foreach ($opsxArg in $opsxArgs) {
    if ($opsxSkipNext) {
      $opsxSkipNext = $false
      continue
    }
    switch -Wildcard ($opsxArg) {
      "--mode" { $opsxSkipNext = $true; continue }
      "--mode=*" { continue }
      "--opr" { continue }
      "--*" { continue }
      default { $opsxSubcmd = $opsxArg; break }
    }
    if ($opsxSubcmd) { break }
  }

  $opsxExe = (Get-Command opsx -CommandType Application).Source
  switch ($opsxSubcmd) {
    { $_ -in @("use", "kube", "mode") } {
      foreach ($opsxArg in $opsxArgs) {
        if ($opsxArg -eq "-h" -or $opsxArg -eq "--help") {
          & $opsxExe @opsxArgs
          return
        }
      }

      $opsxOutput = & $opsxExe shell-switch --shell powershell @opsxArgs
      $opsxStatus = $LASTEXITCODE
      if ($opsxStatus -ne 0) {
        $global:LASTEXITCODE = $opsxStatus
        return
      }

      foreach ($opsxLine in $opsxOutput) {
        if ($opsxLine -eq "" -or $opsxLine -match '^\$env:[A-Z_][A-Z0-9_]* = "[A-Za-z0-9%._/@:=+~\\-]*"$') {
          continue
        }
        Write-Error "opsx: refusing to evaluate unexpected shell-switch output: $opsxLine"
        $global:LASTEXITCODE = 1
        return
      }

      foreach ($opsxLine in $opsxOutput) {
        if ($opsxLine -ne "") {
          Invoke-Expression $opsxLine
        }
      }
      $global:LASTEXITCODE = 0
      return
    }
    default {
      & $opsxExe @opsxArgs
      return
    }
  }
}`
