---
stepsCompleted: [1, 2, 3, 4, 5, 6, 7, 8]
lastStep: 8
status: 'complete'
completedAt: '2026-05-30'
inputDocuments:
  - _bmad-output/planning-artifacts/PRD.md
workflowType: 'architecture'
project_name: 'ops-cli'
user_name: 'Tiny'
date: '2026-05-30'
decision:
  direction: 'Plan B — Native single static binary (Go + aws-sdk-go-v2)'
  pluggable: 'login/auth module behind an Authenticator interface'
  deferred: 'credential_process delivery → v2'
  gaps_resolved: ['G1 Entra auth input config', 'G2 ~/.aws/credentials concurrent file lock', 'G3 per-terminal KUBECONFIG naming', 'G4 expiry metadata/state', 'G5 citizen role ARN composition']
---

# Architecture Decision Document

_This document builds collaboratively through step-by-step discovery. Sections are appended as we work through each architectural decision together._

## Project Context Analysis

### Requirements Overview

**Functional Requirements:**
11 must-have features (M1–M11) across four domains:
- Authentication (M2): Entra + ADFS + interactive MFA → SAMLResponse →
  AssumeRoleWithSAML into master_admin / master_AWSOpr; both cacheable simultaneously.
  `OPSX_SAML_ASSERTION` / `_FILE` remain accepted escape hatches behind the same
  SAMLProvider seam.
- Configuration (M3): declarative YAML (accounts + clusters) at ~/.config/opsx/config.yaml.
- Context switching (M4 use, M5 kube, M6 mode): derive citizen creds from cached master
  creds; switch account/cluster by short alias; per-terminal mode (admin|opr).
- Shell integration & isolation (M7, M8): per-terminal AWS_PROFILE + KUBECONFIG via
  `opsx shell-switch` + parent-shell evaluation; v1 ships one-time `opsx init zsh`
  and `opsx init powershell` functions.
- Introspection (M9 status, M10 ls) and clear expiry errors (M11).
7 nice-to-haves (N1–N7) explicitly deferred to v2 / v1.1.

**Non-Functional Requirements (architecture-driving):**
- Concurrency safety: two terminals operate different account/mode with ZERO credential
  collision — validated by a deliberate cross-terminal test. (Hardest constraint.)
- Switch latency < 2s once logged in → high-frequency commands (use/kube) must hit the
  credential cache, never trigger MFA.
- MFA frequency ≤ 1 per master role per hour → credential caching + expiry detection are core.
- Single static binary, no runtime dependency (M1) → Go + native aws-sdk-go-v2.
- Proxy: always honor system env vars (HTTP(S)_PROXY/NO_PROXY); never hardcode proxy in code.

**Scale & Complexity:**
- Primary domain: CLI / system tooling (Go single static binary)
- Complexity level: Medium — small surface area, but precise credential-lifecycle and
  concurrency-isolation logic.
- Estimated architectural components: ~7 (auth, config, credential store/cache, account
  switch, cluster/kube, shell-switch emitter, status/introspection).

### Technical Constraints & Dependencies

- Language/stack locked: Go single static binary; macOS primary, Linux secondary.
  Windows amd64 cross-build is available. The shell package keeps output dialects
  pluggable so POSIX/zsh and PowerShell assignment syntax stay isolated from switching
  business logic.
- External tools shelled out: `aws eks update-kubeconfig` (kube); `kubectl`/`helm` follow
  the per-terminal KUBECONFIG. STS operations are native (SDK), NOT via aws CLI.
- Entra SAML real implementation lives in `internal/auth/entra.go` and remains
  company-environment dependent (proxy-gated network). v1 can also consume a pre-obtained
  SAMLResponse/assertion, so the auth path remains a pluggable module behind an Authenticator
  interface.
- Three-tier credential chain, each 1h TTL: Entra SAML assertion → master STS → citizen STS.
- Child process cannot mutate parent shell env → indirection via shell-switch plus the
  parent shell's native evaluation mechanism (`eval` for POSIX/zsh, `Invoke-Expression`
  through a guarded wrapper for PowerShell).

### Cross-Cutting Concerns Identified

- Credential lifecycle: caching, expiry detection, clear re-login prompts (M11).
- Concurrency isolation: shared ~/.aws/credentials file requires locking (G2); per-terminal
  state via exported env (AWS_PROFILE, KUBECONFIG) (G3).
