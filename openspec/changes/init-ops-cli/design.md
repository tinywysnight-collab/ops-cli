## Context

`opsx` replaces a Python switcher script that re-authenticates to Entra (with MFA) on every account switch and writes to a single shared `default` profile. The replacement is a single static Go binary that authenticates once per master role per hour, then switches accounts/clusters by short alias with full multi-terminal isolation. See `proposal.md` for motivation and the per-capability `specs/` for behavioral requirements.

Hard constraints shaping the design:
- **Three-tier credential chain**, each with a 1h TTL: Entra SAML assertion → master STS → citizen STS.
- **Concurrency safety is the hardest requirement**: two terminals must operate different account/mode combinations with *zero* credential collision, validated by a deliberate cross-terminal test.
- **Latency < 2s** for `use`/`kube` once logged in → high-frequency commands must hit cached creds and never trigger MFA.
- **Single static binary**, no runtime dependency (`CGO_ENABLED=0`); STS done natively via `aws-sdk-go-v2`, never by shelling out to the `aws` CLI.
- **A child process cannot mutate its parent shell's environment** → switches must be delivered to the terminal indirectly via `shell-switch` + `eval`.
- **The Entra SAML wire details are company-specific and proxy-gated**, so the implemented HTTP/MFA flow is isolated behind a seam and must be live-verified against the existing Python script.
- **Proxy**: all HTTP and AWS API calls honor system env vars only (`HTTP(S)_PROXY`/`NO_PROXY`); never hardcoded.

## Goals / Non-Goals

**Goals:**
- One MFA per master role per hour; sub-2s account/cluster switches by short alias afterward.
- Structural concurrency safety across terminals (no credential collision), including concurrent `admin` and `AWSOpr` modes.
- Quarantine all company-specific Entra logic behind a one-method seam so the rest is testable off the company machine while the real HTTP/MFA path remains locally contained.
- Zero hardcoding of account IDs, role names, or proxy — everything config-driven.
- A clean `cmd/ + internal/` layout with enforced boundaries (auth seam, sole file writers, sole ARN source).

**Non-Goals (v1):**
- A complete/guaranteed `opsx logout`. Logout is best-effort and is NOT a security boundary: terminals are disposed by closing them and every STS session self-expires within ~1h. `opsx logout` purges opsx-managed credential profiles and state entries it can see, but it intentionally does not clean per-cluster kubeconfig files, cannot reach credential profiles with no `state.json` entry (the `HasSessionToken` reuse guard prevents trusting those orphans), and does not prompt before `--all`. Its only hard guarantee is that it never deletes or corrupts non-opsx-managed profiles. See tasks 12.8.
- Background/automatic token refresh or any long-running daemon (N1 → v2).
- `credential_process` delivery (deferred to the N1 background-refresh era); v1 uses static profiles.
- AWS Org / EKS auto-discovery (N2), per-account default region (N3), shell completion (N4), interactive selector (N5), `opsx exec` (N6), bash/fish generators (N7).
- Non-EKS Kubernetes, ECS, IAM/account management, credential encryption beyond file permissions, Windows-first support, any GUI.

## Decisions

**D1 — Pluggable auth seam (`SAMLProvider`).** Fetching the SAML assertion is the company-specific part of the system; everything downstream (`AssumeRoleWithSAML`, `AssumeRole`, writing creds) is stable and native. The seam is therefore minimized to a single method `FetchAssertion(ctx, role) (assertion string, err error)` in `internal/auth/provider.go`. `EntraSAMLProvider` is the only company-specific implementation: it performs Entra bootstrap, ADFS form authentication, Microsoft relay, optional MFA polling, and SAMLResponse extraction, while tests for command orchestration use a fake.
- *Alternatives:* embedding Entra HTTP logic directly in `login` (rejected — couples every other package to an unverified, proxy-gated flow and blocks off-machine testing); a fat multi-method auth interface (rejected — over-abstraction; only assertion-fetching is actually variable).

**D2 — Static `[alias.mode]` profiles + `AWS_PROFILE` injection.** Citizen creds are written as standard `[<alias>.<mode>]` profiles in `~/.aws/credentials` (e.g. `[dev.admin]`, `[prod.opr]`); masters cached as `[master_admin]`/`[master_awsopr]`. Isolation is structural: terminals export different `AWS_PROFILE` values pointing at distinct profiles in the shared file. `aws`/`kubectl`/`helm` read these natively, zero adaptation.
- *Alternatives:* per-terminal credentials files (rejected — over-engineered for v1, needs a session-id concept); `credential_process` (deferred to v2 with background refresh).

