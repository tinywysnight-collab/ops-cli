# config Specification

## Purpose

Load and validate `~/.config/opsx/config.yaml` into typed structs with up-front structural validation, resolve the STS region from configuration, and compose role ARNs entirely from config values.

## Requirements

### Requirement: Load and validate YAML config
The system SHALL load `~/.config/opsx/config.yaml` and decode its `accounts`, `clusters`, and `auth` sections into typed structs, validating referential integrity.

#### Scenario: Valid config decodes
- **WHEN** a valid config with `accounts`, `clusters`, and `auth` sections is loaded
- **THEN** all three sections decode into typed structs

#### Scenario: Cluster references unknown account
- **WHEN** a cluster's `account` references an account alias not present in `accounts`
- **THEN** validation fails with a clear error naming the offending cluster and the missing account alias

#### Scenario: Missing config file
- **WHEN** a command needs config but the file is absent
- **THEN** a clear error tells the user the expected path and to create it

### Requirement: Up-front structural validation
The system SHALL validate the full config at load time, failing once with all offending names rather than surfacing scattered errors later at ARN-composition time. Validation MUST cover: non-empty `account_id` for each account; non-empty `region` and `name` for each cluster; presence of `auth.master_account_id` and `auth.saml_provider_arn`; mode-keyed `master_roles`/`citizen_roles` entries for the supported modes; and rejection of duplicate `account_id` values across accounts.

Validation MUST additionally enforce well-formedness of values interpolated into ARNs and external commands, so malformed config fails at load with a name-bearing message rather than producing a malformed ARN that fails opaquely at STS-call time:
- every `account_id` in account entries and `auth.master_account_id` MUST be exactly 12 ASCII digits;
- every cluster `region` MUST be non-empty after trimming and MUST NOT contain whitespace.
- optional Entra endpoint overrides `auth.entra.base_url`, `auth.entra.ms_login_url`, and `auth.entra.myapps_url` MUST be absolute `http` or `https` URLs when set.
- optional `auth.entra.debug` MUST decode as a boolean and default to `false` when omitted.

#### Scenario: Account missing account_id
- **WHEN** an account entry has an empty `account_id`
- **THEN** validation fails at load naming the offending account alias

#### Scenario: Malformed account_id rejected
- **WHEN** any `account_id` is not exactly 12 digits, for example `"123"`, `"acme-prod"`, or `"1234 5678 9012"`
- **THEN** validation fails at load naming the offending account alias or `auth.master_account_id`
- **AND** no ARN is composed from the malformed value

#### Scenario: Cluster missing region or name
- **WHEN** a cluster entry has an empty `region` or `name`
- **THEN** validation fails at load naming the offending cluster alias and missing field

#### Scenario: Whitespace region rejected
- **WHEN** a cluster `region` is blank or contains whitespace, for example `" "` or `"ap southeast 2"`
- **THEN** validation fails at load naming the offending cluster alias

#### Scenario: Malformed Entra endpoint URL rejected
- **WHEN** an Entra endpoint override is blank, contains whitespace, is relative, or uses a non-http(s) scheme
- **THEN** validation fails at load naming the offending `auth.entra.*` URL field

#### Scenario: Entra debug flag is optional
- **WHEN** `auth.entra.debug` is omitted
- **THEN** Entra debug logging remains disabled
- **AND** the sample config does not need to include the field

#### Scenario: Missing auth fields
- **WHEN** `auth.master_account_id`, `auth.saml_provider_arn`, or a required mode role entry is missing
- **THEN** validation fails at load naming the missing field

#### Scenario: Duplicate account IDs
- **WHEN** two accounts share the same `account_id`
- **THEN** validation fails at load naming both offending aliases

