# shell-integration Specification

## Purpose

Let `opsx` mutate the parent shell's environment across dialects (POSIX/zsh/bash, PowerShell, Command Prompt): `shell-switch` emits only native assignment lines to stdout, one-time `init` generators emit wrapper functions that route only switching subcommands through `eval`, all emitted values are shell-safe by validation, and a manual `eval` fallback always works.
## Requirements
### Requirement: shell-switch emits only assignment lines to stdout
The system SHALL ensure `opsx shell-switch …` prints ONLY shell-specific environment assignment lines to stdout, routing all prompts, menus, confirmations, logs, and errors to stderr. The default/POSIX dialect SHALL emit plain `export KEY=value` lines safe to consume via `eval`; other dialects SHALL emit only their native assignment syntax.

#### Scenario: Export-only stdout for account switch
- **WHEN** `opsx shell-switch use dev` runs
- **THEN** stdout contains only assignment lines for `AWS_PROFILE`, `AWS_REGION`, and `AWS_DEFAULT_REGION`
- **AND** all prompts and logs go to stderr

#### Scenario: Export-only stdout for cluster switch
- **WHEN** `opsx shell-switch kube dev-syd` runs
- **THEN** stdout contains only assignment lines for `AWS_PROFILE`, `AWS_REGION`, `AWS_DEFAULT_REGION`, and `KUBECONFIG`
- **AND** no other output appears on stdout

#### Scenario: Export-only stdout for mode
- **WHEN** `opsx shell-switch mode opr` runs
- **THEN** stdout contains only an assignment for `OPSX_MODE`

#### Scenario: Export-only stdout for interactive region switch
- **WHEN** `opsx shell-switch region` runs and the operator selects an allowed region
- **THEN** stdout contains only assignments for `AWS_REGION` and `AWS_DEFAULT_REGION`
- **AND** the numbered menu and confirmation appear only on stderr

### Requirement: One-time zsh function generator
The system SHALL emit, via `opsx init zsh`, a one-time shell function suitable for appending to the rc file that wraps `opsx` to apply `shell-switch` output through `eval`.

#### Scenario: init zsh emits the wrapper function
- **WHEN** `opsx init zsh` runs
- **THEN** it emits a wrapper function suitable for appending to the rc file

### Requirement: One-time Bash/Git Bash function generator
The system SHALL emit, via `opsx init bash`, a one-time Bash function suitable for appending to `.bashrc` that wraps `opsx` to apply POSIX `shell-switch` output through guarded `eval`. This includes Git Bash on Windows, where the shell is Bash and a bare `opsx use` child process cannot mutate the parent shell's `AWS_PROFILE`.

#### Scenario: init bash emits the wrapper function
- **WHEN** `opsx init bash` runs
- **THEN** it emits a Bash-compatible wrapper function suitable for appending to `.bashrc`
- **AND** `opsx use dev` through that function exports `AWS_PROFILE` into the current Bash/Git Bash session

### Requirement: Command Prompt shell integration
The system SHALL treat Windows Command Prompt as a distinct shell dialect. The CLI SHALL provide `opsx init cmd`, emitting a batch wrapper suitable for installation as `opsx.cmd`, and `opsx shell-switch --shell cmd` SHALL emit only `cmd.exe` native assignment lines such as `set "AWS_PROFILE=dev.admin"`.

Because `cmd.exe` normally resolves `.exe` before `.cmd` within the same directory, the Command Prompt wrapper SHALL be documented as a PATH-priority wrapper: `opsx.cmd` must be placed in a directory earlier on `PATH` than `opsx.exe`.

#### Scenario: init cmd emits a batch wrapper
- **WHEN** `opsx init cmd` runs
- **THEN** it emits a `.cmd` batch wrapper that routes only `use`, `kube`, `mode`, and `region` through `shell-switch --shell cmd`
- **AND** all other subcommands run `opsx.exe` directly

#### Scenario: cmd account switch emits only cmd assignments
- **WHEN** `opsx shell-switch --shell cmd use dev` runs
- **THEN** stdout contains only cmd `set "KEY=value"` assignments for `AWS_PROFILE`, `AWS_REGION`, and `AWS_DEFAULT_REGION`
- **AND** stdout does not contain POSIX `export` or PowerShell `$env:` syntax

