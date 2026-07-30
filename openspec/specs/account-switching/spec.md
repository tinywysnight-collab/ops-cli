# account-switching Specification

## Purpose

Switch between AWS citizen accounts by short alias for the current terminal's mode using cached master credentials, with per-terminal isolation and credential reuse within the validity window.

## Requirements

### Requirement: Switch account by alias
The system SHALL, on `opsx use <account-alias>`, assume the citizen role for the current terminal's mode using cached master credentials, write the `[<alias>.<mode>]` profile, update state, and complete in under 2 seconds without triggering MFA.

The account alias MUST be validated against the current loaded config before any cached citizen profile is reused. A profile left behind from an older config MUST NOT allow switching to an account alias that has since been removed or is no longer valid.

The citizen `AssumeRole` STS client MUST be built with the region resolved per the config "Configuration-driven STS region" rule (`accounts.<alias>.region` → `auth.region` → environment), so `opsx use` does not depend on an ambient `AWS_REGION`.

When `opsx use` runs through shell integration (`shell-switch` or an installed wrapper), the terminal session SHALL also receive `AWS_REGION` and `AWS_DEFAULT_REGION` set to the account's resolved region. This lets subsequent `aws` CLI commands in that terminal use the configured session region without passing `--region`.

`opsx use` SHALL accept an optional `--region <region>` flag that overrides the exported session region for that terminal (the multi-region-per-account case: running plain `aws` against a non-home region of the same account, without going through a cluster). When omitted, the account's config-resolved region is used (unchanged default). The override value MUST be validated to a plain region token (`[A-Za-z0-9._-]+`) — a strict subset of the shell-safe export charset — so it can never inject into the emitted `export AWS_REGION=<value>` line; this validation applies on the bare `opsx use` path too, which never reaches the shell emitter. `opsx kube` does NOT take `--region`: a cluster's own region is authoritative for its terminal.

#### Scenario: Successful account switch
- **WHEN** `opsx use dev` runs with valid cached master creds for the current mode
- **THEN** the citizen role is assumed using the account's resolved region, the `[dev.<mode>]` profile is written, and state is updated
- **AND** it completes in under 2 seconds with no MFA

#### Scenario: Account switch exports session region
- **WHEN** `opsx use dev` runs through shell integration
- **THEN** the terminal has `AWS_PROFILE=dev.<mode>` exported
- **AND** `AWS_REGION` and `AWS_DEFAULT_REGION` are set to the account's resolved region

#### Scenario: --region overrides the session region
- **WHEN** `opsx use dev --region us-west-2` runs through shell integration
- **THEN** `AWS_REGION` and `AWS_DEFAULT_REGION` are set to `us-west-2` rather than the account's config-resolved region
- **AND** `AWS_PROFILE=dev.<mode>` is still exported

#### Scenario: --region rejects an unsafe value
- **WHEN** `opsx use dev --region` is given a value containing shell metacharacters or whitespace
- **THEN** the command fails with a clear `invalid --region` error and exports nothing

#### Scenario: Expired master creds block the switch
- **WHEN** `opsx use` runs with expired master creds
- **THEN** it fails with the re-login message and exits non-zero

#### Scenario: Removed alias cannot be reused from stale cache
- **WHEN** a `[dev.<mode>]` profile exists locally but `dev` is no longer present in `config.yaml`
- **THEN** `opsx use dev` fails with an unknown-account/config error
- **AND** it does NOT reuse the stale local profile

### Requirement: Reuse unexpired citizen credentials
The system SHALL reuse an existing citizen profile when it is present and not expired (honoring the expiry skew buffer), assuming the citizen role again only when the profile is missing or stale. Re-running `AssumeRole` and rewriting the profile on every `opsx use`/`opsx kube` is wasteful and undermines the "switch in seconds" goal.

#### Scenario: No redundant assume within the validity window
- **WHEN** `opsx use dev` runs twice in succession while the `[dev.<mode>]` profile is still valid
- **THEN** the second invocation reuses the cached citizen credentials without a second `AssumeRole`

