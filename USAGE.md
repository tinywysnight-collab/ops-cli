# opsx — Usage Guide

`opsx` is a single static Go binary that lets a cloud-migration operator authenticate **once per
master role per hour** with Entra + MFA, then switch between many AWS citizen accounts and
multi-region EKS clusters in seconds using short aliases — with concurrent `admin`/`AWSOpr` modes
and full multi-terminal isolation.

> 中文版见 [USAGE.zh-CN.md](./USAGE.zh-CN.md)。

---

## 1. Mental model

| Concept | What it means |
|---------|---------------|
| **Master role** | The Entra-federated role you log in as (`master_admin` or `master_AWSOpr`). One `opsx login` caches it for ~1 hour. |
| **Citizen account** | A target AWS account reached by `AssumeRole` from the master role. Switched with `opsx use <alias>`. |
| **Cluster** | An EKS cluster bound (in config) to a citizen account. Switched with `opsx kube <alias>`. |
| **Mode** | `admin` or `opr` (`AWSOpr`). Per-terminal runtime state, **not** stored in config — one config serves both modes. |
| **Profile** | The AWS shared-credentials profile `opsx` writes, named `<alias>.<mode>` (e.g. `dev.admin`). |

A child process cannot mutate its parent shell's environment, so the switching commands
(`use`, `kube`, `mode`) take effect through a one-time installed shell function (see §3). Every
other command (`login`, `status`, `ls`, `logout`) runs as a plain binary with no setup.

---

## 2. Install

```bash
make build            # builds bin/opsx for this platform (CGO_ENABLED=0)
make cross            # cross-compiles darwin/linux + Windows amd64 into bin/
make windows          # builds bin/opsx-windows-amd64.exe
```

Put `bin/opsx` on your `PATH`. `opsx` shells out to `aws` (for `eks update-kubeconfig`),
`kubectl`, and `helm`, so those must be installed for `opsx kube` to work. STS itself is native
(no `aws` CLI needed for credentials).

---

## 3. One-time shell integration

Install the shell function once so switching commands update your **current** terminal:

```bash
# zsh
opsx init zsh >> ~/.zshrc && exec zsh

# Bash / Git Bash
opsx init bash >> ~/.bashrc && exec bash

# PowerShell
opsx init powershell >> $PROFILE ; . $PROFILE
```

Command Prompt (`cmd.exe`):

```bat
mkdir %USERPROFILE%\bin
opsx init cmd > %USERPROFILE%\bin\opsx.cmd
```

Put `%USERPROFILE%\bin` **before** the directory holding `opsx.exe` on `PATH`, then open a new
Command Prompt (the wrapper must be found before `opsx.exe`). If the real binary is not named
`opsx.exe`, rename it or set `OPSX_EXE` to its full path.

`opsx init` supports zsh, Bash/Git Bash, PowerShell, and Command Prompt. For fish and other shells,
use the manual fallback for **every** switching command:

```bash
eval "$(opsx shell-switch use dev)"
```

```powershell
opsx shell-switch --shell powershell use dev | ForEach-Object { Invoke-Expression $_ }
```

```bat
for /f "delims=" %L in ('opsx shell-switch --shell cmd use dev') do %L
```

---

## 4. Configure

```bash
mkdir -p ~/.config/opsx
cp testdata/config.example.yaml ~/.config/opsx/config.yaml
```

```yaml
accounts:
  dev:
    account_id: "111111111111"
    description: "Dev citizen account"
    region: ap-southeast-2   # optional: this account's STS/home region for `opsx use`
clusters:
  dev-syd:
    account: dev
    region: ap-southeast-2   # EKS region for `aws eks update-kubeconfig`
    name: dev-eks-cluster-01
auth:
  master_account_id: "000000000000"
  saml_provider_arn: "arn:aws:iam::000000000000:saml-provider/EntraID"
  region: us-east-1          # region for the master AssumeRoleWithSAML call
  entra:
    app_id: "..."            # Entra federated application ID used by the login bootstrap
    username: "operator@example.com"
  master_roles:              # configurable — no role name is hardcoded
    admin: master_admin
    opr:   master_AWSOpr
  citizen_roles:
    admin: Admin
    opr:   AWSOpr
```

**Region resolution** (AWS SDK needs a region to resolve the STS endpoint):

- `opsx use`: `--region` flag (if given) → `accounts.<alias>.region` → `auth.region` → `AWS_REGION`/`AWS_DEFAULT_REGION`
- `opsx login`: `auth.region` → `AWS_REGION`/`AWS_DEFAULT_REGION`
- `clusters.<alias>.region` is the EKS region for `update-kubeconfig`, distinct from STS region.
- `opsx use <alias> --region <region>` overrides the exported session region for that terminal (same account, different region for plain `aws`). `opsx status` then shows that region.

Optional Entra endpoint overrides (`auth.entra.base_url`, `ms_login_url`, `myapps_url`) default to
`https://auth.entra.io`. Set `auth.entra.debug: true` for sanitized stderr troubleshooting logs
(no secrets are ever logged).

---

## 5. Daily usage

```bash
opsx login                 # Entra + ADFS + MFA → cache master_admin (~1h)
opsx login --opr           # second master role (master_AWSOpr); both coexist
opsx mode opr              # set this terminal's default mode (or use --opr per command)

opsx use dev               # assume citizen role → AWS_PROFILE=dev.admin (no MFA, < 2s)
opsx use dev --region us-west-2   # same account, override the session AWS_REGION for plain `aws`
opsx kube dev-syd          # update kubeconfig → per-terminal KUBECONFIG + merge into ~/.kube/config
opsx logout                # purge this mode's opsx-managed cached credentials/state

opsx ls                    # list configured account & cluster aliases
opsx status                # show this terminal's account, mode, cluster, and expiry
```