### Requirement: Config-driven mode set
The system SHALL derive the set of valid `mode` values entirely from configuration: the valid modes are the key set of `auth.master_roles`, which MUST be identical to the key set of `auth.citizen_roles`. That combined key set MUST include `admin` (the default mode) and `opr` (the `--opr` shorthand); any additional entry (e.g. `prod-admin`) becomes a selectable mode with no code change, chosen via the existing `--mode <name>` flag. Every mode token MUST match `[A-Za-z0-9_-]+` — letters, digits, `_`, and `-`, with NO `.` — because a mode is used both as an exported `AWS_PROFILE` segment and as a filesystem path segment (kubeconfig storage), and excluding `.` keeps it reserved as the profile-name separator and prevents a mode like `".."` from escaping the kube directory. Each configured mode MUST have a default citizen role in `auth.citizen_roles[mode]`, used unless overridden per-switch (see the account-switching spec's `--role` requirement).

#### Scenario: Additional mode is config-only
- **WHEN** `auth.master_roles` and `auth.citizen_roles` both gain a new key, e.g. `prod-admin`
- **THEN** `--mode prod-admin` becomes valid on `opsx login` and `opsx use` with no code change
- **AND** `prod-admin` composes ARNs and profile names exactly like `admin`/`opr`

#### Scenario: Mismatched mode key sets rejected
- **WHEN** `auth.master_roles` and `auth.citizen_roles` do not define the identical set of mode keys
- **THEN** validation fails at load naming the offending mode and which map it is missing from

#### Scenario: Mode token charset enforced
- **WHEN** a mode key in `master_roles`/`citizen_roles` contains a character outside `[A-Za-z0-9_-]+`, for example `"prod.admin"` or `"prod admin"`
- **THEN** validation fails at load naming the offending mode token
- **AND** no ARN, profile name, or kube path is composed from the malformed token

#### Scenario: admin and opr remain mandatory
- **WHEN** `auth.master_roles` or `auth.citizen_roles` omits `admin` or `opr`
- **THEN** validation fails at load naming the missing required mode

### Requirement: Configuration-driven STS region
The system SHALL source the AWS region for every STS call from configuration rather than relying solely on ambient environment, because AWS SDK Go v2 requires a region to resolve the STS endpoint and `opsx login` / `opsx use` would otherwise fail on a machine with no `AWS_REGION`/`AWS_DEFAULT_REGION` and no default in `~/.aws/config`.

The schema SHALL support:
- `auth.region` — the region used for the master `AssumeRoleWithSAML` call.
- `accounts.<alias>.region` (optional) — the region used for that account's citizen `AssumeRole`. An account may run EKS in several regions, so this account-level STS/home region is distinct from each cluster's own `clusters.<alias>.region`.
- `clusters.<alias>.region` — unchanged; the EKS region passed to `aws eks update-kubeconfig`.

Region resolution order SHALL be: for citizen STS, `accounts.<alias>.region` → `auth.region` → `AWS_REGION`/`AWS_DEFAULT_REGION`; for master STS, `auth.region` → `AWS_REGION`/`AWS_DEFAULT_REGION`. When no region resolves, the command MUST fail with a clear, name-bearing error (which region field to set) rather than the opaque SDK "an AWS region is required" message.

The resolved citizen region SHALL also be exported as the terminal session's `AWS_REGION` and `AWS_DEFAULT_REGION` when `opsx use` runs through shell integration. When `opsx kube` runs through shell integration, the active cluster's `clusters.<alias>.region` SHALL be exported as `AWS_REGION` and `AWS_DEFAULT_REGION`, because subsequent `aws eks ...` commands in that session should default to the active cluster's region.

#### Scenario: Configured region is applied to STS
- **WHEN** `auth.region` (and optionally `accounts.<alias>.region`) is set and `opsx login` / `opsx use` runs
- **THEN** the master and citizen STS clients are built with the resolved configured region
- **AND** neither call depends on `AWS_REGION` being present in the environment

#### Scenario: Configured region becomes the terminal AWS CLI region
- **WHEN** `opsx use dev` runs through shell integration
- **THEN** `AWS_REGION` and `AWS_DEFAULT_REGION` are set to the resolved citizen region for `dev`
- **WHEN** `opsx kube dev-syd` runs through shell integration
- **THEN** `AWS_REGION` and `AWS_DEFAULT_REGION` are set to `clusters.dev-syd.region`

#### Scenario: No resolvable region fails clearly
- **WHEN** no region is configured and no `AWS_REGION`/`AWS_DEFAULT_REGION` is set
- **THEN** the command fails with a clear error naming the region field to set
- **AND** it does NOT surface the opaque SDK endpoint-resolution error

### Requirement: Config-driven ARN composition
The system SHALL compose role ARNs entirely from config values, with no hardcoded account IDs or role names. The master ARN MUST be `arn:aws:iam::{master_account_id}:role/{master_roles[mode]}` and the citizen ARN MUST be `arn:aws:iam::{accounts[alias].account_id}:role/{citizen_roles[mode]}`.

#### Scenario: ARNs composed from config and mode
- **WHEN** ARNs are composed for a loaded config and a given mode
- **THEN** the master ARN is `arn:aws:iam::{master_account_id}:role/{master_roles[mode]}`
- **AND** the citizen ARN is `arn:aws:iam::{accounts[alias].account_id}:role/{citizen_roles[mode]}`

#### Scenario: Role name changes flow through
- **WHEN** role names in `master_roles` / `citizen_roles` are changed
- **THEN** the composed ARNs change accordingly with no code changes
