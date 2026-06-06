# credential-store Specification

## Purpose

Persist AWS credentials and opsx state safely: concurrency-safe, atomic, crash-safe, permission-correct writes that preserve unrelated content, keep credentials and state consistent as one unit, single-flight concurrent switches, handle symlinks, detect expiry with sentinels, and purge on logout — all testable against temp dirs and an injectable clock.

## Requirements

### Requirement: Concurrency-safe credential and state writes
The system SHALL guard every read-modify-write of `~/.aws/credentials` and `~/.config/opsx/state.json` with a `gofrs/flock` advisory exclusive lock so that concurrent writers never corrupt the files.

#### Scenario: Concurrent writes are serialized
- **WHEN** two processes write profiles concurrently through the store
- **THEN** the advisory exclusive lock serializes the writes
- **AND** the credentials file is never corrupted

### Requirement: Atomic, crash-safe writes
The system SHALL write `~/.aws/credentials` and `state.json` atomically AND durably: write to a temporary file in the same directory (mode `0600`), `fsync` that temporary file, `os.Rename` it into place, then `fsync` the parent directory. A crash or signal mid-write can never leave a truncated or partially written file, and a write that returns success is durable across power loss. The advisory lock serializes concurrent writers but does NOT provide atomicity or durability.

The parent-directory sync step SHALL be platform-aware. On macOS and Linux it SHALL open and sync the parent directory after rename. On Windows, where portable directory fsync is not available through Go's standard `os` APIs in the same way, the directory sync step MAY be a no-op; Windows writes MUST still fsync the temporary file before rename and MUST NOT fail solely because directory fsync is unsupported.

#### Scenario: Interrupted write never corrupts the destination
- **WHEN** a credentials or state write is interrupted before completion
- **THEN** the destination file is either the complete old content or the complete new content
- **AND** it is never a truncated or partial file

#### Scenario: Write is durable after rename
- **WHEN** an atomic credentials or state write returns success
- **THEN** the temporary file's contents were fsync'd before the rename
- **AND** on macOS/Linux the parent directory entry was fsync'd after the rename
- **AND** on Windows the write does not fail solely because portable directory fsync is unsupported

### Requirement: Preserve unrelated credentials-file content
The system SHALL preserve comments, blank lines, key casing, and unrelated profiles/keys when upserting a profile's STS keys into `~/.aws/credentials`. Only the three STS keys (`aws_access_key_id`, `aws_secret_access_key`, `aws_session_token`) of the target profile may be rewritten; all other content MUST round-trip unchanged.

When the target profile name appears more than once in the file, the upsert MUST NOT leave stale managed-key values behind in any duplicate section. The implementation MUST either consolidate all duplicate sections of the target profile or fail with a clear error; it MUST NOT update only one occurrence while leaving another occurrence's old secret/session token in place.

#### Scenario: Comments survive a profile write
- **WHEN** `~/.aws/credentials` contains comment lines and a hand-formatted unrelated profile, and opsx upserts a different profile
- **THEN** the comments, blank lines, and unrelated profile are preserved verbatim
- **AND** only the target profile's three STS keys are written

#### Scenario: Duplicate target profile leaves no stale secret
- **WHEN** the target profile header appears twice and opsx upserts new STS keys
- **THEN** after the write no duplicate section of the target profile still contains the old `aws_secret_access_key` or `aws_session_token`
- **AND** a subsequent read returns exactly the newly written values

### Requirement: Credential and state written as one atomic unit
The system SHALL write a profile's credentials (`~/.aws/credentials`) and its state entry (`state.json`) so that the two never persistently disagree. A crash between the two writes MUST NOT leave a usable credential whose expiry/context is unknown. The implementation MUST either perform both writes under a single held lock with both files flushed before releasing, or order and reconcile the writes so that a half-applied switch is self-healing and not silently trusted.

#### Scenario: No credential without recorded expiry
- **WHEN** a switch is interrupted after the credentials file is written but before the state entry is written
- **THEN** the next command does not treat the orphaned credential as a valid cached session
- **AND** it re-derives or completes the state so credential and expiry agree

