---
stepsCompleted: [1, 2, 3, 4]
status: 'complete'
completedAt: '2026-05-30'
inputDocuments:
  - _bmad-output/planning-artifacts/PRD.md
  - _bmad-output/planning-artifacts/architecture.md
---

# opsx - Epic Breakdown

## Overview

This document provides the complete epic and story breakdown for opsx, decomposing the
requirements from the PRD and Architecture into implementable stories. (No UX Design
document exists — opsx is a terminal CLI.)

## Requirements Inventory

### Functional Requirements

FR1: Single static Go binary `opsx` with no runtime dependency; macOS primary, Linux secondary. (PRD M1)
FR2: `opsx login [--opr]` — programmatic Entra SAML auth + interactive MFA; `AssumeRoleWithSAML` into `master_admin` (default) or `master_AWSOpr` (`--opr`); cache master STS creds to `[master_admin]` / `[master_awsopr]`; both coexist. (PRD M2)
FR3: YAML config with `accounts`, `clusters`, and `auth` sections loaded from `~/.config/opsx/config.yaml`. (PRD M3)
FR4: `opsx use <account-alias>` — from cached master creds, assume the citizen role for the current terminal's mode; write `[<alias>.<mode>]` profile; inject `AWS_PROFILE` into the current terminal. (PRD M4)
FR5: `opsx kube <cluster-alias>` — auto-ensure the cluster's account creds for current mode; run `aws eks update-kubeconfig` for the alias's region + real name; set a per-terminal `KUBECONFIG`; helm follows automatically. (PRD M5)
FR6: `opsx mode admin|opr` — set the current terminal's default mode; per-command flags (e.g. `--opr`) override. (PRD M6)
FR7: Per-terminal isolation — both `AWS_PROFILE` and `KUBECONFIG` scoped to each terminal; two terminals never collide. (PRD M7)
FR8: Shell integration — `opsx init zsh` installs a one-time shell function; `opsx shell-switch …` emits `export` statements consumed via `eval`; manual fallback supported. (PRD M8)
FR9: `opsx status` — show the current terminal's active account, mode, cluster, and credential expiry. (PRD M9)
FR10: `opsx ls` — list configured account and cluster aliases. (PRD M10)
FR11: Clear expiry errors — when master/citizen creds are expired, fail with a clear message prompting `opsx login`. (PRD M11)

### NonFunctional Requirements

NFR1: MFA frequency ≤ 1 per master role per hour (typically ≤ 2/hour total). (PRD §7)
NFR2: Switching an account or cluster completes in < 2 seconds once logged in. (PRD §7)
NFR3: Ergonomics — every switch via a short alias; no account IDs, cluster names, or regions typed manually. (PRD §7)
NFR4: Concurrency safety — two terminals operate different accounts/modes with ZERO credential collision, validated by a deliberate cross-terminal test. (PRD §7)
NFR5: Single static binary, no runtime dependency (CGO_ENABLED=0); STS via native SDK, never the aws CLI. (PRD M1 / Arch)
NFR6: Secrets — login password via no-echo prompt with `OPSX_PASSWORD` env fallback; never persisted to config/creds/state/logs; held as `[]byte` and zeroized after use. (Arch)
NFR7: Proxy — all HTTP and AWS API calls honor system env vars (HTTP(S)_PROXY/NO_PROXY) only; proxy never hardcoded. (Arch)
NFR8: Onboarding — a teammate becomes productive by editing only `config.yaml`. (PRD §7)

### Additional Requirements

- **Starter / first story:** Project initialization + Cobra skeleton — `go mod init`; add cobra (v1.10.x), aws-sdk-go-v2 (config, credentials, service/sts), yaml.v3, gofrs/flock; `cmd/opsx/main.go` wiring the Cobra root + single top-level error handler. (Arch — First Implementation Priority)
- **Pluggable auth seam:** `internal/auth/provider.go` defines `SAMLProvider.FetchAssertion(ctx, role)`; `EntraSAMLProvider` is the only company-specific code, implementing Entra+ADFS+MFA behind the seam while command orchestration remains testable via fake. (Arch G1)
- **Config-driven ARN composition:** master ARN = `arn:aws:iam::{master_account_id}:role/{master_roles[mode]}`; citizen ARN = `arn:aws:iam::{accounts[alias].account_id}:role/{citizen_roles[mode]}`; all role names configurable, no hardcoding. (Arch G5)
- **Concurrency locking:** all read-modify-write of `~/.aws/credentials` and `state.json` guarded by a gofrs/flock advisory exclusive lock. (Arch G2)
- **Expiry/state store:** `~/.config/opsx/state.json` keyed by profile name stores `{expiry, account, mode, cluster, updated_at}`. (Arch G4)
- **KUBECONFIG naming:** `~/.config/opsx/kube/<mode>/<encoded-cluster>.yaml`, one cluster per file. (Arch G3)
- **stdout/stderr discipline:** `shell-switch` prints only `export KEY=value` to stdout; all prompts/logs/errors to stderr. (Arch)
- **External orchestration:** shell out to `aws eks update-kubeconfig`; kubectl/helm consume KUBECONFIG (no direct calls). (Arch)
- **File permissions:** credentials/state 0600; `~/.config/opsx/` dirs 0700. (Arch)
- **Project structure:** `cmd/opsx` + `internal/{cli,config,auth,creds,state,kube,shell,paths}`; thin Cobra adapters, business logic in domain packages. (Arch)