**D3 — `gofrs/flock` advisory lock + durable atomic file replacement (G2).** Every read-modify-write of `~/.aws/credentials` and `state.json` is guarded by a cross-platform advisory exclusive lock, so concurrent writers serialize. Actual crash safety comes from temp-file replacement: write the temp file in the same directory, `fsync` it, `os.Rename` it into place, then `fsync` the parent directory. The lock prevents concurrent corruption; the temp+rename+fsync sequence prevents partial/truncated durable results.
- *Alternatives:* no locking (rejected — corrupts the shared ini file under concurrency); OS-specific `flock(2)` (rejected — `gofrs/flock` abstracts macOS/Linux portably).

**D3-state — Credential/state consistency is explicit, not assumed.** Credentials and their state entry are separate files, so a crash between writes can otherwise leave a usable profile with unknown expiry/context. The implementation must either write both under one held lock and flush both before releasing it, or reconcile half-applied switches so orphaned credentials are not silently trusted.
- *Alternatives:* embedding expiry metadata in the credentials file (rejected — risks confusing standard AWS tooling); accepting eventual consistency (rejected — expiry is security-sensitive).

**D4 — Per-(cluster,mode) `KUBECONFIG` files (G3).** `opsx kube <cluster>` writes a collision-free per-(cluster,mode) kubeconfig under `~/.config/opsx/kube/` via `aws eks update-kubeconfig --kubeconfig <path> --profile <alias.mode>` and emits **both** `export AWS_PROFILE=<alias.mode>` and `export KUBECONFIG=<path>` — switching the terminal's account identity along with its cluster, since each cluster is bound to an account in config. Exporting only `KUBECONFIG` is a defect: plain `aws` commands would not run as the cluster's account and `opsx status` (keyed off `AWS_PROFILE`) would show nothing after a bare `kube`. Each file holds exactly one cluster, so its current-context is pinned and terminals can't collide; `helm` follows `KUBECONFIG` automatically. The layout is `kube/<mode>/<alias>.yaml`: mode is a directory level (validated to `admin`/`opr` by `NormalizeMode`), which already makes the historical `<cluster>.<mode>` filename collision structurally impossible. Any per-alias filename encoding is therefore defense-in-depth rather than the primary disambiguator (see task 12.3 — simplify to the remaining threat or retain with a documented rationale); it must still never escape the kube directory.
- *Alternatives:* shared `~/.kube/config` with context switching (rejected — global current-context collides across terminals, the exact pain being removed); per-session unique files (rejected — needs a terminal-session-id, unnecessary for v1).

**D4-state — `state.json` expiry sidecar (G4).** `~/.config/opsx/state.json`, keyed by profile name, stores `{expiry, account, mode, cluster, updated_at}`. `status` reads it; `use`/`kube` compare expiry before acting. Kept separate from `~/.aws/credentials` to avoid polluting `aws` CLI ini parsing.
- *Alternatives:* embedding expiry metadata as comments/extra keys in the credentials file (rejected — risks confusing standard AWS tooling).

