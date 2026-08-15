## Why

The `explicit default profile` behavior reversal (commit ccbcfdc: `opsx use` no longer writes the shared `[default]` profile, `opsx kube` no longer merges into `~/.kube/config`, new `opsx default` command) was shipped by editing the canonical specs directly, with no change proposal or delta. A full-repository review (2026-08) flagged this as a traceability violation: the archive history cannot answer why those Requirements were reversed. The reversal also shipped with a single positive scenario for `opsx default`, leaving its credential-freshness, concurrency, failure-path, and shell-wrapper semantics unspecified.

## What Changes

- Documentation-only, retroactive change: **no code or behavior changes**. The implemented behavior already ships.
- Re-record the reversal as proper deltas so archiving restores full traceability.
- Complete `opsx default` semantics with scenarios for cached-credential reuse, latest-wins concurrency, failure paths (unknown alias, expired master, incomplete profile), and wrapper pass-through.
- State the deliberate logout asymmetry: legacy merged contexts in `~/.kube/config` from older opsx versions are neither written nor cleaned.
- **BREAKING** (already released in ccbcfdc, recorded here for the history): `opsx use` no longer writes `[default]`; `opsx kube` no longer merges `~/.kube/config`; `opsx default <alias>` is the explicit opt-in replacement.

## Capabilities

### Modified Capabilities

- `account-switching`: complete the "Shared default profile is not a switch target" requirement with reuse, concurrency, and failure scenarios for `opsx default`.
- `cluster-switching`: complete the "Default kubeconfig is not a switch target" requirement with the legacy-residue non-cleanup asymmetry.
- `shell-integration`: anchor `opsx default` in the wrapper's direct-execution enumeration with a terminal-unchanged scenario.

## Impact

- No affected packages, config schema, or commands — this change only rewrites spec text.
- Archiving replaces three canonical Requirements with strictly richer versions; the cli-foundation subcommand list is intentionally not re-delta'd because the archived `add-interactive-config-management` change already restates it identically.