### UX Design Requirements

None — opsx is a terminal CLI; no UX Design document exists. Terminal UX concerns (no-echo
prompt, MFA number display, status table formatting) are captured under FR9, NFR6, and the
stdout/stderr discipline in Additional Requirements.

### FR Coverage Map

FR1: Epic 1 (Story 1.1) — single static binary + Cobra skeleton
FR3: Epic 1 (Story 1.2) — YAML config load/validate + ARN composition
FR10: Epic 1 (Story 1.3) — `opsx ls`
FR11: Epic 1 (Story 1.4) — creds/state foundation: expiry detection + sentinels
FR2: Epic 1 (Story 1.5) — `opsx login` (SAMLProvider seam + master cache)
FR4: Epic 1 (Story 1.6) — `opsx use` (citizen profile + AWS_PROFILE)
FR6: Epic 1 (Story 1.6) — `opsx mode`
FR7: Epic 1 (Story 1.6 AWS_PROFILE + Story 1.7 KUBECONFIG) — per-terminal isolation
FR8: Epic 1 (Story 1.6) — shell integration (init zsh + shell-switch)
FR5: Epic 1 (Story 1.7) — `opsx kube`
FR9: Epic 1 (Story 1.8) — `opsx status`

## Epic List

### Epic 1: opsx — Fast AWS Account & EKS Cluster Switcher (v1)

Deliver the complete v1 daily-driver CLI: an operator installs a single static binary,
declares accounts/clusters in YAML, authenticates once per master role per hour, then switches
between accounts and EKS clusters by short alias with full multi-terminal isolation. The whole
tool is one cohesive binary with no inter-epic feedback loop, so v1 is a single epic implemented
as 8 ordered, independently testable vertical-slice stories.

**FRs covered:** FR1–FR11 (all). **NFRs:** NFR1–NFR8.

**Planned stories (ordered):**
1. Story 1.1 — Project init + Cobra skeleton (FR1)
2. Story 1.2 — Config load/validate + ARN composition (FR3, G1/G5)
3. Story 1.3 — `opsx ls` (FR10)
4. Story 1.4 — Credentials/state foundation: store + flock + state.json + expiry sentinels (FR11, G2/G4)
5. Story 1.5 — `opsx login` + SAMLProvider seam + no-echo password + master STS cache (FR2, NFR1/NFR6)
6. Story 1.6 — `opsx use` + `opsx mode` + shell-switch emit + `init zsh` (FR4, FR6, FR8, FR7-AWS_PROFILE, NFR2/NFR3/NFR4)
7. Story 1.7 — `opsx kube` + per-terminal KUBECONFIG (FR5, FR7-KUBECONFIG)
8. Story 1.8 — `opsx status` (FR9)

---

## Epic 1: opsx — Fast AWS Account & EKS Cluster Switcher (v1)

Deliver the complete v1 daily-driver CLI as one cohesive single binary: install, declare
accounts/clusters in YAML, authenticate once per master role per hour, then switch accounts and
EKS clusters by short alias with full multi-terminal isolation. Implemented as 8 ordered,
independently testable vertical-slice stories (S4 builds the shared creds/state foundation used
by S5–S7).

### Story 1.1: Project init + Cobra skeleton

As a developer,
I want the opsx project scaffolded as a single-binary Cobra CLI with the agreed package layout,
So that every subsequent command plugs into a consistent skeleton that builds to one static binary.

**Acceptance Criteria:**

**Given** a clean checkout
**When** `make build` runs
**Then** a single static binary `opsx` is produced with `CGO_ENABLED=0`
**And** it cross-compiles for darwin and linux.

**Given** the built binary
**When** `opsx` runs with no arguments
**Then** Cobra prints help listing the planned subcommands (login, use, kube, mode, status, ls, init, shell-switch) as placeholders
**And** `opsx --version` prints a version string.

