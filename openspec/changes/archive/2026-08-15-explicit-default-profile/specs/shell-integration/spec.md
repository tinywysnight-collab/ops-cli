## MODIFIED Requirements

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
