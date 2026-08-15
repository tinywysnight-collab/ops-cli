## Context

`opsx` currently decodes `config.yaml` directly into typed maps and treats it as operator-authored, read-only input. Account and cluster aliases are referenced throughout credentials, state, kubeconfig paths, and shell exports, while cluster entries form a strict foreign-key relationship to account aliases. The existing `ls` command renders a compact human list, and `use`/`kube`/`mode` mutate the invoking terminal through guarded per-shell wrappers.

This change makes only the `accounts` and `clusters` mappings CLI-mutable, while keeping `auth` and the new department-owned `regions` allowlist manually managed. It must preserve human-authored YAML, maintain referential integrity, remain safe across concurrent terminals, and retain stdout discipline for shell-evaluated commands.

## Goals / Non-Goals

**Goals:**

- Provide portable, TTY-only account and cluster add/delete flows with validation, numbered selection, summaries, and explicit confirmation.
- Make configuration writes local, low-diff, atomic, durable, and serialized across opsx processes.
- Establish one ordered region allowlist used by configuration validation, resource prompts, `use --region`, and interactive terminal switching.
- Improve operator visibility through detailed `ls` tables and terminal-aware `status`.
- Preserve multi-terminal isolation: region switching changes only the invoking shell, while config deletion does not clean shared runtime artifacts.

**Non-Goals:**

- Editing existing accounts or clusters, cascade deletion, allowlist management, config bootstrap/repair, backup/undo, or runtime cleanup.
- Non-interactive mutation flags, piped prompt input, fuzzy/full-screen terminal UI, JSON/YAML `ls`, or stable text-output compatibility.
- AWS API validation of accounts, clusters, regions, or permissions.
- Supporting mutation of YAML anchors, aliases, merge keys, or multiple documents.

## Decisions

### 1. Model allowed regions as an ordered top-level sequence

`Config` gains `Regions []string` mapped from top-level `regions`. Validation requires a non-empty, duplicate-free sequence whose values are non-blank region tokens without whitespace. Every account and cluster region is required and must be a member; configured `auth.region` and explicit `use --region` overrides must also be members.

The sequence order is preserved for every numbered region prompt. A slice is preferred over a set because policy owners control menu priority, while validation can build an internal membership set. Region strings are not checked against a hard-coded AWS catalog so new commercial, sovereign, or isolated partition regions do not require an opsx release.

### 2. Separate typed validation from loss-minimizing YAML mutation

Reads used by normal commands continue to decode into typed config and run full validation. Mutations additionally decode the single document into a `yaml.Node`, reject aliases/anchors/merge keys and additional documents, and locate only the target mapping node.

Add appends one mapping entry to the existing `accounts` or `clusters` node. Delete removes only the selected key/value node pair; deleting the final entry leaves an explicit empty mapping. Untouched scalar styles, comments, mapping order, and nodes remain intact where `yaml.v3` supports round-tripping. This is preferred over typed marshal, which destroys comments and ordering, and over textual splicing, which is fragile around indentation and comments. Byte-for-byte whitespace preservation is not guaranteed.

### 3. Commit configuration through a locked read-modify-validate-write transaction

The interactive phase may read a snapshot to render choices, but confirmation does not authorize writing that stale snapshot. After confirmation, the command acquires the existing shared `.opsx.lock`, rereads the latest file, repeats conflict/dependency checks, applies the node edit, decodes and validates the candidate document, then writes it using the existing durable atomic replacement helper.

This serializes concurrent opsx writers, prevents partial files, preserves configured symlink behavior, and detects a resource that changed while the user was answering prompts. Lock-free readers remain safe because replacement is atomic. The existing file permission bits are preserved. External editors do not honor the advisory lock; rereading inside the lock minimizes, but cannot eliminate, a simultaneous editor-save race.

### 4. Keep interaction behind small I/O and terminal seams

Command adapters use a prompt service parameterized by input, prompt/output writers, and a TTY predicate so behavior is unit-testable without a real terminal. Mutation commands and `opsx region` fail clearly before prompting when stdin is not a TTY.

Free-text validation loops until a valid value or cancellation. Existing resources and allowed regions use portable numbered menus rather than a new full-screen prompt dependency. Account and cluster deletion menus sort aliases; region menus retain configuration order. EOF, Ctrl-C, blank confirmation, `n`, and `no` cancel without writing and exit successfully; only `y`/`yes` commits.

### 5. Make resource lifecycle rules explicit and side-effect free

Existing aliases are never overwritten. Duplicate account IDs are rejected with the owning alias. Duplicate real cluster identity under a different alias remains schema-valid but produces a confirmation warning.

Account deletion checks reverse cluster references at commit time. Any reference aborts non-zero and lists every blocking cluster alias; no cascade option is offered. Selecting an active resource adds a warning but does not block deletion because other terminals cannot be comprehensively inspected. Successful deletion changes only `config.yaml` and states that credentials, state, kubeconfig files, and live terminal environments were retained.

### 6. Extend the existing guarded shell-switch path for region

`opsx region` is a switching command. Its prompt and confirmation text go to stderr, while its shell-switch adapter emits only assignments for `AWS_REGION` and `AWS_DEFAULT_REGION` in the selected dialect. zsh, bash/Git Bash, PowerShell, and Command Prompt wrappers add `region` to their guarded dispatch lists and keep their help/non-assignment defenses.

The menu marks the current `AWS_REGION` when it is allowed, but permits selection when no profile is active or the ambient value is outside the allowlist. The command writes no config or state. A later `use` or `kube` intentionally replaces the override with the selected resource's configured region.

### 7. Render `ls` as two stable-within-a-run human tables

Accounts are sorted by alias and show `ALIAS`, `ACCOUNT ID`, `REGION`, and `DESCRIPTION`; blank descriptions render as `-`. Clusters are sorted by alias and show `ALIAS`, `ACCOUNT`, `ACCOUNT ID`, `REGION`, and `NAME`. Empty sections retain friendly messages. The configured region allowlist is not displayed, and no compatibility or machine-readable contract is introduced.

## Risks / Trade-offs

- [YAML round-trip changes some insignificant formatting] → Test comments, order, quoting, and untouched nodes explicitly; document that indentation/trailing whitespace are not byte-stable.
- [External editor races with a confirmed CLI write] → Reread under the advisory lock immediately before mutation and keep the locked transaction short; report the residual limitation.
- [Schema tightening breaks old config files] → Emit field- and alias-specific errors and update sample/user documentation before release; rollback is the previous binary plus removal/ignoring of `regions`.
- [Interactive commands are harder to automate] → Keep non-interactive mutation explicitly out of scope and expose prompt/TTY seams for deterministic tests.
- [Deleting active configuration leaves stale runtime artifacts] → Warn before confirmation and after success; never silently clean data used by another terminal.
- [Region allowlist drifts from AWS reality] → Treat it as department policy and validate safe token structure/membership only, without network calls or a compiled AWS catalog.

## Migration Plan

1. Before using the upgraded binary, add an ordered non-empty top-level `regions` list containing every configured account, cluster, and optional auth region.
2. Ensure every account has an explicit `region`.
3. Release updated sample configuration and English/Chinese usage documentation with the binary.
4. If rollback is required, the prior binary will reject the unknown `regions` field because strict decoding is enabled; remove that field (account regions may remain) before returning to the prior version.

## Open Questions

None. Product boundaries were resolved during exploration.