**Given** any command that returns an error
**When** it propagates to `cmd/opsx/main.go`
**Then** a single top-level handler prints the error to stderr and exits non-zero (no `panic`, no `os.Exit` inside RunE).

**Given** the repository
**When** inspected
**Then** the structure matches the architecture: `cmd/opsx/main.go` + `internal/{cli,config,auth,creds,state,kube,shell,paths}`
**And** `go.mod` declares cobra, aws-sdk-go-v2 (config, credentials, service/sts), yaml.v3, gofrs/flock.

### Story 1.2: Config load/validate + ARN composition

As an operator,
I want opsx to load and validate my YAML config and derive role ARNs from it,
So that accounts, clusters, and auth are correctly defined with no hardcoded account IDs or role names.

**Acceptance Criteria:**

**Given** a valid `~/.config/opsx/config.yaml` with `accounts`, `clusters`, and `auth` sections
**When** opsx loads it
**Then** all three sections decode into typed structs.

**Given** a cluster whose `account` references an unknown account alias
**When** the config is validated
**Then** a clear error names the offending cluster and the missing account alias.

**Given** a missing config file
**When** any command needs config
**Then** a clear error tells the user the expected path and to create it.

**Given** a loaded config and a mode
**When** composing ARNs
**Then** master ARN = `arn:aws:iam::{master_account_id}:role/{master_roles[mode]}`
**And** citizen ARN = `arn:aws:iam::{accounts[alias].account_id}:role/{citizen_roles[mode]}`.

**Given** role names changed in `master_roles` / `citizen_roles`
**When** ARNs are composed
**Then** the output ARNs change accordingly (composition is config-driven, never hardcoded).

### Story 1.3: `opsx ls`

As an operator,
I want `opsx ls` to list my configured account and cluster aliases,
So that I can discover what I can switch to without opening the YAML.

**Acceptance Criteria:**

**Given** a config with accounts and clusters
**When** `opsx ls` runs
**Then** it prints account aliases with their description, and cluster aliases with their account and region, to stdout.

**Given** an empty or absent `accounts` or `clusters` section
**When** `opsx ls` runs
**Then** it prints a friendly "none configured" line for that section rather than erroring.

**Given** an invalid config
**When** `opsx ls` runs
**Then** the validation error from Story 1.2 is printed to stderr and exit is non-zero.

### Story 1.4: Credentials/state foundation (store + flock + state.json + expiry sentinels)

As a developer,
I want a concurrency-safe credentials and state layer with expiry detection,
So that all commands read/write `~/.aws/credentials` and `state.json` without corruption and can fail clearly when creds are stale.

**Acceptance Criteria:**

**Given** two processes writing profiles concurrently
**When** they both call the store
**Then** a gofrs/flock advisory exclusive lock serializes the writes and the credentials file is never corrupted.

**Given** a credentials or state write
**When** it completes
**Then** `~/.aws/credentials` and `state.json` are mode 0600 and `~/.config/opsx/` directories are 0700.

**Given** a profile is written
**When** the store updates state
**Then** `state.json` records `{expiry, account, mode, cluster, updated_at}` keyed by profile name.

**Given** a profile whose recorded expiry is in the past
**When** expiry is checked
**Then** `ErrMasterExpired` / `ErrCitizenExpired` is returned and is detectable via `errors.Is`.

**Given** an expiry sentinel reaching the top-level handler
**When** formatted
**Then** a clear `master credentials expired — run: opsx login [--opr]` message prints to stderr and exit is non-zero.

**Given** the test suite
**When** run
**Then** it uses temporary directories and an injectable clock (no real home dir, no real wall-clock dependency).

### Story 1.5: `opsx login` + SAMLProvider seam + master STS cache

As an operator,
I want `opsx login [--opr]` to authenticate via Entra and cache master STS credentials,
So that I authenticate once per master role per hour instead of on every switch.

**Acceptance Criteria:**

**Given** `opsx login`
**When** invoked
**Then** authentication goes through `SAMLProvider.FetchAssertion(ctx, role)` (real `EntraSAMLProvider`; a fake in tests) — no Entra HTTP logic leaks outside `internal/auth`.

**Given** no pre-obtained SAML assertion is supplied
**When** the Entra/ADFS pages return the expected bootstrap, federation, relay, MFA, and SAML responses
**Then** `EntraSAMLProvider` prompts for the password, polls interactive MFA, and returns the extracted `SAMLResponse`.

