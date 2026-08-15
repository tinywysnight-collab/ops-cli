## MODIFIED Requirements

### Requirement: Cobra root and subcommand skeleton
The system SHALL expose a Cobra root command listing `login`, `use`, `default`, `kube`, `mode`, `region`, `account`, `cluster`, `status`, `ls`, `init`, and the internal `shell-switch`. The `account` and `cluster` groups SHALL each expose only `add` and `delete`; they MUST NOT expose edit, update, or rename commands.

#### Scenario: Help lists top-level subcommands
- **WHEN** `opsx` runs with no arguments
- **THEN** Cobra prints help listing the top-level commands, including `account`, `cluster`, and `region`

#### Scenario: Resource help exposes no edit operation
- **WHEN** `opsx account --help` or `opsx cluster --help` runs
- **THEN** help lists `add` and `delete`
- **AND** does not list edit, update, rename, or overwrite operations
