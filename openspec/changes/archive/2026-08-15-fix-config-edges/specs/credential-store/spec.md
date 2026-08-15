## ADDED Requirements

### Requirement: STS responses are validated before caching
The system SHALL validate every STS AssumeRole / AssumeRoleWithSAML response before writing it as a cached profile: credentials with an empty access key, secret key, or session token, or a missing (zero) expiry, MUST be refused with a clear error naming the assumed role. A merely past expiry is left to normal expiry detection.

#### Scenario: Incomplete STS response is refused
- **WHEN** STS returns credentials with an empty session token or omits Expiration
- **THEN** the assume fails with an error naming the role
- **AND** nothing is written to `~/.aws/credentials` or `state.json`