- Process / shell boundary: shell-switch as the only way to affect the parent terminal.
- External orchestration: invoking aws/kubectl/helm with correct region/name/context.
- Network: system-env proxy for all HTTP + AWS API calls (never hardcoded in code).

### Resolved Gaps from PRD Review

- G1: Entra auth inputs are configured under `auth.entra.*`, with the password prompted or
  sourced from `OPSX_PASSWORD`; assertion env/file escape hatches are supported.
- G2: Concurrent writes to ~/.aws/credentials and state are serialized by gofrs/flock.
- G3: Per-terminal KUBECONFIG files live under `~/.config/opsx/kube/<mode>/`.
- G4: Expiry metadata/state lives in `~/.config/opsx/state.json`.
- G5: Master/citizen role ARNs are composed from config, with role names keyed by mode.

## Starter Template Evaluation

### Primary Technology Domain

CLI / system tooling — Go single static binary. No web/UI scaffold applies; the
"starter" decision is the CLI framework + core library baseline + project layout.

### Starter Options Considered

- Cobra (spf13/cobra): de-facto standard Go CLI framework (kubectl, helm, gh, docker).
  Built-in subcommands, flags, help, shell completion. Best fit for opsx's 8 subcommands.
- urfave/cli v3: lighter, simpler API; smaller ecosystem and fewer references.
- stdlib `flag`: zero-dependency but no subcommand ergonomics — too bare for 8 commands.

### Selected Baseline: Cobra + yaml.v3 + aws-sdk-go-v2 (+ gofrs/flock)

**Rationale for Selection:**
- Cobra is the boring, proven standard — minimizes bespoke command-routing/help code and
  is instantly familiar to any teammate (M1 / onboarding goals).
- Viper deliberately rejected: opsx reads a single YAML file; Viper's multi-source merge is
  over-abstraction (Rule of Three). Plain `gopkg.in/yaml.v3` is simpler and more transparent.
- `aws-sdk-go-v2` gives native AssumeRoleWithSAML / AssumeRole — honors M1 (no aws CLI for STS).
- `gofrs/flock` provides the cross-platform advisory lock needed for concurrent-safe writes
  to ~/.aws/credentials (gap G2).

**Verified Versions (web-checked 2026-05-30):**
- github.com/spf13/cobra — v1.10.x (latest stable, 2025-12-04). Stay on v1.x (v2 unverified).
- github.com/aws/aws-sdk-go-v2 — modular, actively maintained 2026 (config, credentials, service/sts).
- gopkg.in/yaml.v3 — latest.
- github.com/gofrs/flock — latest.
- Go toolchain: latest stable (resolve at init via `go mod`).

**Initialization (first implementation story):**
```bash
mkdir opsx && cd opsx
go mod init github.com/<org>/opsx
go get github.com/spf13/cobra@latest
go get github.com/aws/aws-sdk-go-v2/config@latest
go get github.com/aws/aws-sdk-go-v2/service/sts@latest
go get gopkg.in/yaml.v3@latest
go get github.com/gofrs/flock@latest
# optional scaffold: go run github.com/spf13/cobra-cli@latest init
```

**Architectural Decisions Provided by Baseline:**

**Language & Runtime:** Go, single static binary; CGO disabled for portability
(`CGO_ENABLED=0`); macOS (primary) + Linux cross-compile targets, plus a Windows amd64
cross-compile target that produces `opsx-windows-amd64.exe`.

**CLI Framework:** Cobra — one command per subcommand, persistent `--opr` / mode flags.

**Config:** yaml.v3 decoding ~/.config/opsx/config.yaml into typed structs.

**Concurrency primitive:** gofrs/flock around all credentials-file read-modify-write.

**Code Organization (proposed, refined in next step):**
```
cmd/opsx/main.go            # entrypoint, wires Cobra root
internal/auth/              # Authenticator interface + Entra SAML impl (pluggable, G1)
internal/config/            # YAML load + validation
internal/creds/             # STS assume chain + credentials-file writer (flock, G2)
internal/state/             # per-terminal/expiry metadata (G4)
internal/switcher/          # `use` account switching
internal/kube/              # `kube` — shells out to aws eks update-kubeconfig (G3)
internal/shell/             # shell-switch dialect emitters + `init zsh`/`init powershell`
```

**Development Experience:** standard `go build` / `go test`; no extra build tooling.

**Note:** Project initialization using these commands should be the first implementation story.

