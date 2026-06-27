# cluster-switching Specification

## Purpose

Switch the active EKS cluster by short alias for the current terminal's mode, generating a self-authenticating per-(cluster,mode) kubeconfig, switching the account profile too, recording the cluster in state, isolating terminals via `KUBECONFIG`, and merging into the default `~/.kube/config` for shells where environment injection is not possible.

## Requirements

### Requirement: Switch EKS cluster by alias
The system SHALL, on `opsx kube <cluster-alias>`, auto-ensure the cluster's account credentials for the current mode, then run `aws eks update-kubeconfig` for the alias's region and real cluster name, writing a per-(cluster,mode) kubeconfig under `~/.config/opsx/kube/`.

#### Scenario: Successful cluster switch
- **WHEN** `opsx kube dev-syd` is invoked
- **THEN** the cluster's account creds for the current mode are auto-ensured
- **AND** `aws eks update-kubeconfig` runs for the alias's region and real cluster name, writing a per-(cluster,mode) kubeconfig file under `~/.config/opsx/kube/`

### Requirement: Generated kubeconfig carries credentials
The system SHALL ensure the kubeconfig produced by `opsx kube` can authenticate on its own. Because `kubectl` resolves credentials via `aws eks get-token` at runtime, the system SHALL pass `--profile <alias.mode>` to `aws eks update-kubeconfig` so the generated `exec` block carries the profile (and/or export `AWS_PROFILE` alongside `KUBECONFIG`). Exporting only `KUBECONFIG` while the kubeconfig has no profile is a defect: `kubectl` has no credentials unless the user separately ran `opsx use`.

#### Scenario: kubeconfig authenticates without a separate `opsx use`
- **WHEN** `opsx kube dev-syd` completes and `kubectl` runs against the exported `KUBECONFIG`
- **THEN** the kubeconfig's `exec` user resolves credentials from the `<alias.mode>` profile
- **AND** `kubectl` obtains a token without requiring a prior `opsx use`

### Requirement: kube switches the account profile and session region too
The system SHALL, on `opsx kube <cluster-alias>`, switch the terminal's account profile and session region in addition to its `KUBECONFIG`. Because config already binds each cluster to an account, `opsx kube` SHALL emit `export AWS_PROFILE=<alias.mode>` (the cluster's account profile), `export AWS_REGION=<cluster-region>`, `export AWS_DEFAULT_REGION=<cluster-region>`, and `export KUBECONFIG=<path>`. This ensures plain `aws` commands run as the cluster's account in the cluster's configured region and that `opsx status` (which keys off `AWS_PROFILE`) can show the active account, cluster, and expiry even when `opsx kube` was run without a prior `opsx use`.

#### Scenario: kube alone sets AWS_PROFILE, region, and KUBECONFIG
- **WHEN** `opsx kube dev-syd` runs in a fresh terminal with no prior `opsx use`
- **THEN** the terminal has `AWS_PROFILE=dev.<mode>`, `AWS_REGION=<dev-syd region>`, `AWS_DEFAULT_REGION=<dev-syd region>`, and `KUBECONFIG=<dev-syd path>` exported
- **AND** `opsx status` shows the account, cluster `dev-syd`, and expiry rather than "no active context"

### Requirement: kube confirms both switches to the operator
Because `opsx kube` switches both the account profile and the cluster, the system SHALL print two human-readable confirmation lines after a successful switch: one stating the account/profile was switched, and one stating the EKS cluster was switched. This makes it explicit that the operator's `aws` identity changed too, not only `KUBECONFIG`. The confirmation MUST go to stderr (stdout stays export-only so `eval` is unaffected), and MUST appear in both the installed-shell-function path (`shell-switch kube`) and the bare `opsx kube` path.

#### Scenario: kube prints account and cluster confirmations
- **WHEN** `opsx kube dev-syd` succeeds
- **THEN** stderr contains one line confirming the account/profile switch and one line confirming the EKS cluster switch
- **AND** stdout contains only the `export` lines (no human text)

### Requirement: Cluster recorded in state
The system SHALL record the cluster alias in the state entry for the active profile when `opsx kube` succeeds, so `opsx status` can display the active cluster. The `cluster` field of the state entry MUST be populated by the real `kube` flow, not left empty.

Recording MUST succeed even when `opsx kube` reused already-valid citizen credentials and therefore did not write a fresh credential/state entry during the auto-ensure step. The cluster MUST NOT be silently dropped just because no new state entry was created by the credential step; if the state entry is absent, the `kube` flow MUST create or upsert it so the cluster is recorded.

Recording the cluster MUST NOT overwrite newer expiry/account/mode data written by a concurrent credential refresh. The read-modify-write that annotates state with the cluster MUST be coordinated under the same state write lock or use an equivalent atomic update, preserving all non-cluster fields from the latest entry.

When the cluster annotation creates a state entry that did not previously exist, the entry MUST carry the real credential expiry (derived from the citizen credentials that were just ensured for this switch) and MUST set `updated_at`. A newly-created entry MUST NOT be written with a zero/empty expiry, because `opsx status` would then report valid credentials as EXPIRED.

#### Scenario: status shows the cluster after kube
- **WHEN** `opsx kube dev-syd` succeeds and `opsx status` runs in the same terminal
- **THEN** the status output shows the active cluster `dev-syd`

#### Scenario: Cluster recorded even when credentials were reused
- **WHEN** `opsx kube dev-syd` runs while the cluster's `[<account>.<mode>]` citizen credentials are still valid
- **THEN** the cluster `dev-syd` is still recorded in the profile's state entry
- **AND** `opsx status` displays it

