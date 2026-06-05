## Why

A cloud-migration operator works across one master account (Entra/Azure AD SAML) and many AWS citizen accounts, each spanning multiple regions and EKS clusters. The current Python script re-runs full Entra login (and MFA) on *every* switch and writes to a single shared `default` profile, causing repeated MFA, cross-terminal credential collisions, no cluster switching, and no aliases. We need a tool that authenticates once per master role per hour, then switches accounts and clusters in seconds by short alias — with full multi-terminal isolation.

## What Changes

- Introduce `opsx`, a single static Go binary (Cobra CLI; macOS primary, Linux secondary; `CGO_ENABLED=0`, no runtime dependency).
- `opsx login [--opr]`: programmatic Entra SAML auth + interactive MFA → `AssumeRoleWithSAML` into `master_admin` (default) or `master_AWSOpr` (`--opr`); cache master STS creds (1h); both roles coexist.
- `opsx use <account-alias>`: from cached master creds, assume the citizen role for the current terminal's mode; write a `[<alias>.<mode>]` profile and inject `AWS_PROFILE` — no MFA, < 2s.
- `opsx kube <cluster-alias>`: auto-ensure account creds, run `aws eks update-kubeconfig`, and set a per-terminal `KUBECONFIG`; `kubectl`/`helm` follow automatically.
- `opsx mode admin|opr`: set the current terminal's default mode (runtime-only, never persisted); per-command `--opr` overrides.
- `opsx init zsh` + `opsx shell-switch …`: one-time shell function plus `eval`-consumed `export` lines, so switches take effect in the parent shell (manual fallback supported).
- `opsx status` / `opsx ls`: show the current terminal's active account/mode/cluster/expiry, and list configured aliases.
- Declarative YAML config (`~/.config/opsx/config.yaml`) with `accounts`, `clusters`, and `auth` sections; all account IDs and role names config-driven (zero hardcoding).
- Per-terminal isolation of `AWS_PROFILE` and `KUBECONFIG`, with `gofrs/flock`-guarded writes to `~/.aws/credentials` and `~/.config/opsx/state.json` so concurrent terminals never collide.
- Secrets handling: no-echo password prompt with `OPSX_PASSWORD` fallback; never persisted, zeroized after use. All HTTP/AWS calls honor system proxy env only.

Out of scope for v1 (deferred to v2): background token refresh/daemon (N1), AWS Org/EKS auto-discovery (N2), per-account default region (N3), shell completion (N4), interactive selector (N5), `opsx exec` (N6), bash/fish generators (N7).

## Capabilities

### New Capabilities
- `cli-foundation`: single static binary, Cobra root + subcommand skeleton, single top-level error handler, project layout, version, and build (Makefile, `CGO_ENABLED=0`, darwin/linux cross-compile). [FR1]
- `config`: load and validate `config.yaml` (`accounts`, `clusters`, `auth`) into typed structs; config-driven master/citizen ARN composition with clear validation errors. [FR3, G1/G5]
- `credential-store`: concurrency-safe `~/.aws/credentials` and `state.json` layer — `gofrs/flock` locking, `0600`/`0700` permissions, expiry detection, and `ErrMasterExpired`/`ErrCitizenExpired` sentinels with re-login messaging. [FR11, G2/G4]
- `entra-auth`: `opsx login [--opr]` via the pluggable `SAMLProvider` seam; no-echo password (with `OPSX_PASSWORD` fallback, zeroized); `AssumeRoleWithSAML` master cache for both roles; system-proxy + context-aware MFA polling. [FR2, NFR1/NFR6/NFR7]
- `account-switching`: `opsx use <alias>` (assume citizen role → write `[<alias>.<mode>]` profile + update state) and `opsx mode admin|opr` (runtime-only mode, `--opr` override). [FR4, FR6]
- `cluster-switching`: `opsx kube <cluster-alias>` — auto-ensure creds, shell out to `aws eks update-kubeconfig` into `~/.config/opsx/kube/<cluster>.<mode>.yaml`, with per-terminal `KUBECONFIG` isolation. [FR5]
- `shell-integration`: `opsx init zsh` function generator and `opsx shell-switch …` export emitter (strict stdout = `export KEY=value` only, everything else to stderr) that delivers `AWS_PROFILE`/`KUBECONFIG`/`OPSX_MODE` to the parent shell. [FR7, FR8]
- `introspection`: `opsx status` (read-only active account/mode/cluster/expiry from state) and `opsx ls` (list configured account/cluster aliases). [FR9, FR10]

### Modified Capabilities
<!-- None — this is the initial change; no existing specs in openspec/specs/. -->

## Impact

- **New codebase** (greenfield): `cmd/opsx/main.go` + `internal/{cli,config,auth,creds,state,kube,shell,paths}`, `Makefile`, `testdata/config.example.yaml`, per the architecture's directory structure.
- **Dependencies** (verified 2026-05-30): `spf13/cobra` v1.10.x, `aws-sdk-go-v2` (`config`, `credentials`, `service/sts`), `gopkg.in/yaml.v3`, `gofrs/flock`, `golang.org/x/term`; managed via `go mod tidy`.
- **External tools / systems**: Entra SAML endpoint (HTTPS via system proxy); AWS STS (`AssumeRoleWithSAML`, `AssumeRole`); shells out to `aws eks update-kubeconfig`; `kubectl`/`helm` consume `KUBECONFIG`.
- **Filesystem**: writes `~/.aws/credentials` (profiles), `~/.config/opsx/config.yaml` (operator-authored), `~/.config/opsx/state.json`, and `~/.config/opsx/kube/*.yaml`.
- **Shell environment**: appends a one-time `opsx()` function to the user's zsh rc; exports `AWS_PROFILE`/`KUBECONFIG`/`OPSX_MODE` per terminal.
- **Company-specific risk**: isolated entirely behind the `SAMLProvider` seam (`internal/auth/entra.go`). The Entra+ADFS+MFA flow is implemented there and should be live-verified on the proxy-gated company machine against the existing Python script; everything else is testable with a fake.