## Core Architectural Decisions

### Decision Priority Analysis

**Critical Decisions (Block Implementation):**
- D1 Pluggable auth seam (SAMLProvider interface) — isolates the only company-specific unknown.
- D2 Credential storage & isolation = static `[alias.mode]` profiles + injected AWS_PROFILE (option a).
- D3 Concurrency lock (gofrs/flock) on all credentials-file writes (G2).
- D5/G1 `auth:` config section; all role names configurable (master + citizen).
- G5 ARN composition rule driven entirely by config (no hardcoded names).

**Important Decisions (Shape Architecture):**
- D4 Per-terminal KUBECONFIG = `~/.config/opsx/kube/<mode>/<encoded-cluster>.yaml` (option a, G3).
- D4-state Expiry/state sidecar `~/.config/opsx/state.json` (G4).
- D6 Three-tier STS chain + regional endpoints + auditable session names.
- D8 Per-terminal mode via injected OPSX_MODE env (never persisted).

**Deferred Decisions (Post-MVP / v2):**
- credential_process delivery (replaces static profiles) → N1 background refresh era.
- Background token refresh daemon (N1); auto-discovery (N2); bash/fish generators (N7).

### Authentication & Security

**D1 — Pluggable auth seam (G1).** Fetching the SAML assertion is the company-specific part
of the system. Everything downstream (assume master, assume citizen, write creds) is stable
and native. The seam is therefore minimized to a single method:

```go
type MasterRole int // Admin | Opr

type SAMLProvider interface {
    // Returns a base64-encoded SAML assertion for the requested master role.
    // v1 may run Entra+ADFS+MFA or source an assertion from
    // OPSX_SAML_ASSERTION / OPSX_SAML_ASSERTION_FILE.
    FetchAssertion(ctx context.Context, role MasterRole) (assertion string, err error)
}
```

`AssumeRoleWithSAML(principalArn, roleArn, assertion)` is done natively by opsx via
aws-sdk-go-v2 (stable). The Entra implementation (`EntraSAMLProvider`) performs bootstrap,
ADFS form auth, Microsoft relay, MFA polling, and SAMLResponse extraction inside
`internal/auth/entra.go`; live company-machine verification touches only that seam.

**D5 / G1 — Auth configuration.** Added `auth:` section to config.yaml. All role names are
configurable (no hardcoding), per Tiny's requirement:

```yaml
auth:
  master_account_id: "000000000000"
  saml_provider_arn: "arn:aws:iam::000000000000:saml-provider/EntraID"
  region: "us-east-1"
  entra:
    app_id: "..."
    username: "operator@example.com"
  master_roles:                # configurable master role NAMES
    admin: "master_admin"
    opr:   "master_AWSOpr"
  citizen_roles:               # configurable citizen role NAMES, by mode
    admin: "Admin"
    opr:   "AWSOpr"
```

The current provider uses `auth.entra.app_id` directly. Optional Entra endpoint overrides are
available but omitted from sample config unless non-default Entra endpoints are required. Older
`auth.entra.tenant_id` and `auth.entra.domain_map` entries are no longer part of the schema.
Optional `auth.entra.debug` enables stderr troubleshooting logs for the live Entra flow. These logs
are intentionally step-level and sanitized: endpoints are logged without query strings, and no
password, cookie, SAML assertion, MFA flow token, session ID, or canary value is emitted.

**G5 — ARN composition (config-driven, zero hardcoded names):**
- master role ARN  = `arn:aws:iam::{master_account_id}:role/{master_roles[mode]}`
- citizen role ARN = `arn:aws:iam::{accounts[alias].account_id}:role/{citizen_roles[mode]}`

This neutralizes PRD Assumption #3 (role-name consistency): inconsistent names are simply
reconfigured, never re-coded.

**D6 — STS assume chain.** `assertion → AssumeRoleWithSAML(master) → AssumeRole(citizen)`.
Regional STS endpoints (lower latency, partition resilience). Session name `opsx-<os-user>`
for CloudTrail traceability. Both master and citizen creds carry 1h TTL.

### Credential Storage, Isolation & Concurrency

