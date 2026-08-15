## MODIFIED Requirements

### Requirement: Expiry detection with sentinels
The system SHALL detect expired credentials by comparing recorded expiry against the current time and return sentinel errors `ErrMasterExpired` / `ErrCitizenExpired` that are detectable via `errors.Is`. On reaching the top-level handler, the system SHALL print a clear re-login message and exit non-zero.

#### Scenario: Past expiry returns a sentinel
- **WHEN** a profile whose recorded expiry is in the past is checked
- **THEN** `ErrMasterExpired` is returned
- **AND** it is detectable via `errors.Is`

#### Scenario: Expiry uses a skew buffer
- **WHEN** a profile's recorded expiry is within the safety buffer (e.g. 2 minutes) of now
- **THEN** it is treated as expired so no switch assumes with about-to-lapse credentials

#### Scenario: Expired citizen cache refreshes transparently
- **WHEN** a citizen profile's recorded expiry has lapsed and the operator switches to it again
- **THEN** no citizen-expiry sentinel is surfaced to the operator
- **AND** the profile is re-ensured through the normal assume/reuse path


#### Scenario: Expiry sentinel formatted for the user
- **WHEN** an expiry sentinel reaches the top-level handler
- **THEN** a clear `master credentials expired — run: opsx login [--opr]` message is printed to stderr
- **AND** the process exits non-zero