#### Scenario: cmd cluster switch emits account and kubeconfig assignments
- **WHEN** `opsx shell-switch --shell cmd kube dev-syd` runs
- **THEN** stdout contains only cmd `set "KEY=value"` assignments for `AWS_PROFILE`, `AWS_REGION`, `AWS_DEFAULT_REGION`, and `KUBECONFIG`
- **AND** all prompts, confirmations, logs, and errors go to stderr

#### Scenario: cmd region switch emits only region assignments
- **WHEN** `opsx shell-switch --shell cmd region` runs and the operator selects a region
- **THEN** stdout contains only cmd assignments for `AWS_REGION` and `AWS_DEFAULT_REGION`
- **AND** interactive output goes to stderr

### Requirement: Wrapper only intercepts switching subcommands
The installed shell function SHALL route ONLY the switching subcommands (`use`, `kube`, `mode`, `region`) through `opsx shell-switch … | eval`, and SHALL execute every other subcommand (`account`, `cluster`, `login`, `status`, `ls`, `init`, `default`, `--help`, `--version`, …) directly so they behave identically to invoking `opsx` without the wrapper.

The wrapper MUST handle global flags that appear before the subcommand, such as `opsx --mode opr use dev` and `opsx --opr kube dev-syd`. Those invocations MUST still route switching subcommands through `shell-switch`; running the bare binary is insufficient because a child process cannot update the parent shell environment.

The wrapper MUST preserve failure semantics. If `command opsx shell-switch ...` fails, the shell function MUST return a non-zero status and MUST NOT evaluate empty or partial output as a successful switch.

The wrapper MUST NOT evaluate unrecognized output. A switching subcommand can emit exit-0 stdout that is not an assignment line, most notably Cobra help. The wrapper SHALL route any `-h`/`--help` invocation of a switching subcommand directly to the bare binary and, as defense in depth, evaluate only blank lines or recognized environment assignments for its dialect, refusing every other stdout line with a non-zero status.

#### Scenario: Help flag on a switching subcommand is shown, never evaluated
- **WHEN** `opsx use --help`, `opsx kube -h`, or `opsx region --help` runs with the installed wrapper
- **THEN** the real subcommand help text is printed directly
- **AND** the help text is not evaluated by the shell

#### Scenario: Non-assignment stdout is refused, not executed
- **WHEN** a switching subcommand exits successfully but emits an unrecognized stdout line
- **THEN** the wrapper does not evaluate that line
- **AND** it returns non-zero instead of executing arbitrary text

#### Scenario: Switching subcommand is applied through the wrapper
- **WHEN** `opsx use dev`, `opsx kube dev-syd`, `opsx mode opr`, or `opsx region` runs with the installed wrapper
- **THEN** the command is dispatched through the matching `opsx shell-switch` command
- **AND** its validated assignment lines are applied to the current shell

#### Scenario: Switching subcommand with leading global flags is applied
- **WHEN** `opsx --mode opr use dev` or `opsx --opr kube dev-syd` runs with the installed wrapper
- **THEN** the command is dispatched through `opsx shell-switch` with the same arguments
- **AND** its assignment lines are applied to the current shell

#### Scenario: Failed shell-switch fails the wrapper
- **WHEN** a switching command fails validation, prompting, authentication, or lookup
- **THEN** the wrapper returns non-zero
- **AND** it does not evaluate empty or partial output as success

#### Scenario: Non-switching subcommand runs directly
- **WHEN** `opsx account add`, `opsx cluster delete`, `opsx login`, `opsx status`, `opsx ls`, or `opsx init zsh` runs with the installed wrapper
- **THEN** the command runs directly rather than through `shell-switch`
- **AND** its normal interactive or display output is preserved

#### Scenario: default command runs directly and leaves the terminal unchanged
- **WHEN** `opsx default dev` runs with the installed wrapper
- **THEN** the command runs directly rather than through `shell-switch`
- **AND** the invoking terminal's `AWS_PROFILE`, regions, and `KUBECONFIG` are unchanged; only the shared `[default]` credentials file is written

### Requirement: Export values are shell-safe by validation
The system SHALL guarantee that every value emitted as `export KEY=value` is shell-safe by validating it, not by assuming it. Account/cluster aliases and the mode token SHALL be validated at config load against a strict charset (e.g. `^[A-Za-z0-9._-]+$`); the export emitter SHALL refuse any value containing shell metacharacters. This prevents a hostile or mistyped alias from injecting commands into the user's live shell via `eval`.

