# opsx — 使用手册

`opsx` 是一个单一静态编译的 Go 二进制工具。它让云迁移运维人员**每个 master 角色每小时认证一次**
（Entra + MFA），随后即可通过简短别名在数十个 AWS citizen 账号和跨区域 EKS 集群之间秒级切换，
并支持 `admin`/`AWSOpr` 双模式并发与完整的多终端隔离。

> English version: [USAGE.md](./USAGE.md)。

---

## 1. 核心概念

| 概念 | 含义 |
|------|------|
| **Master 角色** | 你通过 Entra 联合登录的角色（`master_admin` 或 `master_AWSOpr`）。一次 `opsx login` 缓存约 1 小时。 |
| **Citizen 账号** | 由 master 角色 `AssumeRole` 进入的目标 AWS 账号。用 `opsx use <别名>` 切换。 |
| **集群（Cluster）** | 在配置中绑定到某个 citizen 账号的 EKS 集群。用 `opsx kube <别名>` 切换。 |
| **模式（Mode）** | `admin`、`opr`（`AWSOpr`），或任何你配置的额外模式。属于每个终端的运行时状态，**不写入**配置——一份配置同时服务所有模式。有效模式集由配置驱动（见 §4）。 |
| **Profile** | `opsx` 写入的 AWS 共享凭证 profile，命名为 `<别名>.<模式>.<角色>`（如 `dev.admin.Admin`）。 |

子进程无法修改父 shell 的环境变量，因此切换类命令（`use`、`kube`、`mode`）需通过一次性安装的
shell 函数生效（见 §3）。其余命令（`login`、`status`、`ls`、`logout`）作为普通二进制直接运行，无需配置。

---

## 2. 安装

```bash
make build            # 为当前平台构建 bin/opsx（CGO_ENABLED=0）
make cross            # 交叉编译 darwin/linux + Windows amd64 到 bin/
make windows          # 构建 bin/opsx-windows-amd64.exe
```

把 `bin/opsx` 放入 `PATH`。`opsx kube` 需要调用 `aws`（执行 `eks update-kubeconfig`）、`kubectl`
和 `helm`，因此这些工具必须已安装。STS 凭证本身是原生实现（无需 `aws` CLI）。

---

## 3. 一次性 Shell 集成

安装 shell 函数一次，使切换命令作用于**当前**终端：

```bash
# zsh
opsx init zsh >> ~/.zshrc && exec zsh

# Bash / Git Bash
opsx init bash >> ~/.bashrc && exec bash

# PowerShell
opsx init powershell >> $PROFILE ; . $PROFILE
```

命令提示符（`cmd.exe`）：

```bat
mkdir %USERPROFILE%\bin
opsx init cmd > %USERPROFILE%\bin\opsx.cmd
```

将 `%USERPROFILE%\bin` 放到包含 `opsx.exe` 的目录**之前**，然后新开一个命令提示符（包装脚本必须先于
`opsx.exe` 被找到）。若真实二进制不叫 `opsx.exe`，请重命名或将 `OPSX_EXE` 设为其完整路径。

`opsx init` 支持 zsh、Bash/Git Bash、PowerShell 和命令提示符。fish 等其他 shell 需对**每条**切换
命令使用手动回退：

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

## 4. 配置

```bash
mkdir -p ~/.config/opsx
cp testdata/config.example.yaml ~/.config/opsx/config.yaml
```

```yaml
accounts:
  dev:
    account_id: "111111111111"
    description: "Dev citizen account"
    region: ap-southeast-2   # 可选：该账号用于 `opsx use` 的 STS/home 区域
clusters:
  dev-syd:
    account: dev
    region: ap-southeast-2   # 用于 `aws eks update-kubeconfig` 的 EKS 区域
    name: dev-eks-cluster-01
auth:
  master_account_id: "000000000000"
  saml_provider_arn: "arn:aws:iam::000000000000:saml-provider/EntraID"
  region: us-east-1          # master AssumeRoleWithSAML 调用所用区域
  entra:
    app_id: "..."            # 登录引导所用的 Entra 联合应用 ID
    username: "operator@example.com"
  master_roles:              # 可配置——不硬编码任何角色名
    admin: master_admin
    opr:   master_AWSOpr
    prod-admin: master_production_admin   # 任何额外的键即定义一个新模式（仅需配置）
  citizen_roles:
    admin: Admin
    opr:   AWSOpr
    prod-admin: Admin
```

