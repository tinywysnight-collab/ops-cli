## MODIFIED Requirements

### Requirement: Shared default profile is not a switch target
The system SHALL NOT write the shared `[default]` profile during `opsx use`. `[default]` is a single latest-wins section in the AWS credentials file and therefore cannot support multi-terminal isolation. Current-terminal switching MUST happen through shell integration that exports `AWS_PROFILE`.

The system SHALL provide `opsx default <account-alias>` as an explicit opt-in command that ensures the selected citizen profile for the current mode, then copies that profile's STS credentials into `[default]`. This command is intentionally latest-wins and is for shells or tools that do not consume `AWS_PROFILE`.

For compatibility with older opsx versions that may have written `[default]`, `opsx logout` SHALL clear `[default]` only when it holds a complete opsx STS session (access key, secret key, and session token). A `[default]` maintained by other means — for example the user's long-term access keys — SHALL be preserved and reported, never deleted.

`opsx default` SHALL reuse the cached citizen profile when one is unexpired instead of forcing a fresh AssumeRole; `[default]` then receives the cached credentials with their existing expiry. When the citizen profile must be ensured (cache miss), `opsx default` SHALL fail without writing `[default]` if the alias is unknown, the master credentials are missing or stale (surfaced as the re-login hint), or the ensured profile is incomplete (missing session token). A valid cached citizen profile is reused without consulting master credentials, exactly like `opsx use`.

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

#### Scenario: default command reuses the cached profile
- **WHEN** `opsx default dev` runs and `[dev.<mode>]` is cached and unexpired
- **THEN** `[default]` receives those cached credentials with their existing expiry
- **AND** no additional AssumeRole is issued for the command itself

#### Scenario: default command is latest-wins across invocations
- **WHEN** `opsx default dev` and later `opsx default prod` run (including from different terminals)
- **THEN** `[default]` holds the credentials of whichever invocation wrote last
- **AND** no cross-terminal isolation is claimed for `[default]`

#### Scenario: default command fails on unknown alias
- **WHEN** `opsx default ghost` runs and `ghost` is not a configured account alias
- **THEN** the command fails with an error naming the alias
- **AND** `[default]` is not written

#### Scenario: default command fails on expired master
- **WHEN** `opsx default dev` runs with no unexpired `[dev.<mode>]` cache and the master credentials for the current mode are missing or stale
- **THEN** the command fails with the re-login hint (`opsx login [--opr]`)
- **AND** `[default]` is not written

#### Scenario: default command fails on incomplete profile
- **WHEN** `opsx default dev` runs and the ensured `[dev.<mode>]` profile lacks a session token
- **THEN** the command fails with an error naming the incomplete profile
- **AND** `[default]` is not written

#### Scenario: logout clears an opsx-shaped default
- **WHEN** `opsx logout` runs and `[default]` holds a complete STS session written by an older opsx version
- **THEN** the `[default]` profile is removed as compatibility cleanup along with the other purged opsx-managed profiles

#### Scenario: logout preserves a user-maintained default
- **WHEN** `opsx logout` runs and `[default]` contains the user's own long-term credentials without a session token
- **THEN** `[default]` is left untouched
- **AND** logout reports it as preserved

