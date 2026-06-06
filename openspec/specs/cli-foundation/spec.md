# cli-foundation Specification

## Purpose

Provide the single static `opsx` binary, its Cobra command skeleton, signal-cancellable root context, top-level error handling, agreed package layout, and verification commands that gate the build.

## Requirements

### Requirement: Single static binary
The system SHALL build as a single static Go binary named `opsx` with `CGO_ENABLED=0` and no runtime dependency, cross-compilable for darwin (primary) and linux (secondary).

#### Scenario: Build produces a static binary
- **WHEN** `make build` runs on a clean checkout
- **THEN** a single static binary `opsx` is produced with `CGO_ENABLED=0`
- **AND** it cross-compiles for darwin and linux targets

#### Scenario: Version is reported
- **WHEN** `opsx --version` runs
- **THEN** a version string is printed

### Requirement: Cobra root and subcommand skeleton
The system SHALL expose a Cobra root command listing the planned subcommands (`login`, `use`, `kube`, `mode`, `status`, `ls`, `init`, `shell-switch`).

#### Scenario: Help lists subcommands
- **WHEN** `opsx` runs with no arguments
- **THEN** Cobra prints help listing the planned subcommands as available commands

### Requirement: Single top-level error handler
The system SHALL return errors from command `RunE` functions up to a single top-level handler in `cmd/opsx/main.go`, which prints to stderr and sets a non-zero exit code. Commands MUST NOT call `os.Exit` or `panic` for expected conditions.

#### Scenario: Errors propagate to the top-level handler
- **WHEN** any command returns an error
- **THEN** the single top-level handler prints the error to stderr and exits non-zero
- **AND** no `panic` or `os.Exit` is invoked inside `RunE`

### Requirement: Signal-cancellable root context
The system SHALL run the Cobra root command under a context derived from `signal.NotifyContext` for `SIGINT` and `SIGTERM`, so pressing Ctrl-C or receiving a termination signal cancels in-flight STS calls, Entra HTTP requests, MFA polling, external command execution, and advisory-lock waits promptly and cleanly.

Using `Execute()` with a background context does NOT satisfy this requirement; commands MUST receive the signal-aware context through `cmd.Context()`.

#### Scenario: Ctrl-C cancels an in-flight wait
- **WHEN** a command is blocked waiting on the advisory lock, an STS/HTTP call, `aws eks update-kubeconfig`, or MFA approval, and the process receives SIGINT
- **THEN** the root context is canceled
- **AND** the blocked operation returns a cancellation error rather than relying on a hard process kill

#### Scenario: Root command uses a signal-aware context
- **WHEN** the binary starts
- **THEN** the root command is executed with a `signal.NotifyContext(SIGINT, SIGTERM)` context, for example via `ExecuteContext`
- **AND** every command's `cmd.Context()` is cancellable by those signals

### Requirement: Agreed package layout
The system SHALL follow the agreed structure: `cmd/opsx/main.go` plus `internal/{cli,config,auth,creds,state,kube,shell,paths}`, with `go.mod` declaring cobra, aws-sdk-go-v2 (config, credentials, service/sts), yaml.v3, and gofrs/flock.

#### Scenario: Repository structure matches the architecture
- **WHEN** the repository is inspected
- **THEN** the layout matches `cmd/opsx/main.go` + `internal/{cli,config,auth,creds,state,kube,shell,paths}`
- **AND** `go.mod` declares cobra, aws-sdk-go-v2 (config, credentials, service/sts), yaml.v3, and gofrs/flock

### Requirement: Verification commands fail on violations
The system SHALL provide verification commands that fail non-zero when the required check fails. In particular, `make lint` MUST fail if `gofmt` would change any tracked Go file; printing unformatted filenames while exiting successfully does not satisfy the lint gate.

#### Scenario: make lint fails on unformatted Go files
- **WHEN** a Go file is not gofmt-formatted
- **THEN** `make lint` exits non-zero
- **AND** the developer sees which files need formatting

### Requirement: Windows executable target
The system SHALL provide a Windows amd64 executable target that builds a `opsx.exe` binary with `CGO_ENABLED=0` and without runtime dependencies.

#### Scenario: Windows cross-compile target
- **WHEN** the Windows build target runs
- **THEN** it produces `bin/opsx-windows-amd64.exe`
- **AND** the build uses `CGO_ENABLED=0 GOOS=windows GOARCH=amd64`