**D2 (option a) — Static profiles + AWS_PROFILE injection.** Citizen creds are written as
standard `[<alias>.<mode>]` profiles in `~/.aws/credentials` (e.g. `[dev.admin]`,
`[prod.opr]`); master creds cached as `[master_admin]` / `[master_awsopr]`. `opsx shell-switch
use dev` emits `export AWS_PROFILE=dev.admin`. Isolation is structural: terminals export
different AWS_PROFILE values pointing at distinct profiles in the shared file — different
accounts never collide; same account+mode legitimately shares one profile. aws/kubectl/helm
read these profiles natively (zero adaptation). Rejected option b (per-terminal credentials
files) as over-engineered for v1.

**D2a — Unconditional default-profile overwrite.** D2's AWS_PROFILE injection relies on a shell
function / `eval` mutating the parent shell. Some target environments forbid that — most
concretely Windows PowerShell under a restrictive ExecutionPolicy (where `$PROFILE` and `.ps1`
never load) and Command Prompt. So `opsx use` also writes the assumed citizen credentials into
the shared `[default]` profile, letting `aws`/`kubectl` resolve the active account via the AWS
default-profile fallback — no env var, no function, no `eval`. opsx is a local single-user tool,
so `[default]` is treated as opsx-managed and overwritten unconditionally (no flag, no config
toggle, no ownership marker) — deliberately simpler than an opt-in escape hatch. `[default]` is
one shared file section, so it always reflects the latest `opsx use` and provides no isolation
of its own; the `[<alias>.<mode>]` profiles plus AWS_PROFILE injection remain the isolation path
for shells that support it, and this write is purely additive. `opsx logout` also clears
`[default]`. `opsx kube` has the analogous parent-env problem for `KUBECONFIG`; its default-path
equivalent (writing `~/.kube/config`) is tracked separately rather than bundled here.

**D3 — Concurrency lock + durable atomic replacement (G2).** Every read-modify-write of
`~/.aws/credentials` (and state.json) is guarded by a `gofrs/flock` advisory exclusive lock,
so concurrent `opsx use` from multiple terminals serializes. The lock is not crash safety by
itself: writes use temp-file replacement in the same directory, fsync the temp file, rename it
into place, then fsync the parent directory. This is what makes the PRD's cross-terminal
concurrency-safety and crash-safety criteria credible.

Credential and state consistency is explicit: a profile's credentials and state entry must not
persistently disagree. The implementation either writes both under one held lock and flushes
both before releasing it, or reconciles half-applied switches so orphaned credentials are not
silently trusted.

**D4 (option a) — Per-terminal KUBECONFIG (G3).** `opsx kube <cluster>` writes
a collision-free per-(cluster,mode) file under `~/.config/opsx/kube/<mode>/<encoded-cluster>.yaml` (via
`aws eks update-kubeconfig --kubeconfig <that path> --profile <alias.mode>`) and emits
both `export AWS_PROFILE=<alias.mode>` and `export KUBECONFIG=<that path>`. Each file holds
exactly one cluster and carries the AWS profile in its exec block, so its current-context is
pinned, plain `aws` commands use the cluster account, and kubectl can authenticate without a
separate `opsx use`; terminals export different KUBECONFIG paths → no cross-terminal context
collision. helm follows KUBECONFIG automatically. Rejected option b (per-session unique files)
— needs a terminal-session-id concept, unnecessary for v1.

**D4-state — Expiry/state sidecar (G4).** `~/.config/opsx/state.json`, keyed by profile name,
stores `{expiry, account, mode, cluster, updated_at}`. `opsx status` reads it; `use`/`kube`
compare expiry before acting and fail with a clear "run `opsx login`" message when stale (M11).
Kept separate from the credentials file to avoid polluting aws CLI parsing.

**D8 — Per-terminal mode.** `opsx mode admin|opr` emits `export OPSX_MODE=<mode>`;
per-command `--mode admin|opr` overrides symmetrically, with `--opr` kept as shorthand for
`--mode opr`. Disagreeing selectors fail clearly instead of silently choosing one. Mode is
never persisted (PRD §4) — purely runtime, so the same account can run different modes in
different terminals simultaneously.

### Networking

**Proxy (locked constraint).** All HTTP and AWS API clients rely solely on system proxy env
vars (HTTP(S)_PROXY / NO_PROXY) via `http.ProxyFromEnvironment` (default transport). Proxy is
NEVER hardcoded. Applies to both the Entra HTTP flow and all STS calls.

### Decision Impact Analysis

