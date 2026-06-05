# PRD — `opsx`: Fast AWS Account & EKS Cluster Switcher

**Author:** John (PM) with Tiny
**Date:** 2026-05-30
**Status:** Draft v1
**Language stack:** Go (single static binary)

---

## 1. Problem Statement

During an ongoing cloud migration, the operator works across an AWS Organization with one master account (Entra / Azure AD SAML federation) and **many citizen accounts**, each potentially spanning **multiple regions and multiple EKS clusters**. Daily work requires constant context switching between these accounts and clusters.

The current workflow uses a Python script that, on **every switch**:

1. Authenticates to Entra, obtaining a SAML assertion (requires interactive MFA each time).
2. Assumes the master role (`master_admin` **or** `master_AWSOpr`).
3. Assumes the target citizen account role (`Admin` for `master_admin`, `AWSOpr` for `master_AWSOpr`).
4. Writes the resulting STS token into the **single `default` profile**.

This causes real, repeated friction:

- **Repeated MFA.** Every account switch re-runs the full Entra login, forcing another MFA approval — even though master STS credentials are valid for a full hour.
- **Single shared profile collision.** Writing to one `default` profile means multiple terminals overwrite each other's credentials. A command in a "non-prod" terminal can silently hit prod.
- **No cluster switching.** Selecting an EKS cluster is a manual `aws eks update-kubeconfig` dance with the right region and full cluster name; `kubectl`'s global current-context also collides across terminals.
- **No aliases.** The operator must remember 12-digit account IDs, exact cluster names, and regions.
- **No concurrent modes.** Operating non-prod as `admin` and prod as `AWSOpr` at the same time is not cleanly supported.

**Opportunity:** authenticate once per master role per hour, then switch between accounts and clusters in seconds using short memorable aliases — with full multi-terminal isolation.

---

## 2. Goals & Value

- One MFA per master role per hour, instead of one per switch.
- Sub-second switching between accounts and between clusters using short aliases.
- Safe concurrent use across multiple terminals (no credential collision).
- Concurrent `admin` and `AWSOpr` sessions, selectable per terminal.
- A single declarative YAML the operator (and teammates) can edit to onboard.

---

## 3. Authentication Model (reference)

```
opsx login        → Entra + ADFS + interactive MFA → SAMLResponse
                  → AssumeRoleWithSAML(master_admin)   → cache [master_admin]   (1h)
opsx login --opr  → Entra + ADFS + interactive MFA → SAMLResponse
                  → AssumeRoleWithSAML(master_AWSOpr)  → cache [master_awsopr]  (1h)

opsx use <acct>   → from cached master STS creds → AssumeRole(citizen Admin|AWSOpr)
                    → write profile [<acct>.<mode>] (1h) → inject AWS_PROFILE into current terminal
```

- `master_admin` and `master_AWSOpr` are **separate logins** (each its own MFA); both may be cached and active simultaneously.
- The Entra + ADFS + MFA automation is implemented behind the `SAMLProvider` seam. v1 still accepts a pre-obtained SAMLResponse/assertion via `OPSX_SAML_ASSERTION` or `OPSX_SAML_ASSERTION_FILE` as an escape hatch for CI, off-network testing, and recovery.
- Both master and citizen STS credentials expire after **1 hour**.

---

## 4. Configuration Schema

```yaml
# ~/.config/opsx/config.yaml

accounts:
  dev:                          # alias used on the command line
    account_id: "111111111111"
    description: "Dev citizen account"
    region: ap-southeast-2      # optional STS/home region for this account
  prod:
    account_id: "222222222222"
    description: "Prod citizen account"

clusters:
  dev-syd:                      # cluster alias
    account: dev                # reference to an accounts entry
    region: ap-southeast-2
    name: dev-eks-cluster-01    # real EKS cluster name (for update-kubeconfig)
  dev-tok:
    account: dev
    region: ap-northeast-1
    name: dev-eks-tokyo
  prod-syd:
    account: prod
    region: ap-southeast-2
    name: prod-eks-cluster-01

auth:
  master_account_id: "000000000000"
  saml_provider_arn: "arn:aws:iam::000000000000:saml-provider/EntraID"
  region: us-east-1             # master STS region; default for citizen STS
  entra:
    app_id: "..."               # Entra federated application ID
    username: "operator@example.com"
  master_roles:
    admin: master_admin
    opr:   master_AWSOpr
  citizen_roles:
    admin: Admin
    opr:   AWSOpr
```