Every value opsx itself constructs and emits — in particular the `KUBECONFIG` path, whose file name encodes the cluster alias — MUST pass the export validator. The kubeconfig-alias encoding therefore MUST use only characters the emitter accepts as eval-safe. A config-valid cluster alias (including one containing `.`) MUST NOT produce a `KUBECONFIG` path that the emitter rejects; `opsx kube` failing for a dot-containing alias is a defect, because alias-dot encoding and the export charset must agree.

#### Scenario: Dot-containing cluster alias still produces an eval-safe export
- **WHEN** a cluster alias contains `.` (e.g. `dev.syd`) and `opsx shell-switch kube dev.syd` runs
- **THEN** the emitted `export KUBECONFIG=<path>` passes the export validator and is safe to `eval`
- **AND** `opsx kube` does NOT fail with an "unsafe value" error for the encoded path

#### Scenario: Hostile alias is rejected, not exported
- **WHEN** a config alias contains shell metacharacters (e.g. `dev;rm -rf ~`)
- **THEN** config load (or the export emitter) fails with a clear error
- **AND** no `export` line containing the metacharacters is ever written to stdout

### Requirement: Manual eval fallback
The system SHALL always emit copy-pasteable `export` lines so the same switch works via a manual `eval "$(opsx shell-switch …)"` even when no shell function is installed.

#### Scenario: Manual fallback applies the switch
- **WHEN** a user runs `eval "$(opsx shell-switch use dev)"` without the installed function
- **THEN** `AWS_PROFILE` is exported into the current shell from the emitted export line

### Requirement: PowerShell shell integration
The system SHALL treat PowerShell as a distinct shell dialect, not as a POSIX shell. The CLI SHALL provide a PowerShell init generator and a PowerShell-safe shell-switch output mode for `use`, `kube`, `mode`, and `region`.

The PowerShell dialect MUST emit only PowerShell-native environment assignment lines to stdout, such as `$env:AWS_PROFILE = "dev.admin"`, `$env:AWS_REGION = "ap-southeast-2"`, `$env:KUBECONFIG = "..."`, and `$env:OPSX_MODE = "opr"`. It MUST NOT emit POSIX `export` statements.

#### Scenario: init powershell emits a wrapper
- **WHEN** `opsx init powershell` runs
- **THEN** it emits a PowerShell function suitable for loading from the user's PowerShell profile
- **AND** the function routes only `use`, `kube`, `mode`, and `region` through `shell-switch`
- **AND** all other subcommands run directly

#### Scenario: PowerShell account switch emits only PowerShell assignments
- **WHEN** `opsx shell-switch --shell powershell use dev` runs
- **THEN** stdout contains only PowerShell assignments for `AWS_PROFILE`, `AWS_REGION`, and `AWS_DEFAULT_REGION`
- **AND** stdout does not contain a POSIX `export` statement

#### Scenario: PowerShell cluster switch emits account and kubeconfig assignments
- **WHEN** `opsx shell-switch --shell powershell kube dev-syd` runs
- **THEN** stdout contains only PowerShell assignments for `AWS_PROFILE`, `AWS_REGION`, `AWS_DEFAULT_REGION`, and `KUBECONFIG`
- **AND** all prompts, confirmations, logs, and errors go to stderr

#### Scenario: PowerShell mode switch emits only mode assignment
- **WHEN** `opsx shell-switch --shell powershell mode opr` runs
- **THEN** stdout contains only a PowerShell assignment for `OPSX_MODE`

#### Scenario: PowerShell region switch emits only region assignments
- **WHEN** `opsx shell-switch --shell powershell region` runs and the operator selects a region
- **THEN** stdout contains only PowerShell assignments for `AWS_REGION` and `AWS_DEFAULT_REGION`
- **AND** the interactive menu appears only on stderr

#### Scenario: PowerShell wrapper refuses non-assignment stdout
- **WHEN** a switching subcommand exits successfully but emits text that is not a recognized PowerShell environment assignment
- **THEN** the PowerShell wrapper does not evaluate that text
- **AND** it returns non-zero instead of executing arbitrary text

#### Scenario: PowerShell help is displayed directly
- **WHEN** `opsx use --help`, `opsx kube -h`, or `opsx region --help` runs through the PowerShell wrapper
- **THEN** the subcommand help text is displayed directly
- **AND** the help text is not evaluated as PowerShell commands

