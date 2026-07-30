package shell

import "fmt"

// ErrUnsupportedShell is returned by InitScript for shells without a generator.
type ErrUnsupportedShell struct{ Shell string }

func (e ErrUnsupportedShell) Error() string {
	return fmt.Sprintf("unsupported shell %q: supported shells are zsh, bash, powershell, and cmd", e.Shell)
}

// InitScript returns the one-time shell-integration snippet for a shell.
func InitScript(shell string) (string, error) {
	switch shell {
	case "zsh":
		return zshFunction, nil
	case "bash":
		return bashFunction, nil
	case "powershell", "pwsh":
		return powerShellFunction, nil
	case "cmd", "cmd.exe", "command-prompt":
		return cmdWrapper, nil
	default:
		return "", ErrUnsupportedShell{Shell: shell}
	}
}

// zshFunction wraps opsx so that ONLY switching subcommands
// (use/kube/mode/region)
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
    use|kube|mode|region)
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
      # Defense-in-depth: accept only the exact keys and value charset emitted by
      # opsx. A line that merely starts with "export " may still contain command
      # substitution, so prefix checking is not sufficient before eval.
      while IFS= read -r _opsx_line; do
        case "$_opsx_line" in
          "") ;;
          *)
            if [[ ! "$_opsx_line" =~ ^export\ (AWS_PROFILE|AWS_REGION|AWS_DEFAULT_REGION|KUBECONFIG|OPSX_MODE)=[A-Za-z0-9%._/@:=+~-]+$ ]]; then
              print -r -- "opsx: refusing to eval unexpected shell-switch output: $_opsx_line" >&2
              return 1
            fi
            ;;
        esac
      done <<< "$_opsx_output"
      eval "$_opsx_output"
      ;;
    *) command opsx "$@" ;;
  esac
}`

// bashFunction is the Git Bash / Bash variant of the POSIX wrapper. It keeps the
// same dispatch and export-only eval contract as the zsh function, but uses
// portable Bash built-ins for diagnostics.
const bashFunction = `opsx() {
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
    use|kube|mode|region)
      for _opsx_arg in "$@"; do
        case "$_opsx_arg" in
          -h|--help) command opsx "$@"; return $? ;;
        esac
      done
      _opsx_output="$(command opsx shell-switch "$@")" || return $?
      while IFS= read -r _opsx_line; do
        case "$_opsx_line" in
          "") ;;
          *)
            if [[ ! "$_opsx_line" =~ ^export\ (AWS_PROFILE|AWS_REGION|AWS_DEFAULT_REGION|KUBECONFIG|OPSX_MODE)=[A-Za-z0-9%._/@:=+~-]+$ ]]; then
              printf '%s\n' "opsx: refusing to eval unexpected shell-switch output: $_opsx_line" >&2
              return 1
            fi
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
    { $_ -in @("use", "kube", "mode", "region") } {
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
        if ($opsxLine -eq "" -or $opsxLine -match '^\$env:(AWS_PROFILE|AWS_REGION|AWS_DEFAULT_REGION|KUBECONFIG|OPSX_MODE) = "[A-Za-z0-9%._/@:=+~\\-]+"$') {
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

// cmdWrapper is a batch-file wrapper for Windows Command Prompt. A .exe cannot
// mutate its parent cmd.exe environment, but a .cmd file runs in that command
// interpreter and its SET commands persist after it returns. Install this as an
// opsx.cmd that appears on PATH before opsx.exe; the wrapper delegates
// non-switching commands directly to opsx.exe and applies shell-switch output
// only for use/kube/mode/region.
const cmdWrapper = `@echo off
if not defined OPSX_EXE set "OPSX_EXE=opsx.exe"
set "_opsx_subcmd="
set "_opsx_skip_next="
set "_opsx_args=%*"

:opsx_parse
if "%~1"=="" goto opsx_dispatch
if defined _opsx_skip_next (
  set "_opsx_skip_next="
  shift
  goto opsx_parse
)
set "_opsx_arg=%~1"
if "%_opsx_arg%"=="--mode" (
  set "_opsx_skip_next=1"
  shift
  goto opsx_parse
)
if "%_opsx_arg:~0,7%"=="--mode=" (
  shift
  goto opsx_parse
)
if "%_opsx_arg%"=="--opr" (
  shift
  goto opsx_parse
)
if "%_opsx_arg:~0,2%"=="--" (
  shift
  goto opsx_parse
)
set "_opsx_subcmd=%_opsx_arg%"

:opsx_dispatch
for %%S in (use kube mode region) do if "%_opsx_subcmd%"=="%%S" goto opsx_switch
"%OPSX_EXE%" %_opsx_args%
exit /b %ERRORLEVEL%

:opsx_switch
for %%A in (%_opsx_args%) do (
  if "%%~A"=="-h" (
    "%OPSX_EXE%" %_opsx_args%
    exit /b %ERRORLEVEL%
  )
  if "%%~A"=="--help" (
    "%OPSX_EXE%" %_opsx_args%
    exit /b %ERRORLEVEL%
  )
)
set "_opsx_tmp=%TEMP%\opsx-shell-switch-%RANDOM%-%RANDOM%.tmp"
set "_opsx_invalid=%TEMP%\opsx-shell-switch-%RANDOM%-%RANDOM%.invalid"
"%OPSX_EXE%" shell-switch --shell cmd %_opsx_args% > "%_opsx_tmp%"
set "_opsx_status=%ERRORLEVEL%"
if not "%_opsx_status%"=="0" (
  del "%_opsx_tmp%" >nul 2>nul
  exit /b %_opsx_status%
)
findstr /v /r /x ^
  /c:"set \"AWS_PROFILE=[-A-Za-z0-9%%._/@:=+~\\][-A-Za-z0-9%%._/@:=+~\\]*\"" ^
  /c:"set \"AWS_REGION=[-A-Za-z0-9%%._/@:=+~\\][-A-Za-z0-9%%._/@:=+~\\]*\"" ^
  /c:"set \"AWS_DEFAULT_REGION=[-A-Za-z0-9%%._/@:=+~\\][-A-Za-z0-9%%._/@:=+~\\]*\"" ^
  /c:"set \"KUBECONFIG=[-A-Za-z0-9%%._/@:=+~\\][-A-Za-z0-9%%._/@:=+~\\]*\"" ^
  /c:"set \"OPSX_MODE=[-A-Za-z0-9%%._/@:=+~\\][-A-Za-z0-9%%._/@:=+~\\]*\"" ^
  "%_opsx_tmp%" > "%_opsx_invalid%"
set "_opsx_validation=%ERRORLEVEL%"
if "%_opsx_validation%"=="0" (
  type "%_opsx_invalid%" >&2
  del "%_opsx_invalid%" >nul 2>nul
  del "%_opsx_tmp%" >nul 2>nul
  echo opsx: refusing to execute unexpected shell-switch output >&2
  exit /b 1
)
if not "%_opsx_validation%"=="1" (
  del "%_opsx_invalid%" >nul 2>nul
  del "%_opsx_tmp%" >nul 2>nul
  echo opsx: failed to validate shell-switch output >&2
  exit /b 1
)
del "%_opsx_invalid%" >nul 2>nul
for /f "usebackq delims=" %%L in ("%_opsx_tmp%") do %%L
set "_opsx_status=%ERRORLEVEL%"
del "%_opsx_tmp%" >nul 2>nul
exit /b %_opsx_status%
`
