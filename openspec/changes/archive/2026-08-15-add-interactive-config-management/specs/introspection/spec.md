## MODIFIED Requirements

### Requirement: Show current terminal context
The system SHALL, on `opsx status`, show the current terminal's active account, mode, cluster, region, and credential expiry, reading expiry from `state.json` without mutating credentials or state. The region shown SHALL be the terminal's actual `AWS_REGION` when set, falling back to the config-derived active cluster or account region only when the terminal exported none. When no AWS profile is active but `AWS_REGION` is set, status SHALL still display that terminal region alongside the no-active-context message.

Because `status` derives the active profile from the live `AWS_PROFILE`, `KUBECONFIG`, and `OPSX_MODE` environment, it MUST reconcile that environment with recorded state and clearly distinguish three cases: the env profile matches a known state entry; `AWS_PROFILE` is set but is not opsx-managed or has no state entry; or no opsx context is set. A foreign `AWS_PROFILE` MUST be explained explicitly rather than shown as an ambiguous "unknown" state.

The displayed active cluster MUST reflect THIS terminal. `state.json` is shared and keyed by profile, so two terminals using the same `<alias.mode>` profile for different clusters would overwrite each other's recorded cluster. `status` MUST therefore derive the active cluster from the per-terminal `KUBECONFIG` env (authoritative for this terminal), or clearly label any state-derived cluster as "last recorded for this profile" so it is never presented as this terminal's definitive cluster when it may be another terminal's.

The displayed mode MUST reflect the active profile, not a stale `OPSX_MODE`. The active profile's recorded mode is authoritative because it is the mode the active credentials were minted under. `OPSX_MODE` is only the per-terminal default for the next command and can legitimately disagree with the active profile. `status` MUST therefore show the active profile's recorded mode when a state entry exists, and MAY fall back to `OPSX_MODE` only for a profile opsx has no state for, labeling it as a default rather than the profile's identity. It MUST NOT let `OPSX_MODE` override the active profile's real mode.

#### Scenario: Mode reflects the active profile, not a stale OPSX_MODE
- **WHEN** `OPSX_MODE=admin` is set but the active `AWS_PROFILE` is an opsx-managed profile recorded with mode `opr`
- **THEN** `status` shows `Mode: opr`
- **AND** it does NOT show `admin` from the stale `OPSX_MODE`

#### Scenario: Active context displayed
- **WHEN** `opsx status` runs in a terminal with `AWS_PROFILE` / `KUBECONFIG` / `OPSX_MODE` set to an opsx-managed profile
- **THEN** it shows the active account, mode, cluster, region, and credential expiry to stdout
- **AND** the region shown is the terminal's `AWS_REGION` when set, else the config-derived cluster or account region

#### Scenario: Region reflects the terminal's AWS_REGION override
- **WHEN** `opsx status` runs in a terminal whose `AWS_REGION` was set by `opsx region` or `opsx use --region`
- **THEN** it shows that terminal's actual region
- **AND** it does NOT show the account's configured home region instead

#### Scenario: Foreign AWS_PROFILE is explained, not mislabeled
- **WHEN** `AWS_PROFILE` is set to a profile opsx did not create and no matching state entry exists
- **THEN** `status` clearly states the active profile is not opsx-managed or has no recorded expiry
- **AND** it does not imply an opsx error or show a misleading expired/unknown message without explanation

#### Scenario: Context shown after a bare kube
- **WHEN** `opsx kube dev-syd` ran without a prior `opsx use`, exporting `AWS_PROFILE` and `KUBECONFIG`, and `opsx status` runs in the same terminal
- **THEN** it shows the account, cluster `dev-syd`, and expiry rather than "no active context"

#### Scenario: Cluster shown reflects this terminal
- **WHEN** two terminals share the same `<alias.mode>` profile but have different `KUBECONFIG` clusters
- **THEN** each terminal's `opsx status` reflects its own cluster from `KUBECONFIG`

#### Scenario: No active context with a terminal region
- **WHEN** `opsx status` runs with no active profile or cluster but `AWS_REGION` is set
- **THEN** it clearly states no opsx account or cluster is active
- **AND** it displays the current terminal region

#### Scenario: No active context and no terminal region
- **WHEN** `opsx status` runs with no active opsx context and no `AWS_REGION`
- **THEN** it clearly states nothing is active and suggests `opsx use` / `opsx login`

#### Scenario: Expired creds shown with hint
- **WHEN** `opsx status` runs and the active profile's creds are expired
- **THEN** expiry is shown as expired with the re-login hint

#### Scenario: status is read-only
- **WHEN** `opsx status` reads environment and state
- **THEN** it does not mutate credentials, configuration, or state

### Requirement: List configured aliases
The system SHALL, on `opsx ls`, print two human-readable, aligned tables sorted by alias. The accounts table SHALL show `ALIAS`, `ACCOUNT ID`, `REGION`, and `DESCRIPTION`, using `-` for an empty description. The clusters table SHALL show `ALIAS`, `ACCOUNT`, `ACCOUNT ID`, `REGION`, and real EKS `NAME`, resolving each account ID through the validated account reference. The output SHALL NOT list the top-level region allowlist and is not a stable machine-readable interface.

#### Scenario: Detailed account and cluster tables are listed
- **WHEN** `opsx ls` runs with configured accounts and clusters
- **THEN** stdout contains the account and cluster columns and values defined above
- **AND** rows in each table are sorted by alias

#### Scenario: Region allowlist is not listed
- **WHEN** `opsx ls` runs
- **THEN** it does not render a separate regions section or dump the top-level allowlist

#### Scenario: Empty section
- **WHEN** `opsx ls` runs with an explicit empty `accounts` or `clusters` mapping
- **THEN** it prints a friendly "none configured" line for that section rather than erroring

#### Scenario: Invalid config
- **WHEN** `opsx ls` runs against an invalid config
- **THEN** the validation error is printed to stderr and exit is non-zero