### Requirement: Single-flight citizen switch
The system SHALL avoid redundant `AssumeRole` calls when multiple terminals run `opsx use` or `opsx kube` for the same `[<alias>.<mode>]` profile concurrently. The reuse check and the assume-and-write MUST be coordinated, for example by re-checking under the lock, so the common concurrent case results in at most one `AssumeRole` for that profile within its validity window.

#### Scenario: Concurrent switches do not double-assume
- **WHEN** two terminals run `opsx use dev` simultaneously with no valid cached citizen profile
- **THEN** at most one `AssumeRole` is issued for `[dev.<mode>]`
- **AND** both terminals end up using the same cached citizen credentials

### Requirement: Symlink-safe credentials write
The system SHALL handle the case where `~/.aws/credentials` or the configured credentials path is a symbolic link. The atomic temp+rename write MUST NOT silently replace the symlink with a regular file, breaking a user's intentional indirection such as dotfiles or a vault mount. The system MUST resolve the link target and write through it, or detect the symlink and fail with a clear, actionable error.

Symlink resolution MUST either follow the full chain (so a symlink that points at another symlink lands on the final real target) or explicitly document and test that only a single hop is supported. Resolving the target MUST NOT cause opsx to create directory trees at arbitrary locations outside the configured home/AWS paths without intent.

#### Scenario: Existing symlink is not clobbered
- **WHEN** the credentials path is a symlink to another location and opsx writes a profile
- **THEN** the symlink is preserved and the write lands on the link target, OR the command fails with a clear message
- **AND** the symlink is never silently replaced by a regular file

#### Scenario: Multi-hop symlink resolves to the final target
- **WHEN** the credentials path is a symlink whose target is itself a symlink to a real file, and opsx writes a profile
- **THEN** the write lands on the final real target (full-chain resolution), OR the command fails with a clear message if only single-hop resolution is supported
- **AND** no intermediate link is replaced by a regular file

### Requirement: Omit empty credential keys
The system SHALL omit any STS key whose value is empty rather than writing an empty assignment (e.g. it MUST NOT write `aws_session_token = ` for sessionless master credentials). A profile SHALL be treated as "present" only when both the access key ID AND secret access key are non-empty.

For opsx-managed STS profiles used by `login`, `use`, and `kube`, a cached session MUST NOT be trusted unless `aws_access_key_id`, `aws_secret_access_key`, and `aws_session_token` are all present and non-empty. STS assumed-role credentials require a session token; a no-token or hand-edited profile MUST be treated as missing/stale and re-derived rather than reused.

#### Scenario: Sessionless credentials write no empty token
- **WHEN** credentials without a session token are written
- **THEN** no `aws_session_token` line is emitted for that profile

#### Scenario: Partial credentials are not "present"
- **WHEN** a profile has an access key ID but no secret access key
- **THEN** it is reported as not present

#### Scenario: Opsx-managed STS cache requires a session token
- **WHEN** an opsx-managed master or citizen profile has access key and secret but no `aws_session_token`
- **THEN** `opsx login`/`opsx use`/`opsx kube` do not treat it as a valid cached STS session
- **AND** the command re-authenticates, re-assumes, or reports the master cache as missing/expired as appropriate

### Requirement: Bounded lock acquisition with context
The system SHALL acquire the advisory lock with a bounded timeout derived from the caller's `context`, never blocking indefinitely. On contention beyond the deadline it SHALL return a clear error (naming the lock) and exit non-zero, honoring `ctx` cancellation (Ctrl-C).

#### Scenario: Lock contention fails clearly instead of hanging
- **WHEN** another process holds the lock past the deadline
- **THEN** the command returns a clear error identifying the lock and exits non-zero
- **AND** Ctrl-C cancels the wait promptly

