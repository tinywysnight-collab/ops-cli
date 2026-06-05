## ADDED Requirements

### Requirement: Show current terminal context
The system SHALL, on `opsx status`, show the current terminal's active account, mode, cluster, region, and credential expiry, reading expiry from `state.json` without mutating credentials or state. The region shown SHALL be the active cluster's region when a cluster is active, otherwise the account's resolved STS region, so the multi-region context is visible.

Because `status` derives the active profile from the live `AWS_PROFILE`, `KUBECONFIG`, and `OPSX_MODE` environment, it MUST reconcile that environment with recorded state and clearly distinguish three cases: the env profile matches a known state entry; `AWS_PROFILE` is set but is not opsx-managed or has no state entry; or no opsx context is set. A foreign `AWS_PROFILE` MUST be explained explicitly rather than shown as an ambiguous "unknown" state.

The displayed active cluster MUST reflect THIS terminal. `state.json` is shared and keyed by profile, so two terminals using the same `<alias.mode>` profile for different clusters would overwrite each other's recorded cluster. `status` MUST therefore derive the active cluster from the per-terminal `KUBECONFIG` env (authoritative for this terminal), or clearly label any state-derived cluster as "last recorded for this profile" so it is never presented as this terminal's definitive cluster when it may be another terminal's.

The displayed mode MUST reflect the active profile, not a stale `OPSX_MODE`. The active profile's recorded mode (from its `state.json` entry; the profile name `<alias>.<mode>` also encodes it) is authoritative — it is the mode the active credentials were actually minted under. `OPSX_MODE` is only the per-terminal default for the NEXT command and can legitimately disagree with the active profile: e.g. a terminal with `OPSX_MODE=admin` that ran `opsx use prod --opr` has `AWS_PROFILE=prod.opr` but an untouched `OPSX_MODE=admin`. `status` MUST therefore show the active profile's recorded mode when a state entry exists, and MAY fall back to `OPSX_MODE` only for a profile opsx has no state for, labeling it as a default rather than the profile's identity. It MUST NOT let `OPSX_MODE` override the active profile's real mode.

#### Scenario: Mode reflects the active profile, not a stale OPSX_MODE
- **WHEN** `OPSX_MODE=admin` is set but the active `AWS_PROFILE` is an opsx-managed profile recorded with mode `opr` (e.g. after `opsx use prod --opr`)
- **THEN** `status` shows `Mode: opr` (the active profile's real mode)
- **AND** it does NOT show `admin` from the stale `OPSX_MODE` env

#### Scenario: Active context displayed
- **WHEN** `opsx status` runs in a terminal with `AWS_PROFILE` / `KUBECONFIG` / `OPSX_MODE` set to an opsx-managed profile
- **THEN** it shows the active account, mode, cluster, region, and credential expiry (read from `state.json`) to stdout
- **AND** the region shown is the active cluster's region (or the account's resolved STS region when no cluster is active)

#### Scenario: Foreign AWS_PROFILE is explained, not mislabeled
- **WHEN** `AWS_PROFILE` is set to a profile opsx did not create and no matching state entry exists
- **THEN** `status` clearly states the active profile is not opsx-managed or has no recorded expiry
- **AND** it does not imply an opsx error or show a misleading expired/unknown message without explanation

#### Scenario: Context shown after a bare kube
- **WHEN** `opsx kube dev-syd` ran without a prior `opsx use`, exporting `AWS_PROFILE` and `KUBECONFIG`, and `opsx status` runs in the same terminal
- **THEN** it shows the account, cluster `dev-syd`, and expiry rather than "no active context"

#### Scenario: Cluster shown reflects this terminal
- **WHEN** two terminals share the same `<alias.mode>` profile but have different `KUBECONFIG` clusters
- **THEN** each terminal's `opsx status` reflects its own cluster (from `KUBECONFIG`), not the other terminal's last-recorded cluster

#### Scenario: No active context
- **WHEN** `opsx status` runs with no active opsx context
- **THEN** it clearly states nothing is active and suggests `opsx use` / `opsx login`

#### Scenario: Expired creds shown with hint
- **WHEN** `opsx status` runs and the active profile's creds are expired
- **THEN** expiry is shown as expired with the re-login hint

#### Scenario: status is read-only
- **WHEN** `opsx status` reads state
- **THEN** it does not mutate credentials or state

### Requirement: List configured aliases
The system SHALL, on `opsx ls`, print configured account aliases with their description and region, and cluster aliases with their account and region, handling empty sections and invalid config gracefully. When an account has no explicit `region` configured, `ls` MAY omit it or note that it falls back to `auth.region`.

#### Scenario: Aliases listed
- **WHEN** `opsx ls` runs with a config containing accounts and clusters
- **THEN** it prints account aliases with their description and region, and cluster aliases with their account and region, to stdout

#### Scenario: Empty section
- **WHEN** `opsx ls` runs with an empty or absent `accounts` or `clusters` section
- **THEN** it prints a friendly "none configured" line for that section rather than erroring

#### Scenario: Invalid config
- **WHEN** `opsx ls` runs against an invalid config
- **THEN** the validation error is printed to stderr and exit is non-zero