**D5 / G1 / G5 — Config-driven `auth:` section + ARN composition.** `config.yaml` gains an `auth:` section (`master_account_id`, `saml_provider_arn`, `auth.region`, `entra.username`, `entra.app_id`, optional Entra endpoint URLs, and configurable `master_roles`/`citizen_roles` by mode). ARNs are composed, never hardcoded: master = `arn:aws:iam::{master_account_id}:role/{master_roles[mode]}`; citizen = `arn:aws:iam::{accounts[alias].account_id}:role/{citizen_roles[mode]}`. This neutralizes the PRD assumption about role-name consistency — inconsistent names are reconfigured, never re-coded. Older `tenant_id` / `domain_map` config entries are no longer part of the schema.
- *Alternatives:* hardcoded `Admin`/`AWSOpr` role names (rejected — brittle across accounts/orgs); Viper multi-source config (rejected — opsx reads one YAML file; Viper's merge is over-abstraction by the Rule of Three).

**D6 — Three-tier STS chain, native SDK.** `assertion → AssumeRoleWithSAML(master) → AssumeRole(citizen)` via `aws-sdk-go-v2`, regional STS endpoints (latency + partition resilience), session name `opsx-<os-user>` for CloudTrail traceability; both tiers carry 1h TTL. The STS region is **config-driven, not ambient**: `auth.region` for the master assume and an optional per-account `accounts.<alias>.region` for the citizen assume (resolution order: account → auth → `AWS_REGION` env), so `login`/`use` work on a clean machine and fail with a clear, name-bearing error when no region resolves rather than the opaque SDK endpoint error. An account may run EKS in multiple regions, so the account's STS region is distinct from each `clusters.<alias>.region`.
- *Alternatives:* relying solely on `AWS_REGION`/`~/.aws/config` (rejected — makes the two highest-frequency commands silently environment-dependent); shelling out to `aws sts ...` (rejected — violates the single-binary / no-runtime-dependency goal).

**D7 — Shell integration via `shell-switch` + guarded `eval` (M8).** A one-time zsh function installed by `opsx init zsh` routes switching subcommands (`use`, `kube`, `mode`) through `shell-switch`, captures its stdout, verifies the command succeeded, then `eval`s only the export lines. Non-switching subcommands run the bare binary. The wrapper must also recognize switching subcommands when global flags appear before them (`opsx --mode opr use dev`). `shell-switch` prints *only* `export KEY=value` lines to stdout; everything else (prompts, MFA number, logs, errors) goes to stderr. Manual `eval "$(opsx shell-switch use dev)"` fallback always works for portability.
- *Alternatives:* a background daemon writing env (rejected — no daemon in v1); requiring users to `eval` manually every time (rejected — poor ergonomics, kept only as fallback).

**D8 — Per-terminal mode via `OPSX_MODE` env (never persisted).** `opsx mode admin|opr` emits `export OPSX_MODE=<mode>`; per-command `--mode admin|opr` overrides symmetrically, with `--opr` kept as shorthand for `--mode opr`. Disagreeing selectors are rejected instead of silently choosing one. Mode is purely runtime so the same account runs different modes in different terminals simultaneously.
- *Alternatives:* persisting mode in config/state (rejected — PRD §4 requires one config to serve both modes; persistence would break concurrent dual-mode use).

**Stack & structure.** Cobra v1.10.x (proven, familiar — `kubectl`/`helm`/`gh` use it) + `yaml.v3` + `aws-sdk-go-v2` (`config`, `credentials`, `service/sts`) + `gofrs/flock` + `golang.org/x/term`. Layout: `cmd/opsx/main.go` (root wiring + single top-level error handler) and `internal/{cli,config,auth,creds,state,kube,shell,paths}`, with thin Cobra adapters (`RunE`, never `os.Exit`) over domain packages. Enforced boundaries: the auth seam is the only place that knows Entra; `creds/store.go` and `state/state.go` are the *only* writers of their files (both under flock); `config/arn.go` is the *only* ARN source; `shell/emit.go` is the *only* code that affects the parent shell.

## Risks / Trade-offs

- **Entra SAML wire details depend on company pages/proxy** → Mitigation: isolate entirely behind `SAMLProvider`; build and test command orchestration with a fake; keep the real `internal/auth/entra.go` flow live-verified against the Python baseline.
- **Static profiles in a shared `~/.aws/credentials`** could still race, corrupt, or become inconsistent with state → Mitigation: read-modify-write under `gofrs/flock`, durable atomic replacement, single-flight/recheck behavior for concurrent switches, and an explicit credentials/state consistency rule.
- **Advisory locks are only as strong as the filesystem** → Mitigation: document that multi-terminal safety assumes a local home directory; NFS/SMB lock semantics are an operational caveat.
- **Secret handling** (login password) could leak → Mitigation: no-echo prompt (`x/term`) with `OPSX_PASSWORD` fallback (documented as CI-only); held as `[]byte`, zeroized after use; never written to config/creds/state/logs; prompts/MFA on stderr only.
- **`shell-switch` stdout pollution or swallowed shell errors would break `eval`** → Mitigation: strict discipline — stdout carries only `export` lines; a single emitter (`shell/emit.go`) is the sole stdout-for-eval path; everything else to stderr; the zsh wrapper captures command output and returns non-zero on shell-switch failure before `eval`.
- **External tool dependence** (`aws`, `kubectl`, `helm`) → Mitigation: `kube` checks prerequisites and fails with a clear, actionable message; STS itself never depends on the `aws` CLI.
- **1h TTL expiry mid-task** → Mitigation (v1): clear `ErrMasterExpired`/`ErrCitizenExpired` sentinels surfaced as `run: opsx login [--opr]`; proactive background refresh deferred to N1/v2.
- **Cobra v2 not yet verified** → Mitigation: pin to v1.10.x (latest stable); revisit v2 later.

## Migration Plan

- Greenfield project — no data migration. Implementation order (from the architecture's sequence): (1) project init + Cobra skeleton; (2) `config` (load/validate + ARN); (3) `creds` + `state` foundation with flock (testable via fake `SAMLProvider`); (4) `auth` seam + `EntraSAMLProvider`; (5) `login`/`use`/`mode`; (6) `kube` + `KUBECONFIG`; (7) `shell` emitter + `init zsh`; (8) `status`/`ls` + expiry errors.
- **Adoption/rollback:** `opsx` runs alongside the legacy Python script; the operator cuts over once the cross-terminal concurrency test passes and retires the script. Rollback is simply continuing to use the Python script — no shared state is destructively migrated.

## Open Questions

- Live validation of the implemented `auth.entra.*` request chain on the company machine against the Python script, especially when Entra/ADFS page shape or MFA proof metadata changes.
- Final pinned versions of `aws-sdk-go-v2` submodules and the Go toolchain — resolved at `go mod init` time.
