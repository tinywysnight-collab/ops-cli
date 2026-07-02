# opsx — fast AWS account & EKS cluster switcher

`opsx` is a single static Go binary for cloud-migration operators working across one
Entra/Azure AD master account and many AWS citizen accounts spanning multiple regions and EKS
clusters. Authenticate **once per master role per hour**, then switch accounts and clusters in
seconds by short alias — with full multi-terminal isolation and concurrent `admin`/`AWSOpr`
modes.

> 📖 Full usage guide: [USAGE.md](./USAGE.md) · 完整使用手册（中文）：[USAGE.zh-CN.md](./USAGE.zh-CN.md)

## Install

```bash
make build            # builds bin/opsx for this platform (CGO_ENABLED=0)
make cross            # cross-compiles darwin/linux plus Windows amd64 into bin/
make windows          # builds bin/opsx-windows-amd64.exe
```

Put `bin/opsx` on your `PATH`. `opsx` shells out to `aws` (for `eks update-kubeconfig`),
`kubectl`, and `helm`, so those must be installed for `opsx kube` to work. STS itself is native
(no `aws` CLI needed for credentials).

## One-time shell integration

A child process can't change its parent shell's environment, so install the shell function once:

```bash
opsx init zsh >> ~/.zshrc      # adds: opsx() { eval "$(command opsx shell-switch "$@")"; }
exec zsh                       # reload
```

Git Bash / Bash:

```bash
opsx init bash >> ~/.bashrc
exec bash
```

PowerShell:

```powershell
opsx init powershell >> $PROFILE
. $PROFILE
```

Command Prompt (`cmd.exe`):

```bat
mkdir %USERPROFILE%\bin
opsx init cmd > %USERPROFILE%\bin\opsx.cmd
```

Put `%USERPROFILE%\bin` before the directory containing `opsx.exe` on `PATH`, then open a new
Command Prompt. The wrapper must be found before `opsx.exe`, because `cmd.exe` normally prefers
`.exe` over `.cmd` within the same directory. If the real binary is not named `opsx.exe` (for
example `opsx-windows-amd64.exe`), either rename it or set `OPSX_EXE` to its full path before using
the wrapper.

After this, `opsx use` / `opsx kube` / `opsx mode` transparently update your current terminal.
`opsx init` supports zsh, Bash/Git Bash, PowerShell, and Command Prompt. fish and other shells must
use the manual fallback below for every switching command.
(`opsx use`, `opsx kube`, and `opsx mode` are the switching commands; all other commands run normally.)

Without the function (other shells, restricted machines), use the manual fallback:

```bash
eval "$(opsx shell-switch use dev)"
```

PowerShell manual fallback:

```powershell
opsx shell-switch --shell powershell use dev | ForEach-Object { Invoke-Expression $_ }
```

Command Prompt manual fallback:

```bat
for /f "delims=" %L in ('opsx shell-switch --shell cmd use dev') do %L
```

## Configure

Copy the example and edit for your environment (see `testdata/config.example.yaml`):

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
  master_roles:    # configurable — no role name is hardcoded
    admin: master_admin
    opr:   master_AWSOpr
  citizen_roles:
    admin: Admin
    opr:   AWSOpr
```

Mode (`admin` vs `opr`) is **not** stored — it is per-terminal runtime state, so one config
serves both modes.

**STS region is configuration-driven.** AWS SDK Go v2 needs a region to resolve the STS
endpoint, so `opsx login`/`opsx use` would otherwise fail on a machine with no `AWS_REGION`
and no `~/.aws/config` default. Region resolution order is: for `opsx use`,
`accounts.<alias>.region` → `auth.region` → `AWS_REGION`/`AWS_DEFAULT_REGION`; for `opsx login`,
`auth.region` → `AWS_REGION`/`AWS_DEFAULT_REGION`. If none resolves, the command fails with a
clear message naming the field to set. `clusters.<alias>.region` is unchanged — it is the EKS
region passed to `aws eks update-kubeconfig`, distinct from an account's STS/home region.
When the shell integration is installed, `opsx use` also sets `AWS_REGION` and
`AWS_DEFAULT_REGION` to the account's resolved region for the current terminal. `opsx kube` sets
them to the active cluster's region, so subsequent `aws` commands in that session do not need a
manual `--region`.

Optional endpoint override fields `auth.entra.base_url`, `auth.entra.ms_login_url`, and
`auth.entra.myapps_url` default to `https://auth.entra.io` when omitted. They are intentionally
left out of the sample config unless an environment needs non-default Entra endpoints. The login
flow uses top-level `auth.entra.app_id`; older `tenant_id` / `domain_map` config entries are no
longer part of the schema.

