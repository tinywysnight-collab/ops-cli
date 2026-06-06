## ADDED Requirements

### Requirement: shell-switch emits only assignment lines to stdout
The system SHALL ensure `opsx shell-switch …` prints ONLY shell-specific environment assignment lines to stdout (one per line), routing all prompts, logs, and errors to stderr. The default/POSIX dialect SHALL emit plain `export KEY=value` lines safe to consume via `eval`; other dialects SHALL emit only their native assignment syntax.

#### Scenario: Export-only stdout for account switch
- **WHEN** `opsx shell-switch use dev` runs
- **THEN** stdout contains only `export AWS_PROFILE=dev.admin`
- **AND** all prompts and logs go to stderr

#### Scenario: Export-only stdout for cluster switch
- **WHEN** `opsx shell-switch kube dev-syd` runs
- **THEN** stdout contains only the two export lines `export AWS_PROFILE=dev.<mode>` and `export KUBECONFIG=<dev-syd kubeconfig path>` (one per line)
- **AND** no other output appears on stdout

#### Scenario: Export-only stdout for mode
- **WHEN** `opsx shell-switch mode opr` runs
- **THEN** stdout contains only `export OPSX_MODE=opr`

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
- **THEN** it emits a `.cmd` batch wrapper that routes only `use`, `kube`, and `mode` through `shell-switch --shell cmd`
- **AND** all other subcommands run `opsx.exe` directly

#### Scenario: cmd account switch emits only cmd assignments
- **WHEN** `opsx shell-switch --shell cmd use dev` runs
- **THEN** stdout contains only `set "AWS_PROFILE=dev.admin"`
- **AND** stdout does not contain POSIX `export` or PowerShell `$env:` syntax

#### Scenario: cmd cluster switch emits account and kubeconfig assignments
- **WHEN** `opsx shell-switch --shell cmd kube dev-syd` runs
- **THEN** stdout contains only cmd `set "KEY=value"` assignments for `AWS_PROFILE` and `KUBECONFIG`
- **AND** all prompts, confirmations, logs, and errors go to stderr

### Requirement: Wrapper only intercepts switching subcommands
The installed shell function SHALL route ONLY the switching subcommands (`use`, `kube`, `mode`) through `opsx shell-switch … | eval`, and SHALL execute every other subcommand (`login`, `status`, `ls`, `init`, `--help`, `--version`, …) directly so they behave identically to invoking `opsx` without the wrapper. Wrapping all subcommands through `shell-switch` (which exposes only `use`/`kube`/`mode`) is a defect: it turns `login`/`status`/`ls`/`init` into "unknown command" no-ops.

The wrapper MUST handle global flags that appear before the subcommand, such as `opsx --mode opr use dev` and `opsx --opr kube dev-syd`. Those invocations MUST still route switching subcommands through `shell-switch`; running the bare binary is insufficient because a child process cannot update the parent shell environment.

The wrapper MUST preserve failure semantics. If `command opsx shell-switch ...` fails, the shell function MUST return a non-zero status and MUST NOT `eval` empty or partial output as a successful switch.

The wrapper MUST NOT `eval` non-`export` output. A switching subcommand can emit exit-0 stdout that is NOT an export line — most notably cobra's `--help`/`-h` text, which is printed to stdout with exit status 0. Evaluating that text injects "command not found" errors into the user's live shell. The wrapper SHALL therefore: (a) route any `-h`/`--help` invocation of a switching subcommand directly to the bare binary (so the real subcommand help is shown and never `eval`'d); and (b) as defense-in-depth, `eval` only lines that are blank or begin with `export `, refusing any other stdout with a non-zero status rather than executing it. The contract "switching subcommands emit only export lines" MUST be enforced by the wrapper, not assumed.

#### Scenario: Help flag on a switching subcommand is shown, never eval'd
- **WHEN** `opsx use --help` (or `opsx kube -h`) runs with the installed function
- **THEN** the subcommand's help text is printed to the user
- **AND** the help text is NOT `eval`'d into the shell (no "command not found" errors)

#### Scenario: Non-export stdout is refused, not executed
- **WHEN** a switching subcommand exits 0 but emits a line that is not an `export` statement
- **THEN** the wrapper does NOT `eval` that line
- **AND** it returns a non-zero status instead of executing arbitrary stdout as a shell command

#### Scenario: Switching subcommand is applied via eval
- **WHEN** `opsx use dev` runs with the installed function
- **THEN** the command is dispatched through `opsx shell-switch use dev` and its `export` line is `eval`'d into the current shell

#### Scenario: Switching subcommand with leading global flags is applied via eval
- **WHEN** `opsx --mode opr use dev` or `opsx --opr kube dev-syd` runs with the installed function
- **THEN** the command is dispatched through `opsx shell-switch` with the same arguments
- **AND** its `export` line is `eval`'d into the current shell

#### Scenario: Failed shell-switch fails the wrapper
- **WHEN** `opsx use missing-alias` runs with the installed function and `shell-switch` returns an error
- **THEN** the shell function returns a non-zero status
- **AND** it does NOT silently succeed after evaluating empty output

#### Scenario: Non-switching subcommand runs directly
- **WHEN** `opsx login`, `opsx status`, `opsx ls`, or `opsx init zsh` runs with the installed function
- **THEN** the command runs directly (not via `shell-switch`) and produces its normal output
- **AND** it is never silently swallowed by an `eval` of empty output

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
The system SHALL treat PowerShell as a distinct shell dialect, not as a POSIX shell. The CLI SHALL provide a PowerShell init generator and a PowerShell-safe shell-switch output mode for `use`, `kube`, and `mode`.

The PowerShell dialect MUST emit only PowerShell-native environment assignment lines to stdout, such as `$env:AWS_PROFILE = "dev.admin"`, `$env:KUBECONFIG = "..."`, and `$env:OPSX_MODE = "opr"`. It MUST NOT emit POSIX `export` statements for PowerShell.

#### Scenario: init powershell emits a wrapper
- **WHEN** `opsx init powershell` runs
- **THEN** it emits a PowerShell function suitable for loading from the user's PowerShell profile
- **AND** the function routes only `use`, `kube`, and `mode` through `shell-switch`
- **AND** all other subcommands run directly

#### Scenario: PowerShell account switch emits only PowerShell assignments
- **WHEN** `opsx shell-switch --shell powershell use dev` runs
- **THEN** stdout contains only a PowerShell assignment for `AWS_PROFILE`
- **AND** stdout does not contain a POSIX `export` statement

#### Scenario: PowerShell cluster switch emits account and kubeconfig assignments
- **WHEN** `opsx shell-switch --shell powershell kube dev-syd` runs
- **THEN** stdout contains only PowerShell assignments for `AWS_PROFILE` and `KUBECONFIG`
- **AND** all prompts, confirmations, logs, and errors go to stderr

#### Scenario: PowerShell mode switch emits only mode assignment
- **WHEN** `opsx shell-switch --shell powershell mode opr` runs
- **THEN** stdout contains only a PowerShell assignment for `OPSX_MODE`

#### Scenario: PowerShell wrapper refuses non-assignment stdout
- **WHEN** a switching subcommand exits 0 but emits text that is not a recognized PowerShell environment assignment
- **THEN** the PowerShell wrapper does not evaluate that text
- **AND** it returns a non-zero status instead of executing arbitrary stdout

#### Scenario: PowerShell help is displayed directly
- **WHEN** `opsx use --help` or `opsx kube -h` runs through the PowerShell wrapper
- **THEN** the subcommand help text is displayed directly
- **AND** the help text is not evaluated as PowerShell commands