#### Scenario: Cluster annotation preserves concurrent state refresh
- **WHEN** `opsx kube dev-syd` records a cluster while another command refreshes the same profile's expiry/account/mode
- **THEN** the final state entry contains the cluster annotation
- **AND** it preserves the latest expiry/account/mode values rather than writing back stale data

#### Scenario: A newly-created cluster entry is not reported as expired
- **WHEN** `opsx kube dev-syd` records a cluster for a profile that had no prior state entry, while its citizen credentials are valid
- **THEN** the created state entry carries the real (non-zero) credential expiry and `updated_at`
- **AND** `opsx status` does NOT report the valid credentials as EXPIRED

### Requirement: Unambiguous kubeconfig file naming
The system SHALL derive the per-(cluster,mode) kubeconfig filename so that two distinct `(cluster alias, mode)` pairs can never map to the same file and so the alias can never escape the `~/.config/opsx/kube/` directory. The encoding MUST remain collision-free even when aliases contain `.` and MUST keep the result confined to the kube directory.

The encoding MUST also keep the resulting `KUBECONFIG` path eval-safe: the path is emitted as an `export KUBECONFIG=<path>` line and so MUST contain only characters the shell export validator accepts. The alias encoding and the export charset MUST agree, so a config-valid alias never yields a path the emitter rejects (see shell-integration "Export values are shell-safe by validation").

#### Scenario: Distinct cluster/mode pairs never share a file
- **WHEN** kubeconfig paths are derived for two different `(cluster alias, mode)` pairs
- **THEN** the two paths are distinct files
- **AND** neither path escapes `~/.config/opsx/kube/`

#### Scenario: Alias with a dot is encoded unambiguously
- **WHEN** a cluster alias contains a `.`, and a kubeconfig path is derived
- **THEN** the filename unambiguously identifies that exact `(alias, mode)` pair
- **AND** it cannot be confused with any other configured alias/mode combination

### Requirement: Per-terminal KUBECONFIG isolation
The system SHALL set a per-terminal `KUBECONFIG` pointing at the per-(cluster,mode) file so that `kubectl` and `helm` target the right cluster and terminals never collide.

#### Scenario: kubectl and helm follow the context
- **WHEN** `kubectl` or `helm` runs with the exported `KUBECONFIG`
- **THEN** the current-context is the target cluster
- **AND** helm follows automatically with no extra flags

#### Scenario: Two terminals on different clusters
- **WHEN** two terminals each run `opsx kube` for different clusters
- **THEN** each exports its own `KUBECONFIG` path
- **AND** their contexts never collide across terminals

### Requirement: Default kubeconfig merge
The system SHALL, on every `opsx kube <cluster-alias>`, also merge the cluster into the default kubeconfig at `~/.kube/config` and set it as the current context, in addition to writing the per-(cluster,mode) file under `~/.config/opsx/kube/`. This is the kubeconfig parallel of the account-switching "Default-profile overwrite": it lets `kubectl` target the cluster with no `KUBECONFIG` environment variable, shell function, or `eval`, so `opsx kube` works in shells where opsx cannot inject environment variables (Windows PowerShell under a restrictive ExecutionPolicy, Command Prompt). opsx is a local single-user tool, so this merge happens unconditionally (no flag, no config toggle).

The merge SHALL be performed via `aws eks update-kubeconfig` targeting `~/.kube/config` (so the AWS CLI's own merge preserves existing `clusters`/`contexts`/`users` entries), MUST carry `--profile <alias.mode>` in the generated exec block so `kubectl` authenticates with no `AWS_PROFILE`, and SHALL set a friendly context name (the cluster alias) as `current-context`.

Because `~/.kube/config` has a single `current-context`, it always reflects the most recent `opsx kube` across all terminals and provides no per-terminal isolation of its own. Terminals that inject `KUBECONFIG` via the installed shell function remain isolated through their per-(cluster,mode) files — that path is unchanged and `KUBECONFIG` takes precedence over `~/.kube/config`, so the merge is purely additive. opsx does NOT remove its merged context on `opsx logout` (editing structured kubeconfig YAML is out of scope), consistent with logout already not deleting per-cluster kubeconfig files.

#### Scenario: kube merges into the default kubeconfig
- **WHEN** `opsx kube dev-syd` runs
- **THEN** the cluster is merged into `~/.kube/config` with `--profile dev.<mode>` in its exec block and set as `current-context`
- **AND** `kubectl` with no `KUBECONFIG` set targets `dev-syd` and authenticates as the cluster's account

#### Scenario: default merge reflects the latest kube
- **WHEN** `opsx kube dev-syd` then `opsx kube prod-syd` run in succession
- **THEN** `~/.kube/config` current-context is `prod-syd` after the second switch

#### Scenario: unrelated kube contexts are preserved
- **WHEN** `~/.kube/config` already contains the user's own unrelated contexts and `opsx kube dev-syd` runs
- **THEN** those unrelated contexts remain intact in `~/.kube/config`

#### Scenario: function-installed shells still isolate via KUBECONFIG
- **WHEN** a terminal with the installed shell function runs `opsx kube dev-syd`
- **THEN** it exports `KUBECONFIG` to the per-(cluster,mode) file, which takes precedence over `~/.kube/config`
- **AND** the default-kubeconfig merge does not change that terminal's isolated context

### Requirement: Clear prerequisite and expiry errors
The system SHALL fail with a clear, actionable error when `aws` or `kubectl` is missing, or print the re-login message when credentials are expired.

#### Scenario: Missing tool or expired creds
- **WHEN** `opsx kube` runs but `aws` or `kubectl` is missing, or creds are expired
- **THEN** a clear error explains the missing prerequisite, or the re-login message is printed for expiry