Pre-obtained SAML escape hatch (CI / off-network):

```bash
export OPSX_SAML_ASSERTION_FILE=/path/to/saml-response.txt
opsx login
```

When credentials expire, commands fail with a clear hint:
`master credentials expired — run: opsx login [--opr]`.

---

## 6. Working in any shell (no env injection needed)

opsx writes two **default** locations so plain `aws` / `kubectl` work even where opsx cannot
inject environment variables (PowerShell under a restrictive ExecutionPolicy, Command Prompt, or
machines without the shell function):

### Default AWS profile

`opsx use` overwrites the shared `[default]` profile in `~/.aws/credentials` with the
freshly-assumed citizen credentials (in addition to `[<alias>.<mode>]`). So `aws`/`kubectl`
target the account you last switched to via AWS's default-profile fallback — no env var, no `eval`.

- `[default]` reflects your **most recent** `opsx use` (no per-terminal isolation on its own).
- opsx treats `[default]` as opsx-managed and overwrites it unconditionally. Move any long-term
  credentials you keep there to a named profile first.
- `opsx logout` clears `[default]` too.

### Default kubeconfig (`~/.kube/config`)

Every `opsx kube <alias>` **also** merges the cluster into `~/.kube/config` via
`aws eks update-kubeconfig` (carrying `--profile <alias>.<mode>` in the generated exec block) and
sets it as `current-context`. The context is named by the cluster's **real EKS name**
(`clusters.<alias>.name`), not the friendly opsx alias. So `kubectl` targets the cluster — and
authenticates as the cluster's account — with **no** `KUBECONFIG`, shell function, or `eval`. The
merge is unconditional (opsx is a local single-user tool).

- `~/.kube/config` reflects your **most recent** `opsx kube` across all terminals — no per-terminal
  isolation there. Function-installed terminals stay isolated via their per-`(cluster,mode)`
  `KUBECONFIG`, which takes precedence over `~/.kube/config`, so the merge is purely additive.
- The context name is the real EKS cluster name, which is **not** globally unique: two clusters that
  share a real name (across accounts/regions) collapse to one context here, latest-switch-wins. Use
  the per-`(cluster,mode)` `KUBECONFIG` (keyed by alias+mode) when you need collision-free contexts.
- The AWS CLI's own merge preserves your unrelated `clusters`/`contexts`/`users`; opsx passes no
  destructive flag and creates `~/.kube` if missing.
- `opsx logout` does **not** edit `~/.kube/config`.

---

## 7. How per-terminal isolation works

- Citizen creds are standard `[<alias>.<mode>]` profiles in `~/.aws/credentials`; each terminal
  exports its own `AWS_PROFILE`, so accounts never collide.
- Each cluster gets `~/.config/opsx/kube/<mode>/<encoded-cluster>.yaml`; each terminal exports its
  own `KUBECONFIG`, so contexts never collide. (The shared `~/.kube/config` merge is latest-wins
  and provides no isolation — it exists only for shells with no `KUBECONFIG`.)
- All writes to `~/.aws/credentials` and `~/.config/opsx/state.json` are guarded by a `gofrs/flock`
  advisory lock, so concurrent terminals cannot corrupt them. (Assumes a local home directory;
  NFS/SMB lock semantics are out of scope.)

---

## 8. Security notes

- The login password is read with no echo (or from `OPSX_PASSWORD` for CI), held as `[]byte`,
  zeroized after use, and never written to config, credentials, state, or logs.
- All HTTP and AWS calls honor system proxy env vars (`HTTP(S)_PROXY` / `NO_PROXY`).
- The company-specific Entra SAML flow lives behind a single `SAMLProvider` seam; live-verify it on
  the proxy-gated company network.

---

## 9. Environment variables

| Variable | Purpose |
|----------|---------|
| `OPSX_CONFIG_DIR` | Override `~/.config/opsx`. |
| `OPSX_CREDENTIALS_FILE` | Override `~/.aws/credentials`. |
| `OPSX_DEFAULT_KUBECONFIG` | Override the default `~/.kube/config` merge target. |
| `OPSX_SAML_ASSERTION` / `OPSX_SAML_ASSERTION_FILE` | Pre-obtained SAMLResponse escape hatch. |
| `OPSX_PASSWORD` | Non-interactive login password (CI). |
| `OPSX_EXE` | Full path to the real binary for the Command Prompt wrapper. |
| `AWS_REGION` / `AWS_DEFAULT_REGION` | Fallback STS region. |

---

## 10. Known limitations (v1)

- `opsx status` is local-only — it reads expiry from `state.json` and does not call AWS, so it
  cannot detect a server-side revocation before expiry.
- `opsx kube` requires both `aws` and `kubectl` present in `PATH`.
- `opsx mode` only takes effect through the shell function / `shell-switch` fallback; the bare
  binary cannot mutate its parent shell.
- The real Entra flow is environment-dependent; the SAML assertion escape hatch covers off-network
  testing and recovery.

---

## 11. Development

```bash
make test     # go test -race -cover ./...
make lint     # gofmt + go vet + golangci-lint
make build
make windows
```
