## 1. Region policy schema

- [x] 1.1 RED: add table-driven config tests for ordered `regions` decoding, missing/empty/duplicate/whitespace values, required account regions, and account/cluster/auth membership; run the focused tests and confirm failure
- [x] 1.2 GREEN: add the typed region allowlist and minimal validation/resolution changes that satisfy the new config tests
- [x] 1.3 RED: add `use --region` tests proving an allowed override succeeds and a disallowed override fails without emitting assignments; confirm failure
- [x] 1.4 GREEN: enforce allowlist membership for `use --region`, retaining the existing flag and clear allowed-value errors
- [x] 1.5 REFACTOR: centralize region membership/validation helpers without weakening the focused config and use tests

## 2. Loss-minimizing configuration store

- [x] 2.1 RED: add table-driven YAML-node parsing tests for one conventional document and rejection of multiple documents, anchors, aliases, and merge keys; confirm failure
- [x] 2.2 GREEN: implement the minimal YAML document/node loader and unsupported-structure checks
- [x] 2.3 RED: add account/cluster node mutation tests covering append order, target-only deletion, explicit empty mappings, comments, quoting, untouched auth/regions, and full candidate validation; confirm failure
- [x] 2.4 GREEN: implement local account/cluster node add/delete operations and candidate re-encoding/validation
- [x] 2.5 RED: add store transaction tests for reread-under-lock conflicts, concurrent distinct additions, atomic replacement, symlink preservation, permission preservation, and no backup file; confirm failure
- [x] 2.6 GREEN: implement the locked config read-modify-validate-atomic-write transaction using the existing lock and filesystem primitives
- [x] 2.7 REFACTOR: keep typed config loading and mutation storage responsibilities separate while all config-store tests remain green

## 3. Portable interactive prompt foundation

- [x] 3.1 RED: add table-driven prompt tests for TTY rejection, free-text retry, numbered selection, config-order versus alias-order menus, active markers, `[y/N]`, case-insensitive confirmation, invalid retry, EOF, Ctrl-C, and cancellation; confirm failure
- [x] 3.2 GREEN: implement a small injectable prompt/TTY service using standard I/O and existing terminal support, with prompts written to the supplied prompt writer
- [x] 3.3 REFACTOR: consolidate validation and cancellation results without introducing a third-party full-screen prompt dependency

## 4. Interactive account commands

- [x] 4.1 RED: add command tests for `opsx account` help, successful add, optional blank description, invalid alias retry, duplicate alias/account-ID rejection, region selection, default-negative cancellation, and non-TTY failure; confirm failure
- [x] 4.2 GREEN: register `opsx account add` and implement its interactive summary/confirmation flow without overwrite or edit behavior
- [x] 4.3 RED: add account-delete tests for sorted selection, empty success, complete summary, active-context warning, referenced-account diagnostics, explicit empty mapping, cancellation, and retained runtime artifacts; confirm failure
- [x] 4.4 GREEN: implement `opsx account delete` with commit-time reverse-reference checks, warning-only active detection, no cascade, and config-only deletion
- [x] 4.5 REFACTOR: share account rendering/validation helpers while preserving command output and exit semantics

## 5. Interactive cluster commands

- [x] 5.1 RED: add command tests for `opsx cluster` help, successful add, no-account failure, sorted account selection, configured-order region selection, duplicate alias rejection, duplicate real-cluster warning, cancellation, and non-TTY failure; confirm failure
- [x] 5.2 GREEN: register `opsx cluster add` and implement its interactive summary/confirmation flow
- [x] 5.3 RED: add cluster-delete tests for sorted selection, empty success, complete summary, active-context warning, explicit empty mapping, cancellation, and retained runtime artifacts; confirm failure
- [x] 5.4 GREEN: implement `opsx cluster delete` as target-only config deletion with no credential/state/kubeconfig cleanup
- [x] 5.5 REFACTOR: share resource menu, summary, and retained-artifact messaging without creating edit/update paths

## 6. Terminal region switching

