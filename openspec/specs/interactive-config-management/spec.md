# interactive-config-management Specification

## Purpose
TTY-only interactive account and cluster add/delete with validation, confirmation, dependency protection, and safe local YAML persistence — operators manage config.yaml resources without hand-editing.

## Requirements
### Requirement: Configuration mutation is interactive and TTY-only
The system SHALL expose `opsx account add`, `opsx account delete`, `opsx cluster add`, and `opsx cluster delete` as interactive commands. These commands MUST require terminal stdin, MUST NOT accept non-interactive resource-field flags, and MUST fail clearly without modifying configuration when stdin is not a TTY.

#### Scenario: Non-TTY mutation is rejected
- **WHEN** any account or cluster add/delete command runs with piped or redirected stdin
- **THEN** it exits non-zero with a clear message that interactive terminal input is required
- **AND** `config.yaml` remains unchanged

### Requirement: Interactive account creation
`opsx account add` SHALL prompt for an alias, 12-digit account ID, optional description, and a region selected by number from the configured `regions` sequence. It MUST reject invalid aliases, duplicate aliases, and account IDs already owned by another alias, re-prompting without offering overwrite or edit. Before writing, it SHALL display the complete proposed account and require explicit confirmation.

#### Scenario: Account is added after confirmation
- **WHEN** the operator supplies a new valid alias and account ID, optionally supplies a description, selects an allowed region, and answers `y` or `yes` at the final `[y/N]` prompt
- **THEN** exactly that account is appended to the `accounts` mapping
- **AND** no existing account, cluster, auth, or region value is modified

#### Scenario: Duplicate account is not overwritten
- **WHEN** the operator enters an existing alias or an account ID already used by another alias
- **THEN** the command names the conflict and requests a different value
- **AND** it never offers to overwrite or edit the existing account

#### Scenario: Blank description is accepted
- **WHEN** the operator leaves the account description blank but supplies every required field
- **THEN** the account can be confirmed and added without a description

### Requirement: Interactive cluster creation
`opsx cluster add` SHALL prompt for a new alias, an existing account selected from an alias-sorted numbered menu, a region selected in configured allowlist order, and the real EKS cluster name. It SHALL validate the complete candidate and require explicit confirmation before writing. An existing cluster alias MUST never be overwritten; a different alias that identifies the same account, region, and real name MAY be added only after the confirmation summary warns about the duplicate identity.

#### Scenario: Cluster is added after confirmation
- **WHEN** the operator supplies a new valid alias, selects an existing account and allowed region, supplies a cluster name, and confirms
- **THEN** exactly that cluster is appended to the `clusters` mapping with a valid account reference

#### Scenario: Cluster add requires an account
- **WHEN** `opsx cluster add` runs with no configured accounts
- **THEN** it exits non-zero and tells the operator to run `opsx account add` first
- **AND** it does not prompt for an unresolvable account reference

#### Scenario: Duplicate cluster identity warns but remains allowed
- **WHEN** a new cluster alias has the same account, region, and real EKS name as an existing alias
- **THEN** the final summary names the existing alias and warns about the duplicate identity
- **AND** explicit confirmation can still add the new alias

### Requirement: Interactive resource deletion
Account and cluster deletion SHALL use alias-sorted numbered menus, display the selected resource's complete configuration, and require explicit confirmation. Deleting the last entry SHALL retain the corresponding YAML section as an explicit empty mapping. With no resources in the selected section, deletion SHALL report the empty state and exit successfully.

#### Scenario: Cluster is deleted after confirmation
- **WHEN** the operator selects a configured cluster and explicitly confirms deletion
- **THEN** only the selected cluster mapping entry is removed

#### Scenario: Account is deleted after confirmation
- **WHEN** the operator selects an unreferenced configured account and explicitly confirms deletion
- **THEN** only the selected account mapping entry is removed

#### Scenario: Empty delete is a normal result
- **WHEN** account deletion has no accounts or cluster deletion has no clusters
- **THEN** the command prints `No accounts configured.` or `No clusters configured.`
- **AND** exits successfully without changing the file

