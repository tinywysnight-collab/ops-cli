## ADDED Requirements

### Requirement: Switch account by alias
The system SHALL, on `opsx use <account-alias>`, assume the citizen role for the current terminal's mode using cached master credentials, write the `[<alias>.<mode>]` profile, update state, and complete in under 2 seconds without triggering MFA.

The account alias MUST be validated against the current loaded config before any cached citizen profile is reused. A profile left behind from an older config MUST NOT allow switching to an account alias that has since been removed or is no longer valid.

The citizen `AssumeRole` STS client MUST be built with the region resolved per the config "Configuration-driven STS region" rule (`accounts.<alias>.region` → `auth.region` → environment), so `opsx use` does not depend on an ambient `AWS_REGION`.

#### Scenario: Successful account switch
- **WHEN** `opsx use dev` runs with valid cached master creds for the current mode
- **THEN** the citizen role is assumed using the account's resolved region, the `[dev.<mode>]` profile is written, and state is updated
- **AND** it completes in under 2 seconds with no MFA

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