**Implementation Sequence:**
1. Project init + Cobra skeleton (cmd/opsx, root command).
2. config package (YAML load + validation, incl. auth section & ARN composition).
3. creds package: credentials-file writer + flock + state.json (no auth yet — testable with a fake SAMLProvider).
4. auth package: SAMLProvider interface + EntraSAMLProvider HTTP/MFA flow, plus company-machine live verification.
5. `login` → master cache; `use` → citizen profile; `mode`.
6. `kube` (shell out to aws eks update-kubeconfig) + KUBECONFIG emission.
7. shell package: shell-switch POSIX emitter + guarded `init zsh` wrapper (switching commands
   only, leading global flags handled, shell-switch failures propagated).
8. `status`, `ls`, expiry errors.
9. Windows cross-build target + PowerShell emitter/init generator, sharing the same switching
   services but using PowerShell-safe `$env:` output instead of POSIX `export`.

**Cross-Component Dependencies:**
- creds depends on config (ARN composition) and state (expiry); auth depends on config (auth section).
- shell-switch is the single point that affects the parent terminal (AWS_PROFILE / KUBECONFIG / OPSX_MODE).
  It selects a shell dialect at the boundary; business logic returns key/value updates, not
  shell syntax.
- flock is shared by creds and state writers.
- The SAMLProvider interface decouples everything else from the company-specific Entra flow,
  enabling test doubles and focused live verification on the company machine.

## Implementation Patterns & Consistency Rules

### Critical Conflict Points Identified

7 areas where independent AI agents could diverge: stdout/stderr discipline, error
wrapping & sentinels, profile/mode token spelling, Go naming/layout, Cobra command shape,
context propagation, and file permissions.

### stdout / stderr Discipline (most critical — protects the eval pattern)

- `opsx shell-switch …` MUST print ONLY environment-assignment lines for the selected shell
  dialect to **stdout**, one per line, nothing else — it is consumed by the parent shell's
  guarded evaluation path. The v1 POSIX/zsh dialect emits `export KEY=value`, for example
  `export AWS_PROFILE=dev.admin`. The PowerShell dialect emits `$env:KEY = "value"` and
  never emits POSIX `export`.
- ALL human output (prompts, MFA number-match, logs, errors, status tables) goes to **stderr**.
- Every other command prints user output to stdout normally, but NEVER prints secrets
  (passwords, tokens, SAML assertions) anywhere.
- Entra debug logs, when enabled by config, are safe diagnostics only: step names, sanitized
  endpoints, HTTP status, byte counts, and hidden-field counts. They MUST NOT include request
  bodies, cookies, SAML assertions, MFA flow tokens, session IDs, canary values, or query strings.

### Naming Patterns (Go)

- Packages: short, lowercase, no underscores (`auth`, `creds`, `config`, `shell`).
- Files: lowercase with underscores (`saml_provider.go`, `creds_writer.go`); tests co-located
  as `<name>_test.go`.
- Exported PascalCase, unexported camelCase; acronyms keep case (`ARN`, `STS`, `SAML`, `MFA`,
  `ID` → `accountID`, `roleARN`, `samlAssertion`).
- Interfaces named by capability (`SAMLProvider`, `CredentialStore`), not `IFoo`.

### Token Spelling (single source of truth — agents must not re-spell)

- Mode tokens are exactly `admin` and `opr` (CLI flag `--opr`, env `OPSX_MODE=opr`).
- Profile names: `<alias>.<mode>` → `dev.admin`, `prod.opr`. Master caches: `master_admin`,
  `master_awsopr` (lowercase), matching PRD §3.
- KUBECONFIG file: collision-free per-(cluster,mode) file under `~/.config/opsx/kube/<mode>/`.

### Structure Patterns

- `cmd/opsx/main.go` only wires the Cobra root, creates the signal-cancellable root context,
  and calls `cmd.ExecuteContext(ctx)`.
- One Cobra command per file under `internal/cli/` (or `cmd/opsx/`), each defining a
  `*cobra.Command` with `RunE` (return errors — NEVER `os.Exit` inside RunE; only main exits).
- Business logic lives in `internal/<domain>/`; commands are thin adapters calling it.
- Tests are table-driven, co-located `_test.go`; the SAMLProvider is faked in tests.

### Error Handling Patterns

- Wrap with context: `fmt.Errorf("assume citizen role %s: %w", alias, err)`.
- Sentinel errors for control flow, esp. expiry: `var ErrMasterExpired = errors.New(...)`,
  `ErrCitizenExpired`; detected via `errors.Is`. On these, print the M11 message to stderr:
  `master credentials expired — run: opsx login [--opr]` and exit non-zero.