For live Entra troubleshooting, set optional `auth.entra.debug: true`. Debug logs go to stderr and
only include sanitized step, endpoint, status, and count information; passwords, cookies, SAML
assertions, MFA flow tokens, session IDs, and canary values are not logged. This field is omitted
from the sample config because the default is quiet.

## Daily usage

```bash
opsx login                 # Entra + ADFS + MFA → cache master_admin (1h)
export OPSX_SAML_ASSERTION_FILE=/path/to/saml-response.txt
opsx login                 # optional pre-obtained SAMLResponse escape hatch
opsx login --opr          # second master role (master_AWSOpr); both coexist
opsx mode opr             # set this terminal's default mode (or use --opr per command)

opsx use dev              # assume citizen role → AWS_PROFILE=dev.admin + overwrite [default]  (no MFA, < 2s)
opsx kube dev-syd         # update kubeconfig → per-terminal KUBECONFIG + merge into ~/.kube/config
opsx logout               # purge this mode's opsx-managed cached credentials/state

opsx ls                   # list configured account & cluster aliases
opsx status               # show this terminal's account, mode, cluster, and expiry
```

When credentials expire, commands fail with a clear hint (`master credentials expired — run:
opsx login [--opr]`).

> **Note:** `opsx login` now drives the Entra + ADFS + interactive MFA flow through
> `internal/auth/entra.go`, then uses the real AWS `AssumeRoleWithSAML` path and caches master
> credentials. `OPSX_SAML_ASSERTION` / `OPSX_SAML_ASSERTION_FILE` remain as an accepted escape
> hatch for CI, off-network testing, or emergency bypass. Because the HTTP flow is company-specific,
> keep it live-verified on the proxy-gated company machine against the legacy Python baseline before
> broad rollout. The login command asks the Entra provider for the SAML assertion before building
> the AWS STS client, so the password prompt is not delayed by AWS SDK config/credential discovery.

## Default profile (works in any shell, no setup)

