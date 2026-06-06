# Implementation Tasks

Ordered by the architecture's implementation sequence (Stories 1.1 → 1.8). Each domain change follows TDD red-green-refactor: write the failing test first, then the minimal implementation, then refactor. Capability tags reference `specs/<capability>/spec.md`.

Workflow note: review findings discovered before `init-ops-cli` is accepted/archived are folded back into this change's specs, design, and tasks. Do not create a separate "hardening" change for defects in the current change unless the user explicitly asks to split scope.

## 1. Project init + Cobra skeleton (cli-foundation)

- [x] 1.1 `go mod init github.com/<org>/opsx`; add cobra v1.10.x, aws-sdk-go-v2 (config, credentials, service/sts), gopkg.in/yaml.v3, gofrs/flock, golang.org/x/term; run `go mod tidy`
- [x] 1.2 Create the package layout: `cmd/opsx/main.go` + `internal/{cli,config,auth,creds,state,kube,shell,paths}` (with placeholder files)
- [x] 1.3 Add `Makefile` with `build` (`CGO_ENABLED=0 go build -o bin/opsx ./cmd/opsx`), darwin/linux cross-compile, and `test` targets; add `.gitignore`
- [x] 1.4 Implement `internal/paths/paths.go` helpers (`~/.config/opsx`, `~/.aws/credentials`, `kube/` paths)
- [x] 1.5 Wire Cobra root in `internal/cli/root.go` with persistent `--opr` flag and placeholder subcommands (`login`, `use`, `kube`, `mode`, `status`, `ls`, `init`, `shell-switch`); add `--version`
- [x] 1.6 Implement single top-level error handler in `cmd/opsx/main.go` (print to stderr, non-zero exit; no `os.Exit`/`panic` in `RunE`)
- [x] 1.7 Verify: `make build` produces a static binary, `opsx` prints help with all subcommands, `opsx --version` works, and an errored command exits non-zero via the top handler

## 2. Config load/validate + ARN composition (config)

- [x] 2.1 Write failing tests for `config.Load`/`Validate`: valid decode, cluster→unknown-account error, missing-file error
- [x] 2.2 Implement `internal/config/config.go`: typed structs for `accounts`, `clusters`, `auth`; `Load` (yaml.v3) + `Validate` (referential integrity, clear errors with offending names)
- [x] 2.3 Write failing tests for ARN composition (master + citizen) and config-driven role-name changes
- [x] 2.4 Implement `internal/config/arn.go`: master ARN `arn:aws:iam::{master_account_id}:role/{master_roles[mode]}`, citizen ARN `arn:aws:iam::{accounts[alias].account_id}:role/{citizen_roles[mode]}` (no hardcoding)
- [x] 2.5 Add `testdata/config.example.yaml` sample/fixture

## 3. Credentials/state foundation (credential-store)

- [x] 3.1 Write failing tests using temp dirs + injectable clock: concurrent-write serialization, file permissions, state record shape, expiry sentinels via `errors.Is`
- [x] 3.2 Implement `internal/creds/store.go`: ini read-modify-write of `~/.aws/credentials` under a `gofrs/flock` exclusive lock; files `0600`, dirs `0700`
- [x] 3.3 Implement `internal/state/state.go`: `state.json` keyed by profile name with `{expiry, account, mode, cluster, updated_at}`, written under the shared flock
- [x] 3.4 Implement `internal/creds/expiry.go`: expiry comparison + `ErrMasterExpired`/`ErrCitizenExpired` sentinels
- [x] 3.5 Wire the top-level handler to format expiry sentinels as `… expired — run: opsx login [--opr]` on stderr, exit non-zero
- [x] 3.6 Verify: cross-process write test shows no corruption; permission + expiry tests pass

## 4. Auth seam + Entra provider (entra-auth)

- [x] 4.1 Define `internal/auth/provider.go`: `SAMLProvider.FetchAssertion(ctx, role)` + `MasterRole` (Admin|Opr); add a fake provider for tests
- [x] 4.2 Write failing tests (fake provider): login routes through the seam; master cache to `[master_admin]`/`[master_awsopr]` with 1h expiry; both roles coexist
- [x] 4.3 Implement `internal/auth/master.go`: `AssumeRoleWithSAML` (aws-sdk-go-v2, regional endpoint, session name `opsx-<os-user>`) → master STS → cache via `creds`/`state`
- [x] 4.4 Implement `internal/auth/prompt.go`: no-echo password (`x/term`) with `OPSX_PASSWORD` fallback; `[]byte`, zeroized; never persisted/logged
- [x] 4.5 Implement `internal/auth/entra.go` `EntraSAMLProvider` (HTTP via default transport/system proxy; Entra bootstrap, ADFS form auth, Microsoft relay, context-aware MFA polling to stderr, SAMLResponse extraction; env/file assertion escape hatch retained)
- [x] 4.6 Implement `internal/cli/login.go` (`opsx login [--opr]`) wiring the seam + master cache

## 5. use + mode + shell-switch + init zsh (account-switching, shell-integration)

- [x] 5.1 Write failing tests: `use` assumes citizen role → writes `[alias.mode]` + state (no MFA); expired master creds → re-login error
- [x] 5.2 Implement `internal/creds/citizen.go`: `AssumeRole` → citizen STS → write `[<alias>.<mode>]` profile + update state
- [x] 5.3 Implement `internal/cli/use.go` (`opsx use <alias>`) using current mode (env/flag), under 2s
- [x] 5.4 Write failing tests for `shell/emit.go`: stdout contains ONLY `export KEY=value`; all else to stderr (AWS_PROFILE, KUBECONFIG, OPSX_MODE)
- [x] 5.5 Implement `internal/shell/emit.go` export emitter (strict stdout discipline)
- [x] 5.6 Implement `internal/cli/mode.go` (`opsx mode admin|opr` → emit `OPSX_MODE`; `--opr` override; never persisted) and `internal/cli/shellswitch.go`
- [x] 5.7 Implement `internal/shell/initzsh.go` + `internal/cli/init.go`: emit `opsx() { eval "$(command opsx shell-switch "$@")"; }`
- [x] 5.8 Verify: deliberate cross-terminal test — two terminals, different account/mode, no `AWS_PROFILE` collision