### Requirement: Per-terminal mode selection
The system SHALL set the current terminal's default mode via `opsx mode admin|opr`, allow a per-command flag to override it symmetrically, and never persist mode to disk. The override MUST be able to force either mode (not only `opr`): provide a symmetric flag (e.g. `--admin` / `--mode admin|opr`) and define precedence as flag > `OPSX_MODE` > default `admin`.

Conflicting mode flags MUST NOT be resolved silently. When both `--mode` and `--opr` (or any two mutually-exclusive mode selectors) are supplied on the same invocation and they disagree, the command MUST fail with a clear error rather than silently honoring one and ignoring the other.

An explicitly-negated boolean shorthand (`--opr=false`) MUST be treated as "opr is not selected" and MUST NOT be reported as a conflict against `--mode`. The conflict rule applies only when a selector actively requests a mode that disagrees with another active selector; `--mode admin --opr=false` resolves to admin without error.

#### Scenario: Mode set for the terminal
- **WHEN** `opsx mode opr` runs
- **THEN** the resulting export sets `OPSX_MODE=opr`
- **AND** the mode is never persisted to disk

#### Scenario: Per-command flag overrides mode in both directions
- **WHEN** a command is run with the admin override while `OPSX_MODE=opr`
- **THEN** the command runs in admin mode for that invocation
- **WHEN** a command is run with `--opr` while `OPSX_MODE=admin`
- **THEN** the command runs in opr mode for that invocation

#### Scenario: Conflicting mode flags are rejected
- **WHEN** a command is run with both `--opr` and `--mode admin`
- **THEN** the command fails with a clear error naming the conflict
- **AND** it does NOT silently pick one mode

#### Scenario: Agreeing redundant flags are accepted
- **WHEN** a command is run with both `--opr` and `--mode opr`
- **THEN** the command runs in opr mode without error

#### Scenario: Explicitly-negated --opr is not a conflict
- **WHEN** a command is run with `--mode admin --opr=false`
- **THEN** the command runs in admin mode without error
- **AND** `--opr=false` is treated as "opr not selected", not as a conflicting selector

### Requirement: Per-terminal AWS_PROFILE isolation
The system SHALL scope `AWS_PROFILE` to each terminal so that two terminals using different accounts or modes never collide.

#### Scenario: Two terminals do not collide
- **WHEN** two terminals each run `opsx use` for different accounts/modes
- **THEN** each exports its own `AWS_PROFILE`
- **AND** the two never collide (validated by a deliberate cross-terminal test)

### Requirement: Shared default profile is not a switch target
The system SHALL NOT write the shared `[default]` profile during `opsx use`. `[default]` is a single latest-wins section in the AWS credentials file and therefore cannot support multi-terminal isolation. Current-terminal switching MUST happen through shell integration that exports `AWS_PROFILE`.

The system SHALL provide `opsx default <account-alias>` as an explicit opt-in command that ensures the selected citizen profile for the current mode, then copies that profile's STS credentials into `[default]`. This command is intentionally latest-wins and is for shells or tools that do not consume `AWS_PROFILE`.

For compatibility with older opsx versions that may have written `[default]`, `opsx logout` SHALL continue to clear `[default]` along with other opsx-managed profiles.

#### Scenario: use writes only the named profile
- **WHEN** `opsx use dev` runs
- **THEN** `[dev.<mode>]` is written with the freshly-assumed citizen credentials
- **AND** `[default]` is not written or rewritten

#### Scenario: default is preserved on cache reuse
- **WHEN** `[dev.<mode>]` is reused from cache and `[default]` already contains unrelated credentials
- **THEN** `[default]` remains unchanged

#### Scenario: default command writes default explicitly
- **WHEN** `opsx default dev` runs
- **THEN** `[dev.<mode>]` is ensured
- **AND** `[default]` is written with the `[dev.<mode>]` credentials

#### Scenario: logout clears default
- **WHEN** `opsx logout` runs
- **THEN** the `[default]` profile is removed as compatibility cleanup along with the other purged opsx-managed profiles
