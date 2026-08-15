## MODIFIED Requirements

### Requirement: Single static binary
The system SHALL build as a single static Go binary named `opsx` with `CGO_ENABLED=0` and no runtime dependency, cross-compilable for darwin (primary) and linux (secondary).

#### Scenario: Build produces a static binary
- **WHEN** `make build` runs on a clean checkout
- **THEN** a single static binary `opsx` is produced with `CGO_ENABLED=0`
- **AND** `make cross` cross-compiles the darwin, linux, and windows targets in one invocation

#### Scenario: Version is reported
- **WHEN** `opsx --version` runs
- **THEN** a version string is printed
