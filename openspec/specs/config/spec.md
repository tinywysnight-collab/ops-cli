# config Specification

## Purpose

Load and validate `~/.config/opsx/config.yaml` into typed structs with up-front structural validation, resolve the STS region from configuration, and compose role ARNs entirely from config values.
## Requirements
### Requirement: Load and validate YAML config
The system SHALL load `~/.config/opsx/config.yaml` and decode its ordered `regions`, `accounts`, `clusters`, and `auth` sections into typed structs, validating referential integrity and region-policy membership.

#### Scenario: Valid config decodes
- **WHEN** a valid config with `regions`, `accounts`, `clusters`, and `auth` sections is loaded
- **THEN** all four sections decode into typed values
- **AND** the declared region order is preserved

#### Scenario: Cluster references unknown account
- **WHEN** a cluster's `account` references an account alias not present in `accounts`
- **THEN** validation fails with a clear error naming the offending cluster and the missing account alias

#### Scenario: Missing config file
- **WHEN** a command needs config but the file is absent
- **THEN** a clear error tells the user the expected path and to create it

### Requirement: Up-front structural validation
The system SHALL validate the full config at load time, failing with name-bearing errors rather than surfacing scattered failures later. Validation MUST cover: a non-empty ordered `regions` sequence with no duplicates; non-empty `account_id` and `region` for each account; non-empty account reference, `region`, and `name` for each cluster; membership of every account and cluster region and optional `auth.region` in the allowlist; presence of `auth.master_account_id` and `auth.saml_provider_arn`; mode-keyed `master_roles`/`citizen_roles` entries for the supported modes; rejection of duplicate `account_id` values across accounts; and case-insensitive uniqueness of cluster aliases, because per-cluster kubeconfig file names collide on case-insensitive filesystems (default APFS and NTFS) when two aliases differ only by letter case.

Validation MUST additionally enforce well-formedness of values interpolated into ARNs and external commands:
- every `account_id` in account entries and `auth.master_account_id` MUST be exactly 12 ASCII digits;
- every allowlisted, account, cluster, and configured auth region MUST be non-empty after trimming and MUST NOT contain whitespace;
- optional Entra endpoint overrides `auth.entra.base_url`, `auth.entra.ms_login_url`, and `auth.entra.myapps_url` MUST be absolute `http` or `https` URLs when set;
- optional `auth.entra.debug` MUST decode as a boolean and default to `false` when omitted.

#### Scenario: Region allowlist is missing or empty
- **WHEN** `regions` is absent or contains no values
- **THEN** validation fails with a clear error requiring at least one allowed region

#### Scenario: Duplicate allowed region is rejected
- **WHEN** the same region token appears more than once in `regions`
- **THEN** validation fails and names the duplicate value

#### Scenario: Account missing account_id
- **WHEN** an account entry has an empty `account_id`
- **THEN** validation fails at load naming the offending account alias

#### Scenario: Account missing region
- **WHEN** an account entry has an empty or absent `region`
- **THEN** validation fails at load naming the offending account alias and field

#### Scenario: Malformed account_id rejected
- **WHEN** any `account_id` is not exactly 12 digits, for example `"123"`, `"acme-prod"`, or `"1234 5678 9012"`
- **THEN** validation fails at load naming the offending account alias or `auth.master_account_id`
- **AND** no ARN is composed from the malformed value

#### Scenario: Cluster missing region or name
- **WHEN** a cluster entry has an empty `region` or `name`
- **THEN** validation fails at load naming the offending cluster alias and missing field

#### Scenario: Case-variant cluster aliases are rejected
- **WHEN** two cluster aliases differ only by letter case, for example `dev-syd` and `DEV-SYD`
- **THEN** validation fails at load naming both aliases and the kubeconfig file-name collision on case-insensitive filesystems

#### Scenario: Whitespace region rejected
- **WHEN** an allowed, account, cluster, or configured auth region is blank or contains whitespace, for example `" "` or `"ap southeast 2"`
- **THEN** validation fails at load naming the offending value or field

#### Scenario: Resource region is outside policy
- **WHEN** an account or cluster region is not present in `regions`
- **THEN** validation fails naming the offending resource, its region, and the allowlist requirement

#### Scenario: Auth region is outside policy
- **WHEN** `auth.region` is configured but is not present in `regions`
- **THEN** validation fails naming `auth.region` and the disallowed value

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

### Requirement: Configuration-driven STS region
The system SHALL source the AWS region for every STS call from configuration rather than relying solely on ambient environment. The schema SHALL require `accounts.<alias>.region` as that account's citizen AssumeRole/home region and `clusters.<alias>.region` as the EKS region passed to `aws eks update-kubeconfig`. It SHALL support optional `auth.region` for the master `AssumeRoleWithSAML` call. Every configured region MUST belong to the top-level `regions` allowlist.

For citizen STS, the selected account's required region SHALL be used. For master STS, resolution order SHALL remain `auth.region` then `AWS_REGION`/`AWS_DEFAULT_REGION`; when no master region resolves, login MUST fail with a clear error naming `auth.region` rather than an opaque SDK error.

The account's region SHALL be exported as the terminal's `AWS_REGION` and `AWS_DEFAULT_REGION` when `opsx use` runs through shell integration, unless an explicit allowed `--region` override is supplied. `opsx use --region` MUST reject values absent from `regions`. When `opsx kube` runs through shell integration, the active cluster's configured region SHALL be exported.

#### Scenario: Configured region is applied to STS
- **WHEN** `opsx login` or `opsx use` runs with valid configured regions
- **THEN** the master STS client uses `auth.region` when present and the citizen STS operation uses the selected account's required region
- **AND** the citizen call does not depend on ambient region variables

#### Scenario: Configured region becomes the terminal AWS CLI region
- **WHEN** `opsx use dev` runs through shell integration without a region override
- **THEN** `AWS_REGION` and `AWS_DEFAULT_REGION` are set to `accounts.dev.region`
- **WHEN** `opsx kube dev-syd` runs through shell integration
- **THEN** both variables are set to `clusters.dev-syd.region`

#### Scenario: Allowed use override is applied
- **WHEN** `opsx use dev --region ap-northeast-1` runs and `ap-northeast-1` is in `regions`
- **THEN** the terminal region variables use `ap-northeast-1`

#### Scenario: Disallowed use override is rejected
- **WHEN** `opsx use dev --region us-west-2` runs and `us-west-2` is absent from `regions`
- **THEN** switching fails clearly and lists the allowed region values
- **AND** no disallowed region assignment is emitted

#### Scenario: No resolvable master region fails clearly
- **WHEN** `auth.region` is empty and no `AWS_REGION`/`AWS_DEFAULT_REGION` is set
- **THEN** login fails with a clear error naming `auth.region`
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

