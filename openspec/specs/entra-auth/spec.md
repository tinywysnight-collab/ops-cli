# entra-auth Specification

## Purpose

Authenticate once with Entra + ADFS + MFA behind a pluggable SAML seam, exchange the assertion for master STS credentials for both roles, collect the password securely, honor context during MFA, sanitize the STS session name, route through the system proxy, and exercise the flow end-to-end against a fake provider.
## Requirements
### Requirement: Authentication behind a pluggable SAML seam
The system SHALL perform Entra authentication exclusively through a `SAMLProvider.FetchAssertion(ctx, role)` seam in `internal/auth`. No Entra/HTTP logic MAY leak outside `internal/auth`; tests MUST use a fake provider.

#### Scenario: Login uses the provider seam
- **WHEN** `opsx login` is invoked
- **THEN** authentication goes through `SAMLProvider.FetchAssertion(ctx, role)`
- **AND** no Entra HTTP logic exists outside `internal/auth`

### Requirement: Entra ADFS MFA assertion fetch
The system SHALL implement the production Entra assertion fetch inside `internal/auth/entra.go`. When no pre-obtained assertion is supplied, `EntraSAMLProvider.FetchAssertion` SHALL use `auth.entra.username`, top-level `auth.entra.app_id`, and the configured/default Entra endpoint URLs to bootstrap the Entra federated application, submit the ADFS password form, relay through Microsoft login/MyApps, poll interactive MFA when required, print the MFA number/prompt to stderr, and extract the returned `SAMLResponse`.

`auth.entra.base_url`, `auth.entra.ms_login_url`, and `auth.entra.myapps_url` SHALL be configurable and SHALL default to `https://auth.entra.io` when omitted. The provider SHALL use top-level `auth.entra.app_id`; older `auth.entra.tenant_id` and `auth.entra.domain_map` entries are not part of the schema.

When optional `auth.entra.debug` is `true`, the provider SHALL write Entra troubleshooting logs to stderr. These logs SHALL include only safe step, sanitized endpoint, status, and count information; they MUST NOT include passwords, cookies, SAML assertions, MFA flow tokens, session IDs, canary values, or full request URLs containing query parameters.

`OPSX_SAML_ASSERTION` and `OPSX_SAML_ASSERTION_FILE` SHALL remain accepted escape hatches; when either supplies a non-empty assertion, the provider SHALL return it without prompting or making Entra HTTP calls.

#### Scenario: Real Entra flow extracts assertion
- **WHEN** `opsx login` runs with no `OPSX_SAML_ASSERTION` / `OPSX_SAML_ASSERTION_FILE`
- **AND** the configured Entra/ADFS endpoints return the expected bootstrap, federation, relay, MFA, and SAML pages
- **THEN** the provider returns the extracted `SAMLResponse`
- **AND** the password and SAML assertion are never logged or persisted

#### Scenario: Configured Entra endpoints are used
- **WHEN** `auth.entra.base_url`, `auth.entra.ms_login_url`, or `auth.entra.myapps_url` are set
- **THEN** `EntraSAMLProvider` uses those URLs instead of the default `https://auth.entra.io`

#### Scenario: Debug logging is safe
- **WHEN** `auth.entra.debug` is enabled and the Entra flow runs
- **THEN** stderr contains step-level troubleshooting logs for bootstrap, federation, ADFS relay, Microsoft relay, MFA polling, and SAML extraction
- **AND** no password, SAML assertion, MFA token, session ID, canary value, cookie, or query-string-bearing URL is logged

#### Scenario: Escape hatch bypasses Entra HTTP flow
- **WHEN** `OPSX_SAML_ASSERTION` or a non-empty `OPSX_SAML_ASSERTION_FILE` is set
- **THEN** `FetchAssertion` returns that assertion directly
- **AND** no password prompt, MFA poll, or Entra HTTP request is required

### Requirement: Master STS caching for both roles
The system SHALL exchange the SAML assertion via `AssumeRoleWithSAML` and cache master credentials to `[master_admin]` (default) or `[master_awsopr]` (`--opr`) with a 1h expiry recorded in state. Both caches MUST coexist without overwriting each other.

The `AssumeRoleWithSAML` STS client MUST be built with the region resolved per the config "Configuration-driven STS region" rule (`auth.region` → environment), so login does not depend on an ambient `AWS_REGION`.

The system SHALL fetch the SAML assertion before building the AWS STS client. This keeps the interactive Entra/password path from being delayed by AWS SDK config and credential discovery.