- Commands return errors up to a single top-level handler that formats them to stderr and
  sets the exit code. No `panic` for expected conditions.

### Process Patterns

- `context.Context` is threaded through all auth/STS/HTTP calls (MFA polling honors ctx
  cancellation + timeout). First param of such functions is `ctx context.Context`.
- HTTP/AWS clients always use default transport (system-env proxy); never construct a
  Transport with an empty Proxy field.
- File writes: `~/.aws/credentials` and `state.json` written 0600; `~/.config/opsx/` dirs 0700;
  all mutations wrapped in a gofrs/flock exclusive lock and written via temp+fsync+rename+dir-fsync.

### Secrets Handling (login password)

The Entra flow requires a username + password (then interactive MFA). Decision:

- **Primary path — interactive no-echo prompt.** `opsx login` reads the password via
  `golang.org/x/term.ReadPassword` (no echo). Login is ≤ 1/master-role/hour, so prompting is
  acceptable and avoids any at-rest secret.
- **Fallback — `OPSX_PASSWORD` env var.** If set, it is used instead of prompting (scripting /
  CI convenience). Documented trade-off: env vars are process-visible and can leak into shell
  history — recommended only for non-interactive/CI use.
- **Resolution order:** `OPSX_PASSWORD` if present → else interactive prompt.
- **Username:** from `auth.entra.username` (config, non-secret) or prompted if absent.
- **Never persisted:** password is NEVER written to config, credentials file, state, or logs.
- **Zeroized:** held as `[]byte`, overwritten immediately after use (not stored as `string`).
- **Prompts/MFA number on stderr** (stdout discipline).
- **Behind the seam:** password collection is the `EntraSAMLProvider`'s responsibility; opsx
  core only provides the secure no-echo prompt helper and guarantees no persistence. OS
  Keychain storage is a v2 increment that swaps inside the provider only.

### Enforcement Guidelines

**All AI Agents MUST:**
- Keep `shell-switch` stdout pure (export lines only); route everything else to stderr; make
  the shell wrapper return non-zero when `shell-switch` fails.
- Drive every role ARN from config (master_account_id + master_roles/citizen_roles) — never
  hardcode account IDs or role names.
- Use the SAMLProvider interface as the only auth seam; never inline Entra HTTP logic into
  commands.
- Wrap errors with `%w` and use the expiry sentinels for re-login prompts.
- Never persist or log the password; zeroize it after use.

**Anti-Patterns (avoid):**
- Printing logs/prompts to stdout in any command consumed by eval.
- `os.Exit` inside Cobra RunE or deep in business logic.
- Hardcoded role names / account IDs / proxy settings.
- Storing the password in config/keychain in v1; logging tokens or SAML assertions.
- Writing creds without flock or without durable atomic replacement.

## Project Structure & Boundaries

### Complete Project Directory Structure

```
opsx/
├── go.mod
├── go.sum
├── README.md
├── .gitignore
├── Makefile                       # build / test / cross-compile (CGO_ENABLED=0)
├── cmd/
│   └── opsx/
│       └── main.go                # wires root cmd; single top-level error handler + exit code
├── internal/
│   ├── cli/                       # Cobra adapters (thin; RunE only, no business logic)
│   │   ├── root.go                # root cmd + persistent flags (--opr); dependency wiring
│   │   ├── login.go               # M2  opsx login [--opr]
│   │   ├── use.go                 # M4  opsx use <account-alias>
│   │   ├── kube.go                # M5  opsx kube <cluster-alias>
│   │   ├── mode.go                # M6  opsx mode admin|opr
│   │   ├── status.go              # M9  opsx status
│   │   ├── ls.go                  # M10 opsx ls
│   │   ├── init.go                # M8  opsx init zsh / powershell
│   │   └── shellswitch.go         # M8  opsx shell-switch … (parent-shell assignment target)
│   ├── config/                    # M3, G1, G5
│   │   ├── config.go              # types (accounts, clusters, auth) + Load + Validate
│   │   └── arn.go                 # ARN composition (master/citizen, config-driven)
│   ├── auth/                      # M2, G1 — the pluggable seam
│   │   ├── provider.go            # SAMLProvider interface + MasterRole
│   │   ├── entra.go               # EntraSAMLProvider HTTP/MFA flow
│   │   ├── prompt.go              # no-echo prompt + OPSX_PASSWORD fallback + zeroize
│   │   └── master.go              # AssumeRoleWithSAML → master STS
│   ├── creds/                     # D2, D3, M4, G2
│   │   ├── store.go               # ~/.aws/credentials read-modify-write (ini) under flock
│   │   ├── citizen.go             # AssumeRole → citizen STS; write [alias.mode]
│   │   └── expiry.go              # expiry checks + sentinels (ErrMasterExpired, ErrCitizenExpired)
│   ├── state/                     # D4-state, G4
│   │   └── state.go               # ~/.config/opsx/state.json read/write under flock
│   ├── kube/                      # M5, G3
│   │   └── kube.go                # shell out: aws eks update-kubeconfig --kubeconfig <path>
│   ├── shell/                     # M8, M7
│   │   ├── emit.go                # shell assignment emitters (stdout discipline)
│   │   └── initzsh.go             # zsh function generator
│   └── paths/
│       └── paths.go               # ~/.config/opsx, ~/.aws/credentials, kube/ path helpers
└── testdata/
    └── config.example.yaml        # sample config + test fixture
```