## 6. kube + per-terminal KUBECONFIG (cluster-switching)

- [x] 6.1 Write failing tests: `kube` auto-ensures creds, invokes `aws eks update-kubeconfig` with the alias's region + real name into `~/.config/opsx/kube/<cluster>.<mode>.yaml`; missing-tool / expired-creds errors
- [x] 6.2 Implement `internal/kube/kube.go`: shell out to `aws eks update-kubeconfig --kubeconfig <path>` (os/exec); prerequisite checks for `aws`/`kubectl`
- [x] 6.3 Implement `internal/cli/kube.go` (`opsx kube <alias>`) → ensure creds → kube → emit `export KUBECONFIG=<path>` via `shell/emit`
- [x] 6.4 Verify: two terminals on different clusters export distinct `KUBECONFIG` paths with no context collision; helm follows automatically

## 7. status + ls (introspection)

- [x] 7.1 Write failing tests for `ls`: lists accounts (with description) and clusters (account + region); friendly "none configured" for empty sections; invalid-config error to stderr/non-zero
- [x] 7.2 Implement `internal/cli/ls.go` (`opsx ls`) reading `config`
- [x] 7.3 Write failing tests for `status`: active context from env + `state.json`; "nothing active" hint; expired-shown-with-hint; read-only (no mutation)
- [x] 7.4 Implement `internal/cli/status.go` (`opsx status`) reading `state` + `creds/expiry` (read-only)

## 8. Final verification

- [x] 8.1 Run `gofmt`, `golangci-lint run ./...`, `go vet ./...` — clean _(golangci-lint 2.12.2 installed; `.golangci.yml` added; all three pass with 0 issues)_
- [x] 8.2 Run `go test -v -race -cover ./...` — all pass; core business packages ≥ 80% coverage _(config 95.7%, state 82.1%, creds 83.5%, shell 100%)_
- [x] 8.3 Run `make build` (and darwin/linux cross-compile) — single static binary, `CGO_ENABLED=0`
- [x] 8.4 Confirm no hardcoded account IDs / role names / proxy; password never persisted/logged; `shell-switch` stdout is export-lines-only
- [x] 8.5 Update `README.md` with install, `opsx init zsh`, config example, and daily-usage commands

## 9. Review remediation — adversarial review 2026-05-31

Findings from the adversarial review of `feat/init-ops-cli`. Each item names the defect,
its location, the required fix, and the spec capability that now codifies it. Re-apply the
TDD red-green-refactor cycle: add the failing test that reproduces the defect first.

### Blocking — happy path is broken end-to-end

- [x] 9.1 **(R1, shell-integration) zsh wrapper bricks every non-switching command.** `zshFunction` (`internal/shell/initzsh.go:25`) prefixes *every* `opsx` call with `shell-switch`, but `shell-switch` only registers `use`/`kube`/`mode` (`internal/cli/shellswitch.go:19-23`). After `opsx init zsh >> ~/.zshrc`, `opsx login`/`status`/`ls`/`init` become `opsx shell-switch <cmd>` → "unknown command" + no-op `eval`. **Fix:** the wrapper MUST pass switching subcommands through `shell-switch` and run all other subcommands directly (e.g. case-match `use|kube|mode`, else `command opsx "$@"`). Add a test asserting the generated function dispatches non-switch commands directly.
- [x] 9.2 **(R2, cluster-switching) `opsx kube` produces a kubeconfig that cannot authenticate.** `update-kubeconfig` is run without `--profile` (`internal/kube/kube.go:47-52`) and the shell-switch path exports only `KUBECONFIG` (`internal/cli/shellswitch.go:61`), so `kubectl`'s `aws eks get-token` has no profile at runtime. **Fix:** pass `--profile <alias.mode>` to `update-kubeconfig` so the generated kubeconfig's `exec` block carries the profile (preferred), and/or also export `AWS_PROFILE`. Add a test asserting the profile reaches the generated kubeconfig.
- [x] 9.3 **(R3, shell-integration) shell-injection via unquoted, unvalidated export values.** `shell.Export` (`internal/shell/emit.go:14`) emits values verbatim; profile names derive from user-controlled `config.yaml` aliases (`internal/config/naming.go:22`) with no shell-safety validation anywhere. A malicious/typo alias (`foo;rm -rf ~`) is `eval`'d in the live shell. **Fix:** validate aliases/mode at config-load against a strict charset (`^[A-Za-z0-9._-]+$`) and reject anything else; the export emitter MUST refuse values containing shell metacharacters. Add tests for both gates.

### High — features that silently do not work

- [x] 9.4 **(R4, account-switching / credential-store) `ErrCitizenExpired` is dead and citizen expiry is never checked.** The sentinel (`internal/creds/expiry.go:14`) is handled in `cmd/opsx/main.go:29` but never returned; `CitizenService.Use` (`internal/creds/citizen.go`) blindly re-assumes. **Fix:** decide and implement intended behavior — either return `ErrCitizenExpired` where a still-cached citizen profile is consulted and stale, or remove the dead sentinel. Wire a real code path + test.
- [x] 9.5 **(R5, cluster-switching / introspection) `Entry.Cluster` is written only by tests.** `status.go:44-45` renders `Cluster:` but no production path sets `Entry.Cluster`; `switchCluster` (`internal/cli/kube.go`) never records it. **Fix:** on `opsx kube`, persist a state entry recording the cluster for the active profile so `opsx status` can show it. Add a test through the real `kube` flow (not a hand-built entry).
- [x] 9.6 **(R6, credential-store) INI handler destroys user data it claims to preserve.** `internal/creds/ini.go:9-12` advertises preservation but `parseINI` drops comments/blank lines (`:31`) and `render` reformats `key=value`→`key = value` (`:85`). User comments in `~/.aws/credentials` are silently lost. **Fix:** preserve comments, blank lines, and unrelated formatting; only the three STS keys of the target profile may be rewritten. Add a round-trip test that asserts comments survive.
- [x] 9.7 **(R7, credential-store) writes are not atomic — the lock is a false guarantee.** `store.go:97-103` and `state.go:89-103` `os.WriteFile` over the live file; a crash mid-write truncates `~/.aws/credentials` despite the flock. **Fix:** write to a temp file in the same dir (`0600`) then `os.Rename` into place. Add a test asserting the destination is never partially written.
- [x] 9.8 **(R8, credential-store) the lock can hang the CLI forever.** `lock.With` (`internal/lock/lock.go:21`) calls blocking `fl.Lock()` with no timeout/`context`, violating the project's "context first" standard. **Fix:** accept `ctx` and use `flock.TryLockContext` with a bounded deadline; on contention emit a clear stderr message and exit non-zero. Thread `ctx` from callers.

