## Why
A full-repository spec audit (2026-08) found canonical specs contradicting each other, the code, or the Makefile: account-switching still described a citizen region fallback chain that the required-region schema removed; the "under 2 seconds" switch wording read as a hard deadline although lock acquisition alone may wait up to its bounded timeout; credential-store required an `ErrCitizenExpired` sentinel that no longer exists (and a scenario that both required and allowed removal); the cross-compile scenario referenced no make target; the entra-auth password requirement claimed zeroizable `[]byte` handling that env-var and form-encoding paths cannot satisfy; and two archived capabilities still carried TBD Purpose placeholders. USAGE.md/USAGE.zh-CN.md still documented the pre-ccbcfdc `[default]` overwrite behavior.

## What Changes
Specification and documentation only — no behavior changes. Align wording with implemented behavior: mandatory account region (no fallback), switch latency as an SLO, master-only expiry sentinel with transparent citizen refresh, `make cross` as the cross-compile entry point, honest password-copy trade-offs, real Purpose sections, and current `[default]` semantics in both usage docs.

## Capabilities
### Modified Capabilities
- `account-switching`, `credential-store`, `entra-auth`, `cli-foundation` (wording alignment as above)

## Impact
None on code. Directly updates the two TBD Purpose sections (openspec's own post-archive instruction) and both USAGE files in the same commit.
