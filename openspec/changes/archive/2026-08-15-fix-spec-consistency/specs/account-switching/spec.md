## MODIFIED Requirements

### Requirement: Switch account by alias
The system SHALL, on `opsx use <account-alias>`, assume the citizen role for the current terminal's mode using cached master credentials, write the `[<alias>.<mode>]` profile, update state, and complete in seconds without triggering MFA (an SLO, not a hard deadline: lock acquisition alone may wait up to its bounded timeout under contention).

The account alias MUST be validated against the current loaded config before any cached citizen profile is reused. A profile left behind from an older config MUST NOT allow switching to an account alias that has since been removed or is no longer valid.

The citizen `AssumeRole` STS client MUST be built with the region resolved per the config "Configuration-driven STS region" rule (`accounts.<alias>.region`, required and allowlisted per the config capability), so `opsx use` does not depend on an ambient `AWS_REGION`.

When `opsx use` runs through shell integration (`shell-switch` or an installed wrapper), the terminal session SHALL also receive `AWS_REGION` and `AWS_DEFAULT_REGION` set to the account's resolved region. This lets subsequent `aws` CLI commands in that terminal use the configured session region without passing `--region`.

`opsx use` SHALL accept an optional `--region <region>` flag that overrides the exported session region for that terminal (the multi-region-per-account case: running plain `aws` against a non-home region of the same account, without going through a cluster). When omitted, the account's config-resolved region is used (unchanged default). The override value MUST be validated to a plain region token (`[A-Za-z0-9._-]+`) — a strict subset of the shell-safe export charset — so it can never inject into the emitted `export AWS_REGION=<value>` line; this validation applies on the bare `opsx use` path too, which never reaches the shell emitter. `opsx kube` does NOT take `--region`: a cluster's own region is authoritative for its terminal.

#### Scenario: Successful account switch
- **WHEN** `opsx use dev` runs with valid cached master creds for the current mode
- **THEN** the citizen role is assumed using the account's resolved region, the `[dev.<mode>]` profile is written, and state is updated
- **AND** it completes in seconds with no MFA under normal conditions

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