**模式由配置驱动。** 有效的 `--mode` 取值是 `auth.master_roles` 的键集，且必须与 `auth.citizen_roles`
的键集完全一致；两者都必须包含 `admin` 与 `opr`。只需在配置中新增一个模式（如上面的 `prod-admin`）——
`opsx login --mode prod-admin` 便会 assume `master_roles.prod-admin`，无需改动代码；Entra/SAML 登录流程
本身与角色无关（同一个公司 Entra 应用）。模式 token 必须匹配 `[A-Za-z0-9_-]+`（不含 `.`，因为模式同时用作
文件系统路径段和 profile 名分隔符）。每个模式的默认 citizen 角色是 `citizen_roles[模式]`，可用
`opsx use --role` 对单次切换覆盖（见 §5）。

**区域解析顺序**（AWS SDK 需要区域来解析 STS 端点）：

- `opsx use`：`--region` flag（若给出）→ `accounts.<别名>.region` → `auth.region` → `AWS_REGION`/`AWS_DEFAULT_REGION`
- `opsx login`：`auth.region` → `AWS_REGION`/`AWS_DEFAULT_REGION`
- `clusters.<别名>.region` 是 `update-kubeconfig` 的 EKS 区域，与 STS 区域相互独立。
- `opsx use <别名> --region <区域>` 覆盖该终端导出的会话区域（同账号、跑纯 `aws` 打不同 region）。之后 `opsx status` 会显示该区域。

可选的 Entra 端点覆盖（`auth.entra.base_url`、`ms_login_url`、`myapps_url`）默认为
`https://auth.entra.io`。设置 `auth.entra.debug: true` 可输出脱敏的 stderr 排障日志（绝不记录任何密钥）。

---

## 5. 日常使用

```bash
opsx login                 # Entra + ADFS + MFA → 缓存 master_admin（约 1 小时）
opsx login --opr           # 第二个 master 角色（master_AWSOpr）；两者共存
opsx login --mode prod-admin      # 一个由配置驱动的额外模式（assume master_roles.prod-admin）
opsx mode opr              # 设置本终端默认模式（或对每条命令用 --opr / --mode）

opsx use dev               # assume citizen 角色 → AWS_PROFILE=dev.admin.Admin（无 MFA，< 2s）
opsx use dev --role BAU    # 覆盖该模式的默认 citizen 角色 → AWS_PROFILE=dev.admin.BAU
opsx use dev --region us-west-2   # 同账号，覆盖会话 AWS_REGION，用于跑纯 `aws`
opsx kube dev-syd          # 更新 kubeconfig → 每终端 KUBECONFIG + 合并进 ~/.kube/config
opsx logout                # 清除本模式下 opsx 管理的缓存凭证/状态（--all 清除所有模式）

opsx ls                    # 列出已配置的账号与集群别名
opsx status                # 显示本终端的账号、模式、集群与过期时间
```

预获取 SAML 的备用入口（CI / 离线网络）：

```bash
export OPSX_SAML_ASSERTION_FILE=/path/to/saml-response.txt
opsx login
```

凭证过期时，命令会给出清晰提示：
`master credentials expired — run: opsx login [--opr]`。

`opsx use` 默认 assume 该模式的默认 citizen 角色（`citizen_roles[模式]`）；`--role <角色>` 对单次切换
覆盖它。`--role` 为自由取值，校验字符集 `[A-Za-z0-9._-]+`（非法值以 `invalid --role` 失败），且角色集合是
开放的——`Admin`、`AWSOpr`、`BAU`……都无需配置。`opsx kube` **不接受** `--role`：集群使用其账号在该模式下的
默认 citizen 角色。

> **Profile 命名迁移。** citizen profile 现在一律命名为 `<别名>.<模式>.<角色>`（如 `dev.admin.Admin`、
> `dev.admin.BAU`、`dev.prod-admin.Admin`），包括默认角色在内——这样 `--role` 覆盖就不会覆写另一次切换缓存的
> 凭证。早期版本遗留的旧 `<别名>.<模式>` 缓存会被孤立，但无害：opsx 凭证是短期的，会自行过期，也可运行
> `opsx logout --all` 立即清除它们（以及每个已配置模式的 profile）。master `admin`/`opr` 的 profile 名保持不变；
> 任何额外模式缓存为 `master_<模式>`。

---

## 6. 在任意 Shell 中工作（无需注入环境变量）

opsx 会写入两个**默认**位置，使普通 `aws` / `kubectl` 即使在 opsx 无法注入环境变量的场景下也能工作
（受限 ExecutionPolicy 的 PowerShell、命令提示符、或未安装 shell 函数的机器）：

### 默认 AWS profile

`opsx use` 会用刚 assume 出的 citizen 凭证覆盖 `~/.aws/credentials` 中的共享 `[default]` profile
（同时也写入 `[<别名>.<模式>.<角色>]`）。因此 `aws`/`kubectl` 通过 AWS 的默认 profile 回退机制即可指向你
最近切换的账号——无需环境变量、无需 `eval`。