### Architectural Boundaries

**The auth seam (most important boundary):** `internal/auth/provider.go` defines
`SAMLProvider`. Nothing outside `internal/auth` knows about Entra/HTTP. `EntraSAMLProvider`
in `entra.go` is the only company-specific code; everything else is testable with a fake.

**Parent-shell boundary:** `internal/shell/emit.go` is the ONLY code that affects the parent
terminal, exclusively via shell-dialect environment assignment lines on stdout. The v1
POSIX/zsh dialect emits `export` lines; the PowerShell dialect emits PowerShell assignments
from the same key/value updates. No other package writes to stdout in a way that
could be evaluated by the parent shell.

**Credential-file boundary:** only `internal/creds/store.go` writes `~/.aws/credentials`; only
`internal/state/state.go` writes `state.json`. Both acquire a shared gofrs/flock lock — no
other package touches these files directly.

**Config boundary:** all role ARNs are produced by `internal/config/arn.go` from config data;
no other package constructs ARNs or embeds account IDs / role names.

### Requirements → Structure Mapping

| Feature | Location |
|---------|----------|
| M1 single binary | `cmd/opsx/main.go` + Makefile (CGO_ENABLED=0) |
| M2 login | `internal/cli/login.go` → `auth/` (provider, prompt, master) |
| M3 YAML config | `internal/config/` |
| M4 use | `internal/cli/use.go` → `creds/citizen.go` + `state/` |
| M5 kube | `internal/cli/kube.go` → `kube/kube.go` + `shell/emit.go` |
| M6 mode | `internal/cli/mode.go` → `shell/emit.go` (OPSX_MODE) |
| M7 isolation | `creds` (profiles) + `kube` (KUBECONFIG) + `shell/emit.go` |
| M8 shell integration | `internal/cli/init.go`, `shellswitch.go`, `shell/` |
| M9 status | `internal/cli/status.go` → `state/`, `creds/expiry.go` |
| M10 ls | `internal/cli/ls.go` → `config/` |
| M11 expiry errors | `creds/expiry.go` sentinels + `cmd/opsx/main.go` handler |

### Integration Points

**Internal:** cli (thin) → domain packages (auth/creds/kube/state/config) → shell/emit for any
parent-shell effect. flock shared by creds + state.

**External:** Entra HTTPS (auth/entra.go, system proxy); AWS STS API (auth/master.go,
creds/citizen.go via aws-sdk-go-v2); `aws eks update-kubeconfig` (kube/kube.go, os/exec);
kubectl/helm consume KUBECONFIG (no direct call).

**Data flow:** password/MFA → SAML assertion → master STS (cache + state) → citizen STS
(profile + state) → AWS_PROFILE/KUBECONFIG export → aws/kubectl/helm.

### Development Workflow

- Build: `make build` (`CGO_ENABLED=0 go build ./cmd/opsx`); cross-compile darwin/linux.
- Test: `go test ./...`; SAMLProvider faked, creds/state use temp dirs.

## Architecture Validation Results

### Coherence Validation ✅

**Decision Compatibility:** Go + Cobra + aws-sdk-go-v2 + yaml.v3 + gofrs/flock are mutually
compatible and all current (web-verified). No contradictions: the pluggable SAMLProvider seam,
static-profile isolation (D2-a), per-(cluster,mode) KUBECONFIG (D4-a), and flock-guarded writes
reinforce rather than conflict with each other.