- `admin` vs `AWSOpr` mode is **not** stored here — it is a per-terminal runtime property (one config serves both modes).
- `auth.region` is the master STS region and default citizen STS region; `accounts.<alias>.region` optionally overrides citizen STS for that account. These are distinct from each cluster's EKS region.
- `auth.entra.base_url`, `auth.entra.ms_login_url`, and `auth.entra.myapps_url` are optional endpoint overrides and default to `https://auth.entra.io`; they are intentionally left out of the sample config unless an environment needs non-default Entra endpoints. The login flow uses top-level `auth.entra.app_id`; older `tenant_id` / `domain_map` config entries are no longer part of the schema.
- Citizen profiles are named `[<alias>.<mode>]` (e.g. `[dev.admin]`, `[prod.opr]`) so the same account can be active in both modes without overwriting.

---

## 5. Must-Have Features (v1)

| # | Feature | Detail |
|---|---------|--------|
| M1 | **Single-binary Go CLI** (`opsx`) | No runtime dependency; primary target macOS, also Linux; Windows amd64 binary target is available as `opsx-windows-amd64.exe`. |
| M2 | **`opsx login [--opr]`** | Run the Entra + ADFS + interactive MFA flow behind the auth seam, extract the SAMLResponse, `AssumeRoleWithSAML` into `master_admin` (default) or `master_AWSOpr` (`--opr`), and cache master STS creds to `[master_admin]` / `[master_awsopr]`. Both can coexist. `OPSX_SAML_ASSERTION` / `OPSX_SAML_ASSERTION_FILE` remain accepted escape hatches. |
| M3 | **YAML config** | `accounts`, `clusters`, and `auth` sections per §4; loaded from `~/.config/opsx/config.yaml`. |
| M4 | **`opsx use <account-alias>`** | From cached master creds, assume citizen role for the **current terminal's mode**; write `[<alias>.<mode>]` profile; inject `AWS_PROFILE` into the current terminal. |
| M5 | **`opsx kube <cluster-alias>`** | Auto-ensure the cluster's account creds are ready (current mode); run `aws eks update-kubeconfig` for the alias's region + real name into `~/.config/opsx/kube/<mode>/<encoded-cluster>.yaml` with `--profile <alias.mode>`; export both `AWS_PROFILE` and per-terminal `KUBECONFIG`. `helm` follows the same context automatically. |
| M6 | **`opsx mode admin\|opr`** | Set the current terminal's default mode; per-command flags (e.g. `--opr`) override. |
| M7 | **Per-terminal isolation** | Both `AWS_PROFILE` and `KUBECONFIG` are scoped to each terminal — two terminals never collide. |
| M8 | **Shell integration** | See note below. `opsx init <shell>` installs a one-time shell function; `opsx shell-switch …` emits shell-specific environment assignment statements consumed by the parent shell. Daily usage is transparent. A manual fallback is also supported for portability. |
| M9 | **`opsx status`** | Show the current terminal's active account, mode, cluster, and credential expiry. |
| M10 | **`opsx ls`** | List configured account and cluster aliases. |
| M11 | **Clear expiry errors** | When master/citizen creds are expired, fail with a clear message prompting `opsx login`. |
| M12 | **`opsx logout [--all]`** | Purge opsx-managed cached credentials/state for the selected mode or all modes without hand-editing `~/.aws/credentials`; preserve unrelated profiles/comments. |

### Note on M8 — why shell integration is required

A child process cannot modify its parent shell's environment. When `opsx use dev` runs, `opsx` is a subprocess: it can set `AWS_PROFILE` in its own process, but that value disappears when the command exits — the parent terminal is unchanged, so the switch would not take effect. This is the same constraint every switcher tool faces (`kubectx`, `aws-vault`, `nvm`, `pyenv`, `direnv`).

**Recommended path (transparent daily use).** A shell function is installed **once** via `opsx init <shell>` (e.g. `opsx init zsh`), which appends to the rc file:

```bash
# Simplified shape; the generated function also handles leading global flags
# and returns non-zero when shell-switch fails.
opsx() {
  local subcmd="$1"
  case "$subcmd" in
    use|kube|mode)
      local out
      out="$(command opsx shell-switch "$@")" || return $?
      eval "$out"
      ;;
    *)
      command opsx "$@"
      ;;
  esac
}
```

`opsx shell-switch use dev` prints `export AWS_PROFILE=dev.admin` (and the per-terminal `KUBECONFIG` for `kube`), and `eval` applies it to the **current** shell. After this one-time setup the user only ever types `opsx use dev` / `opsx kube dev-syd` and never deals with `eval`.

The installed function must also handle leading global flags such as `opsx --mode opr use dev` and must return non-zero if `shell-switch` fails; a failed switch must not look successful just because `eval` received empty output.

**Manual fallback (portability across shells/platforms).** Where installing the function is undesirable (other shells, restricted machines, scripts), the same effect is achieved manually:

```bash
eval "$(opsx shell-switch use dev)"
# or apply the printed export lines directly:
export AWS_PROFILE=dev.admin
```

`opsx shell-switch` therefore always emits plain, copy-pasteable `export` lines for POSIX-style shells so the tool works even with no shell integration installed. v1 ships function generators for **zsh** and **PowerShell**; PowerShell uses the distinct `--shell powershell` dialect and emits `$env:` assignment lines. bash/fish use the manual fallback until dedicated generators land (N7).

---

## 6. Nice-to-Have Features (later / v2)

| # | Feature | Notes |
|---|---------|-------|
| N1 | **Background token refresh (v2)** | Detect impending expiry; proactively prompt re-login (MFA) and refresh active citizen credentials so commands don't fail mid-task. |
| N2 | **Account / cluster auto-discovery** | Populate or validate config from AWS Org account list and `aws eks list-clusters`. |
| N3 | **Per-account EKS default region** | Reduce repetition in cluster entries. This is distinct from the v1 account STS/home `region`, which already exists for `AssumeRole`. |
| N4 | **Shell completion** | zsh/bash/fish completion for aliases. |
| N5 | **Interactive selector when alias omitted** | If `opsx use` or `opsx kube` is run with **no alias**, list all configured accounts / clusters and let the operator pick interactively (arrow-key / fuzzy-filter list). Selecting an item performs the same switch as passing its alias. |
| N6 | **`opsx exec <alias> -- <cmd>`** | Run a one-off command in an account/cluster context without changing the current terminal. |
| N7 | **bash/fish shell support** | Beyond zsh. |

---

## 7. Success Criteria

- **MFA frequency:** ≤ 1 MFA per master role per hour (typically ≤ 2/hour total), down from one MFA per switch.
- **Switch speed:** switching an account or a cluster completes in **< 2 seconds** once logged in.
- **Ergonomics:** every switch is done via a short alias — no account IDs, cluster names, or regions typed manually.
- **Concurrency safety:** two terminals can simultaneously operate different accounts/modes (e.g. non-prod `admin` + prod `AWSOpr`) with **zero** credential collision — validated by a deliberate cross-terminal test.
- **Adoption:** the operator uses `opsx` as the daily driver and retires the legacy Python script.
- **Onboarding:** a teammate can become productive by editing only `config.yaml`.

---

## 8. Out of Scope (v1)

- Automatic/background token refresh and any long-running daemon (→ N1 / v2).
- Any Kubernetes other than EKS; ECS or other compute context switching.
- Creating/managing IAM roles, accounts, or AWS Org structure.
- Encryption of credentials at rest beyond the existing `~/.aws/credentials` file permissions.
- Auto-populating config from AWS Org / EKS discovery (→ N2).
- Windows-first UX polish beyond the Windows amd64 binary and PowerShell shell integration shipped in v1.
- Any GUI.

---

## 9. Open Assumptions to Validate

- The implemented Entra SAML assertion flow must be live-validated on the proxy-gated company machine against the legacy Python script's endpoints/parameters, especially the interactive MFA number-match branch. Off-network verification may continue to use `OPSX_SAML_ASSERTION` / `OPSX_SAML_ASSERTION_FILE`.
- `aws`, `kubectl`, and `helm` are present on the target (company) machines; `opsx` shells out to `aws eks update-kubeconfig` rather than reimplementing it.
- Citizen role names are consistent (`Admin` / `AWSOpr`) across accounts, derived from the master mode rather than configured per account.
