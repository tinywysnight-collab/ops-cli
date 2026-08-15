## 1. Logout safety
- [x] 1.1 RED: logout preserves a user-maintained `[default]` (long-term keys, no session token) and reports it as preserved
- [x] 1.2 RED: logout preserves a non-STS profile named by a (corrupted/hand-edited) state entry
- [x] 1.3 GREEN: shape-verify every target before deletion; single shared-lock window for plan/verify/delete; creds+state commit via DeleteSharedLockHeld
- [x] 1.4 Keep existing logout tests green (STS-shaped targets still removed)
- [x] 1.5 Write deltas for account-switching and credential-store; run make spec and strict validation