#### Scenario: Master credentials cached with expiry
- **WHEN** login completes with a returned SAML assertion
- **THEN** `AssumeRoleWithSAML` caches master creds to `[master_admin]` (default) or `[master_awsopr]` (`--opr`)
- **AND** a 1h expiry is recorded in state

#### Scenario: Login asks Entra before building STS
- **WHEN** `opsx login` starts
- **THEN** it calls the SAML provider before constructing the AWS STS client
- **AND** AWS SDK config loading cannot delay the password prompt

#### Scenario: Both master roles coexist
- **WHEN** both master roles are logged in
- **THEN** `[master_admin]` and `[master_awsopr]` caches coexist without overwriting each other

### Requirement: Secure password collection
The system SHALL read the login password with no echo, using `OPSX_PASSWORD` instead of prompting when that env var is set. The password MUST never be written to config, credentials, state, or logs. On the interactive path it is held as `[]byte` and zeroized after use; when supplied via `OPSX_PASSWORD` it is inherently a process-visible environment string, and HTTP form encoding necessarily produces a transient encoded copy — both are documented trade-offs rather than zeroizable buffers.

#### Scenario: No-echo prompt and env fallback
- **WHEN** login collects the password
- **THEN** it is read with no echo, or taken from `OPSX_PASSWORD` if set
- **AND** the password is never written to config/creds/state/logs and is zeroized after use

### Requirement: Interactive MFA honors context
The system SHALL print the MFA number/prompt to stderr and poll for approval in a way that honors `ctx` timeout and cancellation, exiting cleanly on Ctrl-C.

#### Scenario: MFA polling respects cancellation
- **WHEN** login awaits MFA approval
- **THEN** the number/prompt prints to stderr
- **AND** polling honors `ctx` timeout and cancellation so Ctrl-C exits cleanly

### Requirement: STS session name is sanitized
The system SHALL build the STS `RoleSessionName` so it always satisfies the STS constraint `[\w+=,.@-]{2,64}`. The OS username MUST be sanitized (strip/replace disallowed characters, truncate to 64) before composing `opsx-<user>`, with a guaranteed-valid fallback when the username is empty or unavailable. A raw domain login (`DOMAIN\user`) or a name with spaces MUST NOT reach `AssumeRole`.

Truncation MUST be safe and MUST NOT depend on an implicit invariant of an earlier step. The implementation MUST either truncate on rune boundaries (never splitting a multi-byte rune and never emitting invalid UTF-8) or pin the "result is ASCII before truncation" invariant with an explicit test, so a future change to the sanitization charset cannot silently produce an invalid session name.

#### Scenario: Hostile username yields a valid session name
- **WHEN** the OS username contains disallowed characters (e.g. `ACME\jane doe`)
- **THEN** the composed `RoleSessionName` matches `[\w+=,.@-]{2,64}`
- **AND** `AssumeRole` is never called with an invalid session name

#### Scenario: Over-long username truncates safely
- **WHEN** the sanitized `opsx-<user>` would exceed 64 characters
- **THEN** the result is truncated to at most 64 without splitting a multi-byte rune
- **AND** the result is valid UTF-8 and still matches `[\w+=,.@-]{2,64}`

### Requirement: Login flow is exercised end-to-end against a fake provider
The system SHALL provide an integration test that drives `login` → `use` → `kube` → `status` through the real command wiring using the fake `SAMLProvider`, so the orchestration is verified without depending on live Entra/AWS services. `README.md` SHALL document the real Entra path, the accepted assertion escape hatch, and the need to live-verify the company-specific HTTP/MFA path on the proxy-gated company machine.

#### Scenario: End-to-end flow verified with a fake provider
- **WHEN** the integration test runs `login`→`use`→`kube`→`status` with the fake provider
- **THEN** master creds are cached, a citizen profile is written, a kubeconfig is produced, and status reflects the active context

#### Scenario: Real-flow boundary is documented
- **WHEN** a user reads `README.md`
- **THEN** it states that `opsx login` drives the Entra+ADFS+MFA flow
- **AND** it documents the assertion escape hatch and company-network live-verification boundary

### Requirement: Network honors system proxy
The system SHALL route all Entra HTTP and AWS STS calls through the system proxy environment variables, with no proxy hardcoded anywhere.

#### Scenario: Calls use system proxy
- **WHEN** Entra HTTP and STS calls are made with system proxy env vars set
- **THEN** they use the system proxy
- **AND** no proxy is hardcoded in code

