## Why

Operators currently have to edit `config.yaml` by hand for every account or EKS cluster change, which makes routine additions and removals error-prone and provides little protection against broken references or accidental overwrites. The CLI also needs a department-controlled region allowlist, an interactive per-terminal region switch, and richer `ls` output so operators can verify the real AWS targets before acting.

## What Changes

- Add TTY-only interactive resource commands: `opsx account add|delete` and `opsx cluster add|delete`.
- Collect new resource fields through portable numbered/text prompts, validate before writing, show a final summary, and require an explicit `[y/N]` confirmation.
- Never overwrite or edit an existing account or cluster. Reject duplicate aliases and account IDs; allow two cluster aliases to identify the same real cluster with a warning.
- Refuse to delete an account referenced by clusters, list every blocking cluster alias, and require the operator to delete those clusters first. Deletion never cascades.
- Limit deletion to `config.yaml`; leave credentials, state, kubeconfig files, and other terminals untouched, with warnings for active resources and retained runtime data.
- Preserve comments, ordering, quoting, and untouched YAML content as far as a local node edit permits; serialize config changes under the shared lock and replace the file atomically. Reject advanced YAML constructs whose local mutation semantics are ambiguous.
- Add a required, ordered top-level `regions` allowlist. New account and cluster prompts select from it, while the allowlist itself remains manually managed.
- **BREAKING**: require every account to define `region`, require `regions` to be non-empty and unique, and require account, cluster, optional `auth.region`, and `opsx use --region` values to belong to the allowlist.
- Add interactive `opsx region` to select an allowed region and update only the current terminal's `AWS_REGION` and `AWS_DEFAULT_REGION`. It works without an active account and is reset by the next `opsx use` or `opsx kube`.
- Replace `opsx ls` with human-readable account and cluster tables showing account IDs, configured regions, descriptions, account references, and real EKS cluster names. The output is not a stable machine interface and does not list the region allowlist.
- Update `opsx status` to show the current terminal region even when no opsx profile is active.
- Keep config initialization/repair, editing existing resources, non-interactive mutation flags, region allowlist management, cloud-side validation, backup/undo, cascade deletion, runtime cleanup, and machine-readable `ls` output out of scope.

## Capabilities

### New Capabilities

- `interactive-config-management`: TTY-only interactive account/cluster creation and deletion, validation, confirmation, dependency protection, and safe local YAML persistence.
- `terminal-region-switching`: Interactive selection of a department-approved region and per-terminal export behavior without persisted runtime preference.

### Modified Capabilities

- `config`: Add and validate the ordered region allowlist, make account regions mandatory, and enforce allowlist membership for configured and runtime-selected regions.
- `introspection`: Expand `ls` account/cluster details and make `status` reflect a terminal region without requiring an active profile.
- `cli-foundation`: Expose the new resource-management command groups and terminal region command.
- `shell-integration`: Route the new region switch through the existing guarded, multi-shell environment-assignment path.

## Impact

- Affected packages include `internal/config`, `internal/cli`, `internal/paths`, `internal/lock`, `internal/fsutil`, and the shell integration generators/emitters.
- `config.yaml` gains a required top-level `regions` sequence and stricter region validation; existing files that omit it or omit an account region will fail validation until updated.
- Default `opsx ls` text changes incompatibly for consumers that parse its human-oriented output.
- Generated shell wrappers change for zsh, bash, PowerShell, and Command Prompt so `opsx region` can mutate the invoking terminal.
- No AWS API calls, authentication flows, credentials, state, or kubeconfig cleanup are added to config-management operations.
