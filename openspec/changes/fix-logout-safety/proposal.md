## Why

Full-repository review (2026-08) found `opsx logout` unsafe twice over: it deleted any profile named by a state entry (a corrupted or hand-edited state.json could name a user's own profile), unconditionally deleted the shared `[default]` even when it held the user's long-term keys, and ran its state read, credential deletion, and state deletion in three separate lock windows — a concurrent login/use could leave live STS secrets behind after logout reported success.

## What Changes

- Execute logout as ONE shared-lock transaction: reload state, plan, verify, delete credentials and state in the same window.
- Before deleting any target, verify it holds a complete opsx STS session (access key, secret key, session token). A present-but-non-STS profile (e.g. hand-maintained `[default]` with long-term keys) is preserved and reported, never deleted.
- `[default]` compatibility cleanup now applies only to opsx-shaped `[default]` sections.

## Capabilities

### Modified Capabilities
- `account-switching`: `[default]` compatibility cleanup becomes shape-gated.
- `credential-store`: logout gains shape verification and same-window commit semantics.

## Impact

- Affected packages: internal/cli (logout), internal/creds, internal/state (SharedLockHeld delete variants).
- A `[default]` written by very old opsx versions is always STS-shaped, so legitimate compatibility cleanup is unaffected.
