## MODIFIED Requirements

### Requirement: Purge cached credentials and state
The system SHALL provide `opsx logout [--opr] [--all]` to remove cached credentials and their state entries without hand-editing files. By default it SHALL purge the master profile for the current or selected mode and that mode's citizen profiles; `--all` SHALL purge every opsx-managed profile and state entry for both modes. The command MUST run the entire plan-verify-delete sequence under ONE shared advisory lock window — reloading state, planning targets, verifying shapes, and deleting credentials and state together — so a concurrent login or use cannot leave live credentials behind after logout reports success. Before deleting any target, it MUST verify the profile holds a complete opsx STS session (access key, secret key, and session token); a present-but-non-STS profile is user-maintained and MUST be preserved and reported. It MUST preserve unrelated profiles and comments in `~/.aws/credentials` and leave a file that is valid for the AWS CLI.

#### Scenario: Logout clears the master cache for a mode
- **WHEN** `opsx logout` runs for a mode whose master credentials are cached
- **THEN** the `[master_<mode>]` profile and its state entry are removed
- **AND** unrelated profiles and comments in `~/.aws/credentials` are preserved
- **AND** a subsequent `opsx use` for that mode reports the master credentials as missing or expired

#### Scenario: Logout --all clears everything opsx manages
- **WHEN** `opsx logout --all` runs
- **THEN** all opsx-managed master and citizen profiles and their state entries are removed for both modes
- **AND** non-opsx profiles remain untouched

#### Scenario: Logout preserves non-STS profiles
- **WHEN** a state entry (corrupted or hand-edited) names a profile that holds long-term keys without a session token, or `[default]` is user-maintained
- **THEN** logout does not delete that profile
- **AND** it reports the profile as preserved