- `[default]` 反映你**最近一次** `opsx use`（本身不提供多终端隔离）。
- opsx 将 `[default]` 视为由 opsx 管理并无条件覆盖。若你在其中保存了长期凭证，请先移到具名 profile。
- `opsx logout` 也会清除 `[default]`。

### 默认 kubeconfig（`~/.kube/config`）

每次 `opsx kube <别名>` **还会**通过 `aws eks update-kubeconfig` 把集群合并进 `~/.kube/config`
（在生成的 exec 块中携带 `--profile <别名>.<模式>.<角色>`）并设为 `current-context`。该 context 名用集群的
**真实 EKS 名**（`clusters.<别名>.name`），而**不是** opsx 的 friendly 别名。因此 `kubectl` 在
**没有** `KUBECONFIG`、shell 函数或 `eval` 的情况下也能指向该集群，并以集群账号身份认证。该合并是无条件的
（opsx 是本地单用户工具）。

- `~/.kube/config` 反映所有终端中**最近一次** `opsx kube`——此处不做多终端隔离。已安装 shell 函数的
  终端通过各自每 `(集群,模式)` 的 `KUBECONFIG` 保持隔离，而 `KUBECONFIG` 优先于 `~/.kube/config`，
  因此该合并纯属增量。
- context 名是真实 EKS 集群名，**并非**全局唯一：跨账号/区域同名的两个集群会在此处合并成同一个 context，
  最近一次切换覆盖之前的。需要无碰撞隔离时，请用每 `(集群,模式)` 的 `KUBECONFIG`（按 别名+模式 命名）。
- AWS CLI 自身的合并会保留你无关的 `clusters`/`contexts`/`users` 条目；opsx 不传任何破坏性参数，
  并在 `~/.kube` 缺失时创建它。
- `opsx logout` **不会**修改 `~/.kube/config`。

---

## 7. 多终端隔离原理

- Citizen 凭证以标准 `[<别名>.<模式>.<角色>]` profile 写入 `~/.aws/credentials`；每个终端导出各自的
  `AWS_PROFILE`，账号互不冲突。
- 每个集群对应 `~/.config/opsx/kube/<模式>/<编码后的集群名>.yaml`；每个终端导出各自的 `KUBECONFIG`，
  上下文互不冲突。（共享的 `~/.kube/config` 合并是“最近覆盖”，不提供隔离，仅用于无 `KUBECONFIG` 的 shell。）
- 对 `~/.aws/credentials` 和 `~/.config/opsx/state.json` 的所有写入都由 `gofrs/flock` 咨询锁保护，
  并发终端不会损坏它们。（假定本地家目录；NFS/SMB 锁语义不在保证范围内。）

---

## 8. 安全说明

- 登录密码以无回显方式读取（CI 可用 `OPSX_PASSWORD`），以 `[]byte` 持有，用后清零，绝不写入配置、
  凭证、状态或日志。
- 所有 HTTP 与 AWS 调用都遵循系统代理环境变量（`HTTP(S)_PROXY` / `NO_PROXY`）。
- 公司特定的 Entra SAML 流程被单一 `SAMLProvider` 接缝封装；请在受代理管控的公司网络中实地验证。

---

## 9. 环境变量

| 变量 | 用途 |
|------|------|
| `OPSX_CONFIG_DIR` | 覆盖 `~/.config/opsx`。 |
| `OPSX_CREDENTIALS_FILE` | 覆盖 `~/.aws/credentials`。 |
| `OPSX_DEFAULT_KUBECONFIG` | 覆盖默认 `~/.kube/config` 合并目标。 |
| `OPSX_SAML_ASSERTION` / `OPSX_SAML_ASSERTION_FILE` | 预获取 SAMLResponse 的备用入口。 |
| `OPSX_PASSWORD` | 非交互式登录密码（CI）。 |
| `OPSX_EXE` | 命令提示符包装脚本所指向的真实二进制完整路径。 |
| `AWS_REGION` / `AWS_DEFAULT_REGION` | STS 区域回退。 |

---

## 10. 已知限制（v1）

- `opsx status` 仅本地查询——它从 `state.json` 读取过期时间而不调用 AWS，因此无法在过期前察觉服务端撤销。
- `opsx kube` 需要 `PATH` 中同时存在 `aws` 与 `kubectl`。
- `opsx mode` 仅通过 shell 函数 / `shell-switch` 回退生效；裸二进制无法修改父 shell。
- 真实 Entra 流程依赖具体环境；SAML assertion 备用入口可用于离线测试与恢复。

---

## 11. 开发

```bash
make test     # go test -race -cover ./...
make lint     # gofmt + go vet + golangci-lint
make build
make windows
```
