## Why
Full-repository review (2026-08) found three edge-case defects: a `state.json` containing literal JSON `null` produced a nil map that panicked on the next write; `opsx use --region " us-west-2 "` was trimmed into validity and pure whitespace was silently treated as "not provided", against the spec's whitespace rejection; and STS responses with empty token/key fields or a missing expiry were cached as successful sessions.

## What Changes
- `state.Load` returns an empty map for JSON `null`.
- `--region` rejects whitespace outright instead of trimming.
- A shared `ValidateSTSResult` guard refuses incomplete STS credentials and zero expiries before they are cached (wired into master and citizen assumes).

## Capabilities
None modified — the config and credential-store specs already require this behavior; the code now conforms.

## Impact
internal/state, internal/cli (use), internal/auth. Deferred from this change: concurrent `aws eks update-kubeconfig` runs targeting the same per-(cluster,mode) file still race (needs lock/staging around the external command — separate proposal).