`opsx use` also overwrites the shared `[default]` profile in `~/.aws/credentials` with the
freshly-assumed citizen credentials (in addition to `[<alias>.<mode>]`). So even where opsx
cannot inject `AWS_PROFILE` into your shell — Windows PowerShell under a restrictive
ExecutionPolicy (where `$PROFILE` and `.ps1` won't run), Command Prompt, or any machine without
the shell function installed — plain `aws` / `kubectl` still target the account you last switched
to, via AWS's default-profile fallback. No env var, no `eval`, no `init` required.

- `[default]` always reflects your **most recent** `opsx use` (it is a single shared file
  section), so it does **not** provide per-terminal isolation on its own. Terminals that inject
  `AWS_PROFILE` via the installed shell function stay isolated through their `[<alias>.<mode>]`
  profiles — that path is unchanged.
- opsx treats `[default]` as opsx-managed and overwrites it unconditionally. If you keep your own
  long-term credentials in `[default]`, move them to a named profile first.
- `opsx logout` clears `[default]` too.

The same idea applies to clusters. Every `opsx kube <alias>` also merges the cluster into the
default kubeconfig at `~/.kube/config` (via `aws eks update-kubeconfig`, which carries
`--profile <alias>.<mode>` in the generated exec block) and sets it as `current-context`. The
context is named by the cluster's **real EKS name** (`clusters.<alias>.name`), not the friendly
opsx alias. So `kubectl` targets the cluster — and authenticates as the cluster's account — with
**no** `KUBECONFIG`, shell function, or `eval`, covering locked-down PowerShell / Command Prompt.
The merge is unconditional (opsx is a local single-user tool; no flag, no config toggle).

- `~/.kube/config` always reflects your **most recent** `opsx kube` across all terminals, so it
  provides **no** per-terminal isolation on its own. Terminals that inject `KUBECONFIG` via the
  installed shell function stay isolated through their per-`(cluster,mode)` files — that path is
  unchanged, and `KUBECONFIG` takes precedence over `~/.kube/config`, so the merge is purely additive.
- The context name (the real EKS cluster name) is **not** globally unique: two clusters sharing a
  real name across accounts/regions collapse to one context here, latest-switch-wins. The
  per-`(cluster,mode)` `KUBECONFIG` (keyed by alias+mode) remains collision-free for isolation.
- The AWS CLI's own merge preserves your unrelated `clusters` / `contexts` / `users` entries; opsx
  passes no destructive flag and creates `~/.kube` if it does not exist.
- `opsx logout` does **not** edit `~/.kube/config` (editing structured kubeconfig YAML is out of
  scope), consistent with logout already not deleting per-cluster kubeconfig files.

## How isolation works

- Citizen creds are written as standard `[<alias>.<mode>]` profiles in `~/.aws/credentials`;
  each terminal exports its own `AWS_PROFILE`, so accounts never collide.
- Each cluster gets its own `~/.config/opsx/kube/<mode>/<encoded-cluster>.yaml`; each terminal exports
  its own `KUBECONFIG`, so contexts never collide. `opsx kube` additionally merges the cluster into
  the shared `~/.kube/config` (latest-switch-wins, no isolation there) so `kubectl` works even with
  no `KUBECONFIG` set.
- All writes to `~/.aws/credentials` and `~/.config/opsx/state.json` are guarded by a `gofrs/flock`
  advisory lock, so concurrent terminals can't corrupt them.
- Reads are lock-free but see whole-file atomic snapshots. Cross-file consistency is bounded by
  the rule that opsx does not trust cached STS credentials without a matching state entry and
  recorded expiry.
- The advisory lock assumes a local home directory. Network filesystems such as NFS/SMB can have
  weaker or surprising lock semantics, so the multi-terminal guarantee is scoped to local storage.

For different Entra users in different terminals, use separate opsx/AWS storage roots per session:
set `OPSX_CONFIG_DIR`, `OPSX_CREDENTIALS_FILE`, and, when needed, `OPSX_DEFAULT_KUBECONFIG`
before running `opsx login`. A shared credentials file is account/mode-isolated, not user-isolated:
profiles such as `[master_admin]` and `[dev.admin]` are intentionally stable names and would be
overwritten by another user using the same storage files.

## Security notes

- The login password is read with no echo (or from `OPSX_PASSWORD` for CI), held as `[]byte`,
  zeroized after use, and never written to config, credentials, state, or logs.
- All HTTP and AWS calls honor system proxy env vars (`HTTP(S)_PROXY` / `NO_PROXY`); no proxy is
  ever hardcoded.
- The company-specific Entra SAML flow lives behind a single `SAMLProvider` seam
  (`internal/auth/entra.go`); command orchestration is testable with a fake provider, and the real
  HTTP/MFA path should be live-verified in the company network.

## Known limitations (v1)

These are acknowledged, deliberate scope decisions for v1; refinements are planned for v1.1:

- **`opsx status` is local-only.** It reports credential validity from the recorded expiry in
  `state.json` and does not call AWS to verify the session, so it cannot detect a credential
  revoked server-side before its expiry. A lightweight, opt-in `GetCallerIdentity` check is
  planned for v1.1.
- **`opsx kube` requires both `aws` and `kubectl`.** The prerequisite check fails if either is
  missing, even for operators who only use `helm` or a GUI. Making the `kubectl` check lazy
  (only `aws` is strictly needed for the switch) is planned for v1.1.
- **`opsx mode` only takes effect through the shell function or shell-switch fallback.** Run
  outside the installed `opsx` function (i.e. the bare binary), `opsx mode opr` merely prints an
  environment assignment line and changes nothing in the current shell, because a child process
  cannot mutate its parent's environment. Use the installed zsh/Bash/PowerShell function,
  Command Prompt wrapper, `eval "$(opsx shell-switch mode opr)"`, or the shell-specific fallback above.
- **Real Entra flow is environment-dependent.** The HTTP/MFA flow is implemented, but its exact
  request chain depends on the company's Entra/ADFS pages and proxy-gated network. The env/file
  SAML assertion escape hatch remains supported for off-network testing and recovery.

## Development

```bash
make test     # go test -race -cover ./...
make lint     # gofmt + go vet + golangci-lint
make build
make windows
```