**Given** `OPSX_SAML_ASSERTION` or `OPSX_SAML_ASSERTION_FILE` is set
**When** `opsx login` runs
**Then** the provider uses that assertion directly as an accepted escape hatch without prompting or polling MFA.

**Given** a returned SAML assertion
**When** login completes
**Then** `AssumeRoleWithSAML` caches master creds to `[master_admin]` (default) or `[master_awsopr]` (`--opr`) with a 1h expiry recorded in state.

**Given** password collection
**When** login runs
**Then** the password is read with no echo; if `OPSX_PASSWORD` is set it is used instead of prompting; the password is never written to config/creds/state/logs and is zeroized (`[]byte` overwritten) after use.

**Given** interactive MFA
**When** awaiting approval
**Then** the number/prompt prints to stderr and polling honors `ctx` timeout and cancellation (Ctrl-C exits cleanly).

**Given** both master roles are logged in
**When** caches are written
**Then** `[master_admin]` and `[master_awsopr]` coexist without overwriting each other.

**Given** system proxy env vars are set
**When** Entra HTTP and STS calls are made
**Then** they use the system proxy (no proxy hardcoded anywhere).

### Story 1.6: `opsx use` + `opsx mode` + shell-switch emit + `init zsh`

As an operator,
I want to switch accounts by short alias and set my terminal's mode, with the change applied to my current shell,
So that `AWS_PROFILE` points at the right citizen credentials with per-terminal isolation.

**Acceptance Criteria:**

**Given** valid cached master creds for the current mode
**When** `opsx use dev` runs
**Then** the citizen role is assumed, the `[dev.<mode>]` profile is written, and state is updated — completing in under 2 seconds with no MFA.

**Given** `opsx shell-switch use dev`
**When** it runs
**Then** stdout contains ONLY `export AWS_PROFILE=dev.admin` (export lines only); all prompts/logs go to stderr.

**Given** `opsx init zsh`
**When** it runs
**Then** it emits the one-time function `opsx() { eval "$(command opsx shell-switch "$@")"; }` suitable for appending to the rc file.

**Given** `opsx mode opr`
**When** it runs
**Then** shell-switch emits `export OPSX_MODE=opr`; a per-command `--opr` overrides it; the mode is never persisted to disk.

**Given** expired master creds
**When** `opsx use` runs
**Then** it fails with the Story 1.4 re-login message.

**Given** two terminals using different accounts/modes
**When** each runs `opsx use`
**Then** each exports its own `AWS_PROFILE` and they never collide (validated by a deliberate cross-terminal test).

### Story 1.7: `opsx kube` + per-terminal KUBECONFIG

As an operator,
I want to switch EKS clusters by short alias,
So that kubectl and helm in this terminal target the right cluster in isolation from other terminals.

**Acceptance Criteria:**

**Given** `opsx kube dev-syd`
**When** invoked
**Then** the cluster's account creds for the current mode are auto-ensured, then `aws eks update-kubeconfig` runs for the alias's region and real cluster name, writing `~/.config/opsx/kube/<mode>/<encoded-dev-syd>.yaml` with `--profile dev.<mode>`.

**Given** `opsx shell-switch kube dev-syd`
**When** it runs
**Then** stdout contains ONLY eval-safe assignment lines for `AWS_PROFILE=dev.<mode>` and `KUBECONFIG=~/.config/opsx/kube/<mode>/<encoded-dev-syd>.yaml`.

**Given** the exported KUBECONFIG
**When** kubectl or helm runs
**Then** the current-context is the target cluster and helm follows automatically (no extra flags).

**Given** two terminals on different clusters
**When** each runs `opsx kube`
**Then** each exports its own KUBECONFIG path and contexts never collide across terminals.

**Given** `aws` or `kubectl` is missing, or creds are expired
**When** `opsx kube` runs
**Then** a clear error explains the missing prerequisite, or prints the re-login message for expiry.

### Story 1.8: `opsx status`

As an operator,
I want `opsx status` to show my current terminal's active context,
So that I always know which account, mode, and cluster I'm on and when my credentials expire.

**Acceptance Criteria:**

**Given** a terminal with `AWS_PROFILE` / `KUBECONFIG` / `OPSX_MODE` set
**When** `opsx status` runs
**Then** it shows the active account, mode, cluster, and credential expiry (read from `state.json`) to stdout.

**Given** a terminal with no active opsx context
**When** `opsx status` runs
**Then** it clearly states nothing is active and suggests `opsx use` / `opsx login`.

**Given** the active profile's creds are expired
**When** `opsx status` runs
**Then** expiry is shown as expired with the re-login hint.

**Given** `opsx status` runs
**When** it reads state
**Then** it does not mutate credentials or state (read-only).
