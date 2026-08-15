## MODIFIED Requirements

### Requirement: Secure password collection
The system SHALL read the login password with no echo, using `OPSX_PASSWORD` instead of prompting when that env var is set. The password MUST never be written to config, credentials, state, or logs. On the interactive path it is held as `[]byte` and zeroized after use; when supplied via `OPSX_PASSWORD` it is inherently a process-visible environment string, and HTTP form encoding necessarily produces a transient encoded copy — both are documented trade-offs rather than zeroizable buffers.

#### Scenario: No-echo prompt and env fallback
- **WHEN** login collects the password
- **THEN** it is read with no echo, or taken from `OPSX_PASSWORD` if set
- **AND** the password is never written to config/creds/state/logs and is zeroized after use