**Pattern Consistency:** stdout/stderr discipline, error sentinels, token spelling, and Go
naming all align with the chosen stack. The "shell/emit is the only parent-shell effect" rule
is consistent with the eval-based M8 design.

**Structure Alignment:** the cmd/ + internal/ tree enforces every boundary — auth seam isolated,
creds/state as sole file writers, config/arn as sole ARN source. Structure enables the patterns.

### Requirements Coverage Validation ✅

**Functional Requirements Coverage:** M1–M11 each mapped to concrete files (see Requirements →
Structure Mapping). N1–N7 explicitly deferred to v2 / v1.1.

**Non-Functional Requirements Coverage:**
- Concurrency safety → static `[alias.mode]` profiles + flock + per-terminal env (D2-a/D3/D8).
- < 2s switch → use/kube hit cached creds; only login does MFA.
- ≤ 1 MFA / master-role / hour → master STS cache + expiry checks.
- Single static binary → Go, CGO_ENABLED=0, native SDK (no aws CLI for STS).
- Secrets → no-echo prompt + OPSX_PASSWORD fallback, never persisted, zeroized.
- Proxy → system env only, never hardcoded.

### Implementation Readiness Validation ✅

**Decision Completeness:** all critical decisions documented with verified versions and rationale.
**Structure Completeness:** full directory tree with per-file responsibility.
**Pattern Completeness:** all 7 conflict points addressed with enforcement + anti-patterns.

### Gap Analysis Results

- **Critical gaps:** none open. G1–G5 all resolved in this document.
- **Important (known, by design):** the exact Entra HTTP request shape and `auth.entra.*`
  field values depend on company Entra/ADFS pages and should be live-verified against the
  existing Python script — intentionally isolated behind SAMLProvider, does not block the rest
  of implementation.
- **Nice-to-have:** shell completion (N4), interactive selector (N5), bash/fish (N7) — v2 / v1.1.

### Validation Issues Addressed

PRD gaps G1–G5 raised during review are all resolved: G1 (auth: config section), G2 (flock),
G3 (KUBECONFIG naming), G4 (state.json), G5 (config-driven ARN composition). Role names
(master + citizen) made fully configurable per Tiny's requirement.

### Architecture Completeness Checklist

**Requirements Analysis**
- [x] Project context thoroughly analyzed
- [x] Scale and complexity assessed
- [x] Technical constraints identified
- [x] Cross-cutting concerns mapped

**Architectural Decisions**
- [x] Critical decisions documented with versions
- [x] Technology stack fully specified
- [x] Integration patterns defined
- [x] Performance considerations addressed

**Implementation Patterns**
- [x] Naming conventions established
- [x] Structure patterns defined
- [x] Communication patterns specified
- [x] Process patterns documented

**Project Structure**
- [x] Complete directory structure defined
- [x] Component boundaries established
- [x] Integration points mapped
- [x] Requirements to structure mapping complete

### Architecture Readiness Assessment

**Overall Status:** READY FOR IMPLEMENTATION

**Confidence Level:** High — the only unknown (Entra SAML wire details) is isolated behind a
one-method seam and confirmed feasible (HTTP + boto3-equivalent + while-poll all map to Go).

**Key Strengths:**
- True single static binary (M1 honored).
- Company-specific risk quarantined to one swappable provider.
- Concurrency safety structurally guaranteed (profiles + flock + per-terminal env).
- Zero hardcoding: roles, accounts, proxy all externalized.

**Areas for Future Enhancement (v2):**
- credential_process delivery; background refresh (N1); auto-discovery (N2); OS Keychain;
  shell completion (N4); interactive selector (N5); bash/fish generators (N7).

### Implementation Handoff

**AI Agent Guidelines:**
- Follow all architectural decisions exactly as documented.
- Keep shell-switch stdout pure for the selected shell dialect; route all else to stderr.
- Drive every ARN from config; never hardcode account IDs / role names / proxy.
- Use SAMLProvider as the only auth seam; wrap errors with %w; use expiry sentinels.

**First Implementation Priority:**
Project init + Cobra skeleton (go mod init; add cobra/aws-sdk-go-v2/yaml.v3/gofrs/flock;
cmd/opsx/main.go with root command and top-level error handler).