#### Scenario: Last entry leaves an explicit empty section
- **WHEN** the final account or cluster is deleted
- **THEN** the corresponding section remains present as an explicit empty mapping

### Requirement: Account deletion protects cluster references
The system MUST refuse to delete an account referenced by any cluster. It SHALL exit non-zero, list every blocking cluster alias, instruct the operator to delete clusters first with `opsx cluster delete`, and MUST NOT offer cascade deletion.

#### Scenario: Referenced account deletion is blocked
- **WHEN** the selected account is referenced by one or more clusters
- **THEN** deletion exits non-zero and lists all referencing cluster aliases
- **AND** neither the account nor any cluster is removed

### Requirement: Confirmation and cancellation are safe
Every add and delete operation SHALL use a default-negative `[y/N]` confirmation. Only case-insensitive `y` or `yes` SHALL authorize a write. `n`, `no`, blank confirmation, EOF, or Ctrl-C SHALL cancel without mutation, print `Cancelled.`, and return success; invalid prompt values SHALL re-prompt.

#### Scenario: Default-negative confirmation cancels
- **WHEN** the operator submits an empty final confirmation
- **THEN** the command prints `Cancelled.`, exits successfully, and leaves `config.yaml` unchanged

#### Scenario: Invalid selection re-prompts
- **WHEN** the operator enters a non-existent menu number or malformed field value
- **THEN** the command explains the validation failure and requests another value
- **AND** it does not write a partial resource

### Requirement: Configuration writes preserve content and integrity
Mutation commands SHALL require an existing, readable, fully valid `config.yaml`; they MUST NOT initialize or repair missing/invalid auth or region policy. After confirmation, the system SHALL acquire the shared opsx lock, reread the latest file, repeat relevant conflict and dependency checks, edit only the target YAML mapping node, validate the complete candidate configuration, and replace the file atomically and durably while preserving the existing file permissions and symlink target.

Untouched mappings, sequence order, scalar quoting style, and comments SHALL remain intact wherever supported by YAML node round-tripping; byte-for-byte whitespace preservation is not required. Mutation MUST reject multiple YAML documents, anchors, aliases, and merge keys rather than perform an ambiguous rewrite. No automatic backup or undo file SHALL be created.

#### Scenario: Concurrent opsx additions do not lose data
- **WHEN** two terminals confirm distinct valid additions concurrently
- **THEN** their read-modify-write transactions serialize under the shared lock
- **AND** the final valid configuration contains both additions

#### Scenario: Existing comments and unrelated sections survive
- **WHEN** a resource is added or deleted in a conventional single-document config
- **THEN** comments, ordering, quoting, `auth`, `regions`, and all untouched resources are preserved
- **AND** only insignificant serialization whitespace may change

#### Scenario: Unsupported YAML mutation is rejected
- **WHEN** a mutation command loads a config containing multiple documents, anchors, aliases, or merge keys
- **THEN** it exits non-zero with a clear unsupported-structure message
- **AND** the original bytes remain unchanged

#### Scenario: Missing or invalid config is not repaired
- **WHEN** a mutation command encounters a missing file or a config that fails full validation
- **THEN** it exits non-zero with the validation/path error
- **AND** it does not create or partially repair `config.yaml`

### Requirement: Deletion has no runtime cleanup side effects
Deleting an account or cluster SHALL modify only `config.yaml`. It MUST NOT delete AWS credential profiles, state entries, generated kubeconfig files, or environment variables in any terminal. If the selected resource is active in the invoking terminal, the command SHALL warn before confirmation but MAY proceed; after success it SHALL state that runtime artifacts were retained.

#### Scenario: Active resource deletion warns but proceeds
- **WHEN** the selected account or cluster is active in the invoking terminal and has no dependency blocking deletion
- **THEN** the confirmation summary warns that the terminal environment will not be reset
- **AND** explicit confirmation deletes only the configuration entry

#### Scenario: Runtime artifacts are retained
- **WHEN** an account or cluster deletion succeeds
- **THEN** credentials, state, kubeconfig files, and live terminal variables remain unchanged
- **AND** the command tells the operator that only configuration was removed