### Requirement: Purge cached credentials and state
The system SHALL provide `opsx logout [--opr] [--all]` to remove cached credentials and their state entries without hand-editing files. By default it SHALL purge the master profile for the current or selected mode and that mode's citizen profiles; `--all` SHALL purge every opsx-managed profile and state entry for both modes. The command MUST run under the shared advisory lock, preserve unrelated profiles and comments in `~/.aws/credentials`, and leave a file that is valid for the AWS CLI.

#### Scenario: Logout clears the master cache for a mode
- **WHEN** `opsx logout` runs for a mode whose master credentials are cached
- **THEN** the `[master_<mode>]` profile and its state entry are removed
- **AND** unrelated profiles and comments in `~/.aws/credentials` are preserved
- **AND** a subsequent `opsx use` for that mode reports the master credentials as missing or expired

#### Scenario: Logout --all clears everything opsx manages
- **WHEN** `opsx logout --all` runs
- **THEN** all opsx-managed master and citizen profiles and their state entries are removed for both modes
- **AND** non-opsx profiles remain untouched

### Requirement: Secure file permissions
The system SHALL write `~/.aws/credentials` and `state.json` with mode `0600` and create `~/.config/opsx/` directories with mode `0700`.

#### Scenario: Permissions on write
- **WHEN** a credentials or state write completes
- **THEN** `~/.aws/credentials` and `state.json` are mode `0600`
- **AND** `~/.config/opsx/` directories are mode `0700`

### Requirement: State store records context and expiry
The system SHALL maintain `~/.config/opsx/state.json` keyed by profile name, recording `{expiry, account, mode, cluster, updated_at}` whenever a profile is written.

#### Scenario: State updated on profile write
- **WHEN** a profile is written and the store updates state
- **THEN** `state.json` records `{expiry, account, mode, cluster, updated_at}` keyed by profile name

### Requirement: Expiry detection with sentinels
The system SHALL detect expired credentials by comparing recorded expiry against the current time and return sentinel errors `ErrMasterExpired` / `ErrCitizenExpired` that are detectable via `errors.Is`. On reaching the top-level handler, the system SHALL print a clear re-login message and exit non-zero.

#### Scenario: Past expiry returns a sentinel
- **WHEN** a profile whose recorded expiry is in the past is checked
- **THEN** `ErrMasterExpired` or `ErrCitizenExpired` is returned
- **AND** it is detectable via `errors.Is`

#### Scenario: Expiry uses a skew buffer
- **WHEN** a profile's recorded expiry is within the safety buffer (e.g. 2 minutes) of now
- **THEN** it is treated as expired so no switch assumes with about-to-lapse credentials

#### Scenario: ErrCitizenExpired is reachable or removed
- **WHEN** a still-cached citizen profile is consulted and found stale
- **THEN** `ErrCitizenExpired` is returned by a real code path
- **AND** if no such path exists, the unused sentinel is removed rather than left dead

#### Scenario: Expiry sentinel formatted for the user
- **WHEN** an expiry sentinel reaches the top-level handler
- **THEN** a clear `master credentials expired — run: opsx login [--opr]` message is printed to stderr
- **AND** the process exits non-zero

### Requirement: Tests isolate filesystem and clock
The credentials/state layer SHALL be testable using temporary directories and an injectable clock, with no dependency on the real home directory or wall-clock time.

#### Scenario: Test suite uses temp dirs and injectable clock
- **WHEN** the credentials/state test suite runs
- **THEN** it uses temporary directories and an injectable clock
- **AND** it does not touch the real home directory or rely on real wall-clock time

### Requirement: Documented consistency and locking limitations
The system SHALL document, in `README.md`, the operational limits behind its isolation guarantee: advisory `flock` is best-effort on network filesystems (NFS/SMB) and the multi-terminal isolation guarantee assumes a local home directory; reads are lock-free and per-file consistent; cross-file credentials/state consistency is bounded by the atomic-unit requirement above; and the real Entra/MFA path is company-environment dependent even though the assertion escape hatch remains supported.

#### Scenario: Limitations are stated for the user
- **WHEN** a user reads `README.md`
- **THEN** the NFS/network-filesystem locking caveat, the consistency model, the Entra live-verification boundary, and the assertion escape hatch are documented