- [x] 6.1 RED: add region-command tests for TTY enforcement, configured-order menu, current-region marker, selection with no profile, allowlist-only results, cancellation, and no config/state writes; confirm failure
- [x] 6.2 GREEN: add the top-level `opsx region` interaction and the internal shell-switch region adapter emitting only `AWS_REGION`/`AWS_DEFAULT_REGION` assignments
- [x] 6.3 RED: extend zsh, bash/Git Bash, PowerShell, and cmd wrapper tests for region routing, help bypass, failure propagation, dialect-native assignments, and non-assignment refusal; confirm failure
- [x] 6.4 GREEN: route `region` through every generated wrapper while keeping account/cluster config commands on the direct path
- [x] 6.5 REFACTOR: reuse assignment generation and prompt stderr discipline across dialects without weakening eval guards

## 7. Introspection

- [x] 7.1 RED: add `ls` rendering tests for aligned account/cluster columns, account IDs, required regions, blank-description `-`, referenced account IDs, alias sorting, empty mappings, absence of a regions section, and invalid-config errors; confirm failure
- [x] 7.2 GREEN: replace the current list renderer with the two detailed human-readable tables
- [x] 7.3 RED: add status tests for showing `AWS_REGION` with no active profile and preserving existing active/foreign-profile behavior; confirm failure
- [x] 7.4 GREEN: make terminal region capture independent of `AWS_PROFILE` and render it in the no-active-context state
- [x] 7.5 REFACTOR: keep introspection read-only and avoid introducing a stable machine-output contract

## 8. Samples, migration guidance, and verification

- [x] 8.1 Update `testdata/config.example.yaml` with an ordered `regions` allowlist and explicit region on every account
- [x] 8.2 Update README, English usage, and Chinese usage for schema migration, interactive add/delete flows, dependency blocking, runtime-retention warnings, region switching, shell-wrapper regeneration, and detailed `ls`
- [x] 8.3 Run `gofmt` and `go vet ./...`, fixing only issues caused by this change
- [x] 8.4 Run `go test -v -race -cover ./...` and confirm core business coverage remains at least 80%
- [x] 8.5 Run `golangci-lint run ./...` and resolve every reported issue
- [x] 8.6 Run `go build ./...`, the production binary build, and supported cross-build targets
- [x] 8.7 Run `openspec validate add-interactive-config-management --strict` and confirm the implemented behavior matches every delta-spec scenario

## 9. Verification follow-ups

- [x] 9.1 RED: add cross-shell wrapper regression tests proving export-like command substitution and arbitrary cmd output are rejected before evaluation, and confirm the focused tests fail
- [x] 9.2 GREEN: enforce strict assignment syntax and the supported environment-variable allowlist in zsh, bash, PowerShell, and cmd wrappers
- [x] 9.3 Add regression coverage for invalid `ls` config, missing/invalid mutation config, and commit-time account-reference protection
- [x] 9.4 Update stale region comments to describe required account regions and the current resolution rules
- [x] 9.5 Run formatting, focused/full tests, vet, lint, builds, and strict OpenSpec validation

## 10. Code-review follow-ups (full-repository review)

- [x] 10.1 RED/GREEN: reject cluster aliases that differ only by letter case at validation time (`validateClusters` case-fold uniqueness), preventing silent kubeconfig file sharing on case-insensitive filesystems; add the matching config-delta scenario
- [x] 10.2 RED/GREEN: make interactive prompts context-aware so Ctrl-C (SIGINT, not a literal `\x03` byte on a cooked TTY) cancels a pending prompt instead of hanging; the login password prompt observes the same context, and an empty `OPSX_PASSWORD` no longer counts as set (empty values fall through to the prompt)
- [x] 10.3 Fix the stale "optional" region comment in `testdata/config.example.yaml` (residue from 9.4)
- [x] 10.4 Satisfy the credential-store single-flight scenario across processes: the citizen miss path (reuse re-check, AssumeRole, credentials write, state write) now runs in ONE shared advisory lock window, so two opsx processes switching to the same profile issue at most one AssumeRole and both files commit atomically together
- [x] 10.5 Verify the cmd-wrapper percent-expansion review finding against authoritative cmd semantics: for-variable substitution does NOT re-run percent expansion, so `%` in emitted values is safe; document in the generated wrapper why lines must keep executing via `for /f ... do %%L` and never via direct execution of the temp file
- [x] 10.6 Move the directly imported `aws-sdk-go-v2/credentials` module into the direct require block (`go mod tidy`)
- [x] 10.7 Re-run gofmt, vet, golangci-lint, `go test -race -cover ./...`, production and cross builds, and strict OpenSpec validation