### Medium — correctness and robustness

- [x] 9.9 **(R9, credential-store) empty `aws_session_token` written; `nonEmpty` uses OR.** `Store.Write` always emits all three keys (`store.go:47-51`), producing `aws_session_token = ` for sessionless creds; `Credentials.nonEmpty` (`store.go:82-83`) treats access-key-only as present. **Fix:** omit empty keys on write; require access key **AND** secret for "present". Add tests.
- [x] 9.10 **(R10, entra-auth) STS `RoleSessionName` is unsanitized.** `SessionName()` (`internal/creds/citizen.go:87-93`) interpolates the OS username verbatim; domain logins (`DOMAIN\user`) or names with spaces violate STS regex `[\w+=,.@-]{2,64}` → opaque `AssumeRole` failure. **Fix:** sanitize to the allowed charset, truncate to 64, and guarantee a valid fallback. Add a table test of hostile usernames.
- [x] 9.11 **(R11, credential-store) `IsExpired` has no skew buffer.** `internal/creds/expiry.go:19-21` treats creds valid up to the exact expiry instant. **Fix:** treat creds expired N minutes early (configurable constant, e.g. 2m) so a switch never assumes with about-to-lapse master creds. Update tests.
- [x] 9.12 **(R12, config) `Validate` checks almost nothing.** `internal/config/config.go:92-102` only resolves `cluster.account`. **Fix:** validate non-empty `account_id` per account, non-empty cluster `region`/`name`, presence of `master_account_id`/`saml_provider_arn`/required `auth` fields, mode-keyed role maps, and reject duplicate account IDs — failing once, up front, with offending names. Add tests per rule.
- [x] 9.13 **(R13, account-switching / cluster-switching) every switch hits STS — no credential reuse.** `CitizenService.Use` and `switchCluster`→`switchAccount` re-`AssumeRole` and rewrite the profile on every call (`internal/creds/citizen.go:59-66`, `internal/cli/kube.go:27`). **Fix:** reuse an existing unexpired citizen profile (honoring R11's skew) and only assume when missing/stale. Add a test asserting no second assume within the validity window.

### Low — gaps and polish

- [x] 9.14 **(R14, entra-auth) the headline auth flow was initially an unimplemented stub.** Interim fix: keep the seam, document the limitation, and add an integration test driving `login`→`use`→`kube`→`status` through a fake provider and the real command wiring. Superseded by the owner-implemented Entra+ADFS+MFA flow in `internal/auth/entra.go`; the fake-provider integration test remains valuable because it avoids live Entra/AWS dependencies.
- [x] 9.15 **(R15, account-switching) `--opr` precedence is asymmetric.** `resolveMode` (`internal/cli/mode.go:20-28`) lets `--opr` override `OPSX_MODE` but offers no flag to force admin when the env says `opr`. **Fix:** add a symmetric override (e.g. `--admin` or `--mode admin|opr`) and define precedence explicitly in the spec.

## 10. Review remediation — consolidation after second adversarial review

These items were previously captured in an accidental follow-up change. They are now part of `init-ops-cli`; implement them here and keep this as the single active OpenSpec change.

### P0 — Security / data integrity

- [x] 10.1 **(F4, credential-store) Durable atomic writes.** Test and implement `fsutil.AtomicWrite` so it fsyncs the temp file before close/rename and fsyncs the parent directory after rename. This is already implemented in the current worktree; keep the tests with the change.
- [x] 10.2 **(F5, credential-store) Credential+state atomic unit.** Add a failing test simulating interruption after credentials write but before state write; then ensure the next command does not trust an orphaned credential without recorded expiry/context.
- [x] 10.3 **(F7, credential-store) Duplicate target profile handling.** Add a table test where the target profile appears twice; then consolidate duplicates or fail clearly so stale `aws_secret_access_key` / `aws_session_token` cannot survive in a second section.
- [x] 10.4 **(F8, credential-store) Symlink-safe credentials write.** Test and implement writing through an existing symlinked credentials path, or fail clearly without replacing the symlink. This is already implemented in the current worktree; keep the tests with the change.

### P1 — Correctness / cancellation / concurrency

- [x] 10.5 **(F3, cli-foundation) Signal-cancellable root context.** Add a failing test or seam proving `cmd.Context()` is canceled on SIGINT/SIGTERM; then wire `signal.NotifyContext` and `ExecuteContext` in `cmd/opsx/main.go`.
- [x] 10.6 **(F6, credential-store) Single-flight citizen switch.** Add a race/concurrency test with two simultaneous `CitizenService.Use` calls for the same profile; then coordinate reuse/assume/write so at most one `AssumeRole` fires.
- [x] 10.6a **(account-switching) Validate alias before cached reuse.** Add a failing test where a stale local `[alias.mode]` profile exists but the alias was removed from config; then ensure `CitizenService.Use` rejects it before considering reuse.
- [x] 10.6b **(credential-store) Opsx-managed STS cache requires session token.** Add tests for master and citizen profiles with access key + secret but no `aws_session_token`; then ensure `login`/`use`/`kube` do not trust those as valid cached STS sessions.
- [x] 10.7 **(F9, config) Strict config format validation.** Reject non-12-digit account IDs for accounts and `auth.master_account_id`, and reject blank/whitespace regions with name-bearing validation errors.
- [x] 10.8 **(F11, account-switching) Mode flag conflict detection.** Reject disagreeing selectors such as `--opr --mode admin`; accept redundant agreeing selectors such as `--opr --mode opr`.
- [x] 10.9 **(F12, cluster-switching) Cluster recorded even on credential reuse.** Ensure `opsx kube` upserts the cluster into state even when valid citizen credentials are reused and no fresh credential/state entry was written.
- [x] 10.9a **(cluster-switching) Cluster annotation must not overwrite concurrent state refresh.** Add a failing test where `recordCluster` races with a refreshed profile state; then make the cluster update preserve the latest expiry/account/mode values under the state lock.
- [x] 10.10 **(F10, entra-auth) Safe session-name truncation invariant.** Add tests proving over-long and non-ASCII usernames never produce invalid UTF-8 or a name outside `[\w+=,.@-]{2,64}`; keep truncation rune-safe or explicitly pin the ASCII-before-truncation invariant.
- [x] 10.11 **(F13, cluster-switching) Collision-free kubeconfig naming.** Replace ambiguous `<cluster>.<mode>.yaml` naming with a collision-free encoding confined to the kube directory, and test aliases containing `.`.
- [x] 10.12 **(shell-integration) zsh wrapper handles leading global flags and failure.** Update the generated wrapper so `opsx --mode opr use dev` and `opsx --opr kube dev-syd` still route through `shell-switch`, and failed `shell-switch` returns non-zero instead of evaluating empty output successfully.
- [x] 10.12a **(cli-foundation) `make lint` fails on gofmt drift.** Change the lint target so `gofmt -l` output causes a non-zero exit; add or document a verification check for an intentionally unformatted file if practical.

### P2 — Completeness / UX

- [x] 10.13 **(F2, credential-store) `opsx logout`.** Add `opsx logout [--opr] [--all]` to purge opsx-managed cached credentials/state while preserving unrelated profiles/comments.
- [x] 10.14 **(F14, introspection) Foreign `AWS_PROFILE` status.** Make `opsx status` explicitly distinguish a non-opsx-managed profile from an opsx-managed profile with recorded state.
- [x] 10.15 **(entra-auth) Production Entra provider decision.** Resolved: `EntraSAMLProvider.FetchAssertion` is implemented behind the seam, with `OPSX_SAML_ASSERTION` / `_FILE` retained as accepted escape hatches and README/PRD updated to describe the live-verification boundary.
- [x] 10.16 **(repo hygiene) Track project OpenSpec/AI assets intentionally.** Ensure `.claude` OpenSpec-related files are not ignored by the repository policy unless an internal tool's own ignore file deliberately excludes its private local state.

### P3 — Documentation / operational limits

- [x] 10.17 **(F16/F17, credential-store) Document consistency and locking limits.** README must state that reads are lock-free/per-file consistent, cross-file consistency follows the atomic-unit rule, and advisory locking assumes a local home directory rather than NFS/SMB semantics.

## 11. Final verification after review remediation

- [x] 11.1 Run `openspec validate init-ops-cli` and confirm only `init-ops-cli` remains as the active change for this work.
- [x] 11.2 Run `go test -race -cover ./...`; core business packages remain at or above the agreed coverage target.
- [x] 11.3 Run `golangci-lint run ./...` and `go vet ./...`.
- [x] 11.4 Run `make build` and any required cross-compile target.
- [x] 11.5 Re-read `README.md`, `design.md`, and PRD/architecture artifacts to confirm they describe the same accepted scope and known limitations.

## 12. Review remediation — third adversarial review 2026-05-31

Findings from a third adversarial review of `feat/init-ops-cli`, after sections 9–11 were
implemented. Triaged with the product owner: items 12.1–12.7 are actionable; the decisions
under "Accepted — no action" are deliberate and MUST NOT be re-flagged as defects.
Re-apply TDD red-green-refactor: add the failing test that reproduces the defect first.

### P1 — Correctness (user-visible)

- [x] 12.1 **(cluster-switching / introspection) `recordCluster` can create a state entry that `status` reports as EXPIRED for valid credentials.** When the profile has no existing state entry, `recordCluster` (`internal/cli/kube.go:50-63`) sets `Account`/`Mode`/`Cluster` but leaves `Expiry` and `UpdatedAt` at their zero values. A zero `Expiry` makes `creds.IsExpired` return true, so `opsx status` shows "EXPIRED" even though the just-ensured citizen credentials are valid. `TestRecordClusterCreatesMissingEntry` (`internal/cli/integration_test.go`) asserts this half-populated entry is created and never checks the timestamps, codifying the gap. **Fix:** when `opsx kube` upserts a previously-absent state entry, it MUST carry the real credential expiry (derived from the just-ensured citizen credentials) and set `updated_at`, so the entry never misrepresents valid credentials as expired. Add a test through the real `kube`→`status` flow asserting status does not show EXPIRED when creds are valid.

### P2 — Robustness / dead complexity

- [x] 12.2 **(credential-store) `resolveSymlink` follows only one link level and can create directory trees at an unexpected target.** `fsutil.resolveSymlink` (`internal/fsutil/atomic.go`) uses a single `os.Readlink` rather than resolving the full chain, so a symlink-to-a-symlink is not fully followed; it then `MkdirAll(dir, 0o700)` on the *target's* directory, so a credentials symlink pointing into an arbitrary location causes opsx to create directory trees there. There is also a TOCTOU window between `Lstat`/`Readlink` and the rename. **Fix:** resolve the full symlink chain (e.g. `filepath.EvalSymlinks` on the existing portion) or explicitly document and test the single-level contract; do not create parent directories outside the configured home/AWS locations without intent. Add a test for a two-hop symlink and for a link whose target dir does not yet exist.
- [x] 12.3 **(cluster-switching) `encodeKubeAlias` percent-encoding is now largely dead defense.** With mode moved to a subdirectory (`kube/<mode>/<alias>.yaml`) and `mode` normalized to `admin`/`opr` via `NormalizeMode` (`internal/paths/paths.go`), the `<cluster>.<mode>` filename collision the encoding was built to prevent is structurally impossible, and `aliasPattern` already forbids `/` and `\` — leaving only `.` and `%` as reachable inputs. **Fix:** either simplify the encoding to match the threat that actually remains (and update D4 + the "Unambiguous kubeconfig file naming" spec rationale accordingly), or keep it explicitly as belt-and-suspenders with a comment that records why it is retained. Keep the existing collision test green either way.
- [x] 12.4 **(account-switching) `--opr=false` is an untested, silently-ambiguous edge of `resolveModeSelection`.** `flagChanged` returns true for an explicitly-set `--opr=false`, giving `oprSet=true, oprFlag=false`; the conflict guard `oprSet && oprFlag && …` then does nothing, so `--mode admin --opr=false` resolves to admin with no complaint and no test pins the behavior (`internal/cli/mode.go`). **Fix:** define and test the behavior of an explicitly-negated `--opr` (treat as "opr not selected") and add table cases to `mode_test.go` covering `--opr=false` alone and combined with `--mode`.
- [x] 12.5 **(credential-store) Missing master session token is reported as `ErrMasterExpired` — a misdiagnosis.** `validMaster` (`internal/creds/citizen.go`) wraps "missing a session token" with `ErrMasterExpired`, and `TestUseFailsWhenMasterHasNoSessionToken` cements it. The re-login remedy is correct, but a corrupt/incomplete credential is not an *expired* one, so any caller distinguishing the two via `errors.Is` is misled. **Fix (optional / low):** either introduce a distinct sentinel (e.g. `ErrMasterIncomplete`) that still maps to the re-login message, or add a code comment documenting that incomplete master creds are intentionally surfaced as expired. No behavior change required for end users.

### P3 — Tests / documentation

- [x] 12.6 **(credential-store, docs) Document the scope of the in-process citizen single-flight lock.** `citizenProfileLocks` (`internal/creds/citizen.go`) single-flights `AssumeRole` only within one process; cross-process safety still relies entirely on the `flock` inside `Store.Write`. **Fix:** add a short note (code comment and/or README/design) stating that `TestConcurrentUseSingleFlightsAssume` proves in-process coalescing, not cross-process, and that the sync.Map is intentionally unbounded for a short-lived CLI.
- [x] 12.7 **(tests) Strengthen two weak assertions.** (a) `TestSessionNameFromTruncatesSafely` (`internal/creds/citizen_test.go`) pads with 200 NUL bytes; add a case with mixed shell/STS-illegal metacharacters so the test exercises real sanitization, not just NUL stripping. (b) `TestEntraProviderRequiresConfiguredUsernameWithoutProvidedAssertion` (`internal/auth/entra_test.go`) covers both set-but-empty and genuinely unset assertion env vars so a regression treating them differently is caught.

### Accepted — no action (deliberate decisions, do not re-flag)

- [x] 12.8 **`opsx logout` is best-effort and intentionally incomplete.** Per the product owner, logout is not a security boundary: terminals are disposed by closing them and all STS sessions self-expire within ~1h, so the following are accepted as-is and MUST NOT be reopened as defects: logout does not delete per-cluster kubeconfig files; logout cannot purge credential profiles that have no `state.json` entry (the `HasSessionToken` reuse guard already prevents trusting orphaned credentials); the "removed N profile(s)" count reflects the planned set rather than profiles that actually existed; `--all` performs no confirmation prompt; `isOpsxManagedEntry` classifies purely by `Mode`; and `deleteProfiles` may leave cosmetic blank-line/comment drift. The only hard requirement is that logout never deletes or corrupts non-opsx-managed profiles (already covered by `TestRunLogoutPurgesCredentialsAndState` / `TestStoreDeleteProfilesPreservesUnrelatedContent`).
- [x] 12.9 **The `OPSX_SAML_ASSERTION` / `OPSX_SAML_ASSERTION_FILE` escape hatch is accepted.** Even with the real Entra+ADFS+MFA flow implemented, the env/file escape hatch remains the intended seam for off-network testing, CI, and recovery and is accepted without added expiry/format validation or file-permission checks.

## 13. Functional completeness — fourth review 2026-05-31

A fourth review focused on functional completeness (per the product owner: security/edge
deferrable in v1, but core function must work). Two items here block real first-use; the
product owner has decided the intended behavior for both, plus a related multi-terminal
correctness item. Re-apply TDD red-green-refactor: failing test first.

### P0 — Blocks real first-use

- [x] 13.1 **(config, entra-auth, account-switching, cluster-switching) STS region is configuration-driven, not ambient.** Today both `masterAssumeFactory` (`internal/cli/wiring.go`) and `STSAssume` (`internal/auth/citizen.go`) build the STS client with `LoadDefaultConfig` and **never set a region** (confirmed: zero `WithRegion` in production code). AWS SDK Go v2 requires a region to resolve the STS endpoint, so `opsx login` and `opsx use` fail with "an AWS region is required to send this request" on any machine without `AWS_REGION`/`AWS_DEFAULT_REGION` or a default in `~/.aws/config`. All integration tests use a fake assumer, so nothing catches this. **Product decision:** region is config-driven — a per-account `region` (an account may run EKS in several regions, so this is the account's STS/home region, distinct from each cluster's own region) and a master/`auth` region for the `AssumeRoleWithSAML` call. **Fix:**
  - Add `accounts.<alias>.region` (optional) and `auth.region` to the config schema.
  - Region resolution for citizen STS: `accounts.<alias>.region` → `auth.region` → `AWS_REGION`/`AWS_DEFAULT_REGION` env. For master STS: `auth.region` → env. If none resolves, fail at load/use with a clear, name-bearing error rather than the opaque SDK error.
  - Pass the resolved region into the STS client via `awsconfig.WithRegion(...)` for both master and citizen assumes.
  - Keep `clusters.<alias>.region` as the EKS region for `update-kubeconfig` (already present and required).
  - Add a test that the STS client is built with the configured region, and a config-validation test for the new region fields. **Do this before finalizing the Entra provider — otherwise the first real login on the company machine will hit this, not an Entra problem.**

- [x] 13.2 **(cluster-switching, shell-integration, introspection) `opsx kube` MUST switch the account profile too, not only `KUBECONFIG`.** Today `shell-switch kube` emits only `export KUBECONFIG=...` (`internal/cli/shellswitch.go`), so after a bare `opsx kube dev-syd` (no prior `opsx use`): the shell's `AWS_PROFILE` is unset, plain `aws` commands do not run as the cluster's account, and `opsx status` (which keys off `AWS_PROFILE`) prints "No active opsx context" and cannot show the just-recorded cluster — contradicting the introspection "status shows the cluster after kube" scenario. **Product decision:** because config already binds each cluster to an account, `opsx kube` SHALL switch the account profile first. **Fix:** `shell-switch kube` emits BOTH `export AWS_PROFILE=<alias.mode>` and `export KUBECONFIG=<path>` (two export lines, eval-safe). Update the shell-integration "Export-only stdout for cluster switch" scenario to expect both lines, and add an integration test that `opsx kube` alone (no prior `use`) leaves `AWS_PROFILE` set and makes `opsx status` show the account, cluster, and expiry.

### P1 — Multi-terminal correctness

- [x] 13.3 **(introspection) The displayed cluster must reflect THIS terminal, not shared state.** `recordCluster` stores the cluster on the shared, profile-keyed `state.json` entry. Two terminals using the same `<alias.mode>` profile but different clusters overwrite each other's `Cluster` field, so `opsx status` in terminal A can show terminal B's cluster while A's own `KUBECONFIG` env points elsewhere — undermining the multi-terminal isolation guarantee. **Fix:** make `opsx status` derive the active cluster from the per-terminal `KUBECONFIG` env (authoritative for this terminal) rather than from the shared state entry, or clearly label the state-derived value as "last recorded for this profile (may differ per terminal)". Add a test with two KUBECONFIG values mapping to the same profile.

### Deferred to v1.1 — acknowledged, not blocking v1

- [x] 13.4 **(introspection) `opsx status` is purely local and never verifies identity with AWS.** It can show "valid until X" for credentials that were revoked server-side. A lightweight `GetCallerIdentity` check (opt-in) would close the trust gap. Acceptable for v1.
- [x] 13.5 **(cluster-switching) `opsx kube` hard-requires `kubectl` though it never runs it.** `requiredTools = {aws, kubectl}` blocks operators who only use helm/a GUI. Consider requiring only `aws` for the switch and checking `kubectl` lazily.
- [x] 13.6 **(shell-integration, docs) Shell integration was zsh-only.** bash/fish operators originally got no auto-switch and had to use the manual `eval "$(opsx shell-switch …)"` fallback; README made that limitation prominent. Superseded for Bash/Git Bash by task 17.2; fish remains manual fallback.
- [x] 13.7 **(cli-foundation, UX) Bare `opsx mode opr` (outside the shell function) only prints an `export` line and changes nothing.** Consider a stderr hint that it must be run via the opsx shell function or `eval` to take effect.

## 14. Functional completeness — fifth review 2026-05-31

A fifth review after the section 13 fixes landed. One real functional bug plus two product
decisions. Re-apply TDD red-green-refactor: failing test first.

### P0 — Functional bug

- [x] 14.1 **(shell-integration, cluster-switching) A cluster alias containing `.` makes `opsx kube` fail.** `aliasPattern` (`^[A-Za-z0-9._-]+$`) allows dots, and `encodeKubeAlias` encodes `.`→`%2E` in the kubeconfig file name (`internal/paths/paths.go`), but the export validator `safeValue` (`internal/shell/emit.go`) is `^[A-Za-z0-9._/@:=+~-]+$` and **does not allow `%`**, so `shell.Export("KUBECONFIG", …)` rejects the path. Reproduced: `KubeConfig("dev.syd","admin")` → `…/kube/admin/dev%2Esyd.yaml` → `Export` returns "refusing to export unsafe value … contains shell metacharacters". The two features (dot-alias encoding vs export safety) contradict; all existing tests use the hyphenated `dev-syd`, so the `%2E` path never reached the emitter. **Fix (either):** (a) add `%` to `safeValue` — in a single `export KEY=value` assignment `%` is not a shell metacharacter, so this is safe; or (b) switch `encodeKubeAlias`/`decodeKubeAlias` to an encoding that uses only characters already allowed by `safeValue`. **Add a test that `opsx shell-switch kube <dotted-alias>` emits a valid, eval-safe `export KUBECONFIG=` line and that `ClusterFromKubeConfig` round-trips the dotted alias.**

### P1 — Product decisions

- [x] 14.2 **(cluster-switching) `opsx kube` MUST print two confirmation lines after switching.** Because `opsx kube` now switches both the account profile and the cluster, the human summary SHALL state both explicitly as two lines — one that the account was switched, one that the EKS cluster was switched — so the operator sees that their `aws` identity changed too, not only `KUBECONFIG`. These lines go to stderr (stdout stays export-only for `eval`). Apply to both the `shell-switch kube` path (so the confirmation appears in the normal installed-function flow) and the bare `opsx kube` path.
- [x] 14.3 **(introspection) Surface region in `status` and `ls`.** `opsx status` SHALL show the region of the active context (the cluster's region when a cluster is active, otherwise the account's resolved STS region), and `opsx ls` SHALL show each account's configured `region` alongside its description (it already shows each cluster's region). This makes the multi-region story visible to the operator.

### Acknowledged — owner-implemented, no task

- [x] 14.4 **Real Entra login finalization is owner-implemented.** `EntraSAMLProvider.FetchAssertion` now performs the Entra bootstrap, ADFS form authentication, Microsoft relay, optional MFA polling, and SAMLResponse extraction when no assertion env/file is supplied. `OPSX_SAML_ASSERTION` / `_FILE` remain accepted escape hatches. Region resolution (13.1) is already fixed, so live company-machine verification should surface Entra/MFA issues rather than STS region masking.

## 15. Review remediation — sixth adversarial review 2026-05-31

A sixth review focused on functional completeness and serious bugs. Two items selected by the
product owner for fixing. TDD red-green-refactor applied: failing test first, then fix.

- [x] 15.1 **(shell-integration) Installed zsh wrapper `eval`s non-export stdout — `opsx use --help` bricks the shell.** Switching subcommands (`use`/`kube`/`mode`) route through `eval "$(command opsx shell-switch "$@")"` (`internal/shell/initzsh.go`), but cobra prints `--help`/`-h` text to stdout with exit 0. The wrapper captured that exit-0 stdout and `eval`'d it, producing `(eval):1: command not found: Switch …` in the user's live shell (reproduced against the real binary). The wrapper trusted that switching subcommands only emit export lines — false for help. **Fix:** (a) route any `-h`/`--help` invocation of a switching subcommand directly to the bare binary (also shows the real subcommand help, not `shell-switch`'s); (b) defense-in-depth — `eval` only blank or `export `-prefixed lines, refusing other stdout with a non-zero status. Tests: `TestInitScriptZshDoesNotEvalHelpOutput` (help shown, never eval'd) and `TestInitScriptZshRefusesNonExportOutput` (non-export line refused, not executed) in `internal/shell/emit_test.go`. Spec: shell-integration "Wrapper only intercepts switching subcommands" extended with the export-only-eval contract and two scenarios.
- [x] 15.2 **(introspection) `opsx status` Mode display can lie — shows stale `OPSX_MODE` over the active profile's real mode.** `renderStatus` (`internal/cli/status.go`) preferred the `OPSX_MODE` env over the active profile's recorded mode. A terminal with `OPSX_MODE=admin` that ran `opsx use prod --opr` (which sets `AWS_PROFILE=prod.opr` but never touches `OPSX_MODE`) would show `Mode: admin` while the live credentials are `prod.opr`. **Fix:** show the active profile's recorded `entry.Mode` when a state entry exists (authoritative — the mode the active creds were minted under; the profile name encodes it); fall back to `OPSX_MODE` only for a profile opsx has no state for, labeled as a default. Test: `TestRenderStatusModeReflectsActiveProfileNotStaleEnv` in `internal/cli/introspection_test.go`. Spec: introspection "Show current terminal context" extended with mode-precedence rule and a scenario.

### Reviewed — no change (low severity / already mitigated)

- [x] 15.3 **(cluster-switching / introspection) `switchCluster` discards the `ok` flag from `ss.Get` and threads a possibly-zero expiry into `recordCluster`.** `internal/cli/kube.go` reads `entry, _, err := ss.Get(profile)` and passes `entry.Expiry` to `recordCluster`, which uses it only when seeding a previously-absent entry. In the normal single-process flow `switchAccount` always writes the state entry first, so `Get` returns the real expiry; the zero-expiry path is reachable only via a concurrent `opsx logout` racing between the `Get` and the `Update`. Largely already mitigated by 12.1. Accepted as-is; the only residual is a near-impossible race, and the hard guarantee (status never shows EXPIRED for valid creds in the common path) holds.

## 16. Windows / PowerShell support planning (N8 / v1.1)

Windows is not part of the completed v1 scope. These tasks make the deferred scope explicit so
the project does not accidentally document Windows as supported before the binary and shell
integration are both real.

- [x] 16.1 **(cli-foundation) Windows cross-build target.** Update the build targets so a Windows cross-compile produces `bin/opsx-windows-amd64.exe` with `CGO_ENABLED=0 GOOS=windows GOARCH=amd64`; add a verification command or Makefile target that fails non-zero if the build breaks.
- [x] 16.2 **(shell-integration) Shell dialect abstraction.** Refactor the shell emitter boundary so switching services return environment key/value updates and dialect-specific emitters format them as POSIX `export` lines or PowerShell `$env:` assignments; keep zsh behavior unchanged.
- [x] 16.3 **(shell-integration) PowerShell assignment emitter.** Implement the PowerShell emitter for `AWS_PROFILE`, `KUBECONFIG`, and `OPSX_MODE`, including validation that refuses unsafe keys/values and never emits POSIX `export` for PowerShell.
- [x] 16.4 **(shell-integration) `opsx init powershell`.** Add a PowerShell function generator that routes only `use`, `kube`, and `mode` through `shell-switch`, handles leading global flags, bypasses help directly, preserves non-zero failures, and refuses non-assignment stdout.
- [x] 16.5 **(tests) PowerShell shell behavior coverage.** Add table-driven tests for PowerShell `use`, `kube`, and `mode` assignment output, wrapper help handling, failure propagation, and non-assignment refusal; skip live PowerShell execution tests when `pwsh` is unavailable.
- [x] 16.6 **(docs) Windows usage documentation.** Update README, PRD, and architecture usage notes after implementation to describe Windows installation, PowerShell profile setup, manual fallback, and remaining platform limitations.
- [x] 16.7 **(verification) Windows / PowerShell verification.** Run OpenSpec validation, the full Go test suite, lint/vet, the Windows cross-build target, and PowerShell-specific tests before marking N8 complete.

## 17. Cross-platform fsutil follow-up

- [x] 17.1 **(credential-store) Platform-aware parent-directory sync.** Split `defaultSyncDir` into platform-specific implementations so macOS/Linux fsync the parent directory after atomic rename, while Windows treats the directory sync as a no-op instead of failing every atomic write; keep temp-file fsync before rename on all platforms and verify Windows/macOS/Linux builds compile.
- [x] 17.2 **(shell-integration) Bash/Git Bash auto-switch wrapper.** Add `opsx init bash` for Bash and Windows Git Bash so `opsx use`/`opsx kube`/`opsx mode` run through guarded POSIX `shell-switch` eval and export `AWS_PROFILE`/`KUBECONFIG`/`OPSX_MODE` into the current shell. Add a failing live Bash wrapper test first, support `bash` as a POSIX dialect alias, and update README/spec/design/proposal docs.
- [x] 17.3 **(shell-integration) Command Prompt auto-switch wrapper.** Add `opsx init cmd` and `--shell cmd` assignment output so Windows Command Prompt users can install an `opsx.cmd` PATH-priority wrapper that applies `set "AWS_PROFILE=..."`, `set "KUBECONFIG=..."`, and `set "OPSX_MODE=..."` in the current `cmd.exe`. Add failing cmd dialect/wrapper tests first and update README/spec/design/proposal docs.

## 18. Owner-added MFA implementation alignment — 2026-06-05

- [x] 18.1 **(entra-auth/docs) Align planning artifacts with the owner-added MFA implementation.** Scan `internal/auth/entra.go` and update README, PRD, architecture, OpenSpec proposal/design/specs, and task history so they describe the implemented Entra bootstrap → ADFS form auth → Microsoft relay → MFA poll → SAMLResponse extraction path, with `OPSX_SAML_ASSERTION` / `_FILE` retained as accepted escape hatches.
- [x] 18.2 **(entra-auth/tests) Add local mock HTTP regression coverage for the MFA flow.** Add a test that drives `EntraSAMLProvider.FetchAssertion` through bootstrap, credential-type lookup, ADFS form submission, Microsoft relay, MFA BeginAuth/EndAuth/post, stderr number display, and SAMLResponse extraction without live Entra/AWS dependencies.
- [ ] 18.3 **(entra-auth/live) Company-machine live verification.** On the proxy-gated company machine, run `opsx login` against the real Entra/ADFS/MFA endpoints and compare the request/response assumptions against the legacy Python script. Keep the env/file assertion escape hatch available for recovery and off-network testing.
- [x] 18.4 **(entra-auth/config) Make Entra URLs configurable and trim unused config.** Add `auth.entra.base_url`, `auth.entra.ms_login_url`, and `auth.entra.myapps_url` with defaults to `https://auth.entra.io`; use top-level `auth.entra.app_id` for the login bootstrap; remove `tenant_id` and `domain_map` from the current config schema instead of carrying an older-config fallback.
- [x] 18.5 **(entra-auth/debug) Add safe debug logging for the live Entra flow.** Add optional `auth.entra.debug`; when enabled, log sanitized bootstrap/federation/ADFS/relay/MFA/SAML-extraction milestones to stderr without printing passwords, assertions, cookies, query strings, MFA flow tokens, session IDs, or canary values.

## 19. Default-profile overwrite — 2026-06-05

`opsx use` always overwrites the shared `[default]` profile with the freshly-assumed citizen
credentials (in addition to `[<alias>.<mode>]`), so `aws`/`kubectl` work with no `AWS_PROFILE`
and no shell integration — covering shells where opsx cannot inject env vars (Windows PowerShell
under a restrictive ExecutionPolicy, Command Prompt). opsx is a local single-user tool, so
`[default]` is overwritten unconditionally — no config toggle, no flag, no ownership marker (see
account-switching "Default-profile overwrite"). Re-apply TDD red-green-refactor: failing test
first.

- [x] 19.1 **(account-switching / credential-store) Write `[default]` on every `opsx use`.** Write failing tests first, then have `CitizenService.Use` also upsert the `[default]` profile with the same assumed citizen credentials, reusing the existing preservation rules (comments / blank lines / unrelated profiles round-trip unchanged). Tests: both `[dev.<mode>]` and `[default]` written; `[default]` holds `prod` creds after `use dev` then `use prod`; unrelated content preserved. _(Done: `creds.DefaultProfile` const; `CitizenService.Use` writes `[default]` in both fresh-assume and reuse paths; `TestUseAlsoWritesDefaultProfile` + `TestUseRewritesDefaultOnReuse`.)_
- [x] 19.2 **(credential-store) `opsx logout` clears `[default]`.** Write a failing test first, then have logout remove the `[default]` profile alongside the other purged opsx-managed profiles, still preserving unrelated content. _(Done: `logoutProfiles` adds `creds.DefaultProfile`; `TestRunLogoutRemovesDefaultProfile` + updated plan tests.)_
- [x] 19.3 **(docs) Document default-profile overwrite.** Update README (PRD/architecture already updated): `opsx use` overwrites `[default]`, why (locked-down PowerShell / Command Prompt), that `[default]` reflects the latest `opsx use` (no per-terminal isolation on that path), and that `opsx kube` default-path support is a deferred follow-up. _(Done: new "Default profile (works in any shell, no setup)" README section.)_
- [x] 19.4 **(verification) Gate the change.** Run `gofmt`, `golangci-lint run ./...`, `go vet ./...`, `go test -race -cover ./...`, and `make build`; confirm existing `[<alias>.<mode>]`/AWS_PROFILE behavior is unchanged and the new `[default]` path is covered. _(Done: gofmt clean, vet clean, lint 0 issues, race suite green — creds 88.8% / shell 88.9% / config 97.8% — make build OK.)_

### Deferred follow-up (not in this task block)

- [ ] 19.5 **(cluster-switching) `opsx kube` default-path equivalent.** `opsx kube` faces the same parent-env limitation for `KUBECONFIG`. The analogous fix is writing/merging into the default `~/.kube/config` (and overwriting `[default]` like 19.1) so a locked-down shell can use the cluster with no env var. Scope and spec separately before implementing.
