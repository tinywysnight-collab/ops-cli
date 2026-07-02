# Design: config-driven master modes + `opsx use --role` citizen override

Date: 2026-07-02
Status: proposed (awaiting review)

## Problem

Today `mode` (`admin`/`opr`) drives **both** the master role assumed at `opsx login`
and the citizen role assumed at `opsx use`, in strict 1:1 lockstep:

- `MasterRoleARN(mode)` = `arn:aws:iam::{master_account_id}:role/{master_roles[mode]}`
- `CitizenRoleARN(alias, mode)` = `arn:aws:iam::{accounts[alias].account_id}:role/{citizen_roles[mode]}`

`NormalizeMode`, `supportedModes`, and `MasterProfile` hardcode exactly two modes.

We need to:

1. Add a **third master role** (a "production admin" login identity), selectable at
   login — and more generally allow master roles to grow without code changes.
2. Let the **citizen role vary independently** of the mode at `opsx use` time, from a
   set of citizen roles that is no longer fixed at two (`Admin`, `AWSOpr`, `BAU`, …).

## Goals

- `opsx login --mode prod-admin` assumes a third, config-defined master role.
- The set of valid modes is **config-driven** (the keys of `master_roles`), so future
  master roles are pure config.
- Each mode has a **default citizen role** (`citizen_roles[mode]`), including new modes
  (e.g. `prod-admin` → `Admin`).
- `opsx use <account> --role <role>` **overrides** the citizen role for that switch,
  drawing from an open, extensible set of role names (no fixed count).
- Per-terminal isolation is preserved: distinct `(account, mode, citizen-role)` triples
  never share an AWS profile.

## Non-goals

- No per-account gating of which master role may reach which account (rejected earlier
  in favor of an explicit login mode).
- No new config *structure* (we extend the existing `master_roles` / `citizen_roles`
  maps rather than introducing a `modes:` block).
- `opsx kube` gains no `--role` (a cluster's account/citizen mapping is unchanged; only
  `opsx use` takes `--role` in this design).

## Design

### 1. Config-driven modes (master side)

- **Valid modes = the key set of `auth.master_roles`.** `admin` remains the default
  mode; `opr` keeps its `--opr` shorthand. Any other key (e.g. `prod-admin`) is selected
  with the existing `--mode <name>` flag.
- `master_roles` and `citizen_roles` **MUST have identical key sets**, and both MUST
  contain `admin` and `opr` (the default and the `--opr` shorthand target). This replaces
  the current "exactly [admin, opr]" rule.
- Mode tokens MUST be shell-safe (`[A-Za-z0-9._-]+`): a mode appears in the exported
  `AWS_PROFILE` value.

Example config:

```yaml
auth:
  master_roles:
    admin: master_admin
    opr: master_AWSOpr
    prod-admin: master_production_admin      # new
  citizen_roles:
    admin: Admin
    opr: AWSOpr
    prod-admin: Admin                         # default citizen role for prod-admin mode
```

### 2. Citizen role: per-mode default + `--role` override

- Default citizen role for a switch = `citizen_roles[mode]` (unchanged mechanism).
- `opsx use <account> --role <role>` overrides it. `<role>` is **free-form**, validated
  to `[A-Za-z0-9._-]+` — a strict subset of both valid IAM role-name characters and the
  shell-safe export charset, so new roles (`BAU`, …) need no config and the value can
  never inject into the exported profile name. (Consistent with the `--region` free-form
  choice.) An empty/whitespace/invalid value fails with a clear `invalid --role` error.
- `CitizenRoleARN(alias, mode, roleOverride string)` gains the override parameter: it uses
  the override when non-empty, else `citizen_roles[mode]`.

### 3. Profile naming / isolation (decided: always include the role)

Because the citizen role is no longer implied by `(alias, mode)`, the AWS profile name
MUST encode it, or two switches for the same `(alias, mode)` but different `--role` would
overwrite each other's cached credentials (silent cross-terminal collision).

- **Citizen profile = `<alias>.<mode>.<role>`**, always — where `<role>` is the effective
  citizen role (the `--role` override, else the mode default). Examples: `dev.admin.Admin`,
  `dev.opr.AWSOpr`, `dev.prod-admin.Admin`, `dev.admin.BAU`.
- `AWS_PROFILE` exports this longer name; it stays shell-safe (alnum, `.`, `_`, `-`).
- Profile names are never reverse-parsed for mode/alias (verified: `state.Entry` carries
  `Account`/`Mode`/`Cluster` explicitly), so the extra `.`-separated segment is safe even
  though aliases may contain `.`.

### 4. Master credential profile

`MasterProfile(mode)` is currently hardcoded (`admin`→`master_admin`, `opr`→`master_awsopr`).
Generalize it: keep `admin`/`opr` returning their current names (backward compatible for the
cached master creds), and return `master_<mode>` for any other mode (e.g.
`master_prod-admin`). This lets a third mode cache its own master session alongside the
existing two.

### 5. Validation changes (`config.Validate`)

- Replace the "both maps have admin and opr" loop with: `master_roles` and `citizen_roles`
  have identical, non-empty key sets; `admin` and `opr` are present; every mode token
  matches `[A-Za-z0-9._-]+`.
- `NormalizeMode` becomes config-aware (a method, or validated against the loaded config's
  mode set at the call sites) so `--mode prod-admin` is accepted only when configured.

### 6. Status / state / logout

- `state.Entry` already stores `Account`/`Mode`/`Cluster`; add nothing structural. The
  entry is keyed by the fuller profile name. `opsx status` still reads `AWS_PROFILE` and
  its state entry; it should additionally surface the effective citizen role (nice-to-have;
  the profile name already shows it).
- `opsx logout` keys off opsx-managed profiles; with `--all` it must match the new
  `<alias>.<mode>.<role>` names. Confirm its profile-enumeration covers the third segment.

## Affected code

- `internal/config/config.go` — `supportedModes`, `NormalizeMode`, `Validate` (mode set).
- `internal/config/arn.go` — `CitizenRoleARN` gains `roleOverride`; `MasterRoleARN` unchanged
  beyond mode validation.
- `internal/config/naming.go` — `MasterProfile` generalized; `CitizenProfile(alias, mode, role)`
  gains the role segment.
- `internal/creds/citizen.go` — thread the role override into the profile name + ARN.
- `internal/cli/use.go`, `internal/cli/shellswitch.go` — add `--role` flag to `use`; thread
  through `switchAccount`.
- `internal/cli/kube.go` — `switchCluster` passes no `--role` (uses the mode default).
- `internal/cli/logout.go` — verify `--all` enumeration matches new names.

## Migration / backward compatibility

- Profile names change for **every** citizen switch (`dev.admin` → `dev.admin.Admin`).
  Existing cached citizen profiles/state become orphaned; they are harmless and expire on
  their own (opsx creds are short-lived). Clean migration: `opsx logout --all` then
  re-login/use. Master profiles for `admin`/`opr` are unchanged.

## Testing (TDD, red first)

- `opsx login --mode prod-admin` assumes the config's `prod-admin` master role and caches
  its master profile.
- `opsx use dev` in `prod-admin` mode (no `--role`) → citizen role = `Admin`, profile
  `dev.prod-admin.Admin`.
- `opsx use dev --role BAU` → citizen ARN uses `BAU`, profile `dev.admin.BAU`; distinct from
  `dev.admin.Admin` (no collision).
- Invalid `--role` (metacharacters/whitespace/empty) → `invalid --role` error, nothing
  written/exported.
- `--mode` for an unconfigured mode → clear error.
- Config validation: mismatched `master_roles`/`citizen_roles` key sets, or missing
  `admin`/`opr`, are rejected with name-bearing errors.

## Spec / doc updates

- `openspec/specs/config/spec.md` — configurable mode set; identical key-set rule.
- `openspec/specs/account-switching/spec.md` — `--role` override + profile naming.
- `openspec/specs/entra-auth/spec.md` — `--mode <name>` selects the config master role.
- `README.md`, `USAGE.md`, `USAGE.zh-CN.md` — new mode + `--role` usage; migration note.

## Open questions

- Should `opsx status` print the effective citizen role explicitly (from the profile), or
  is the profile name enough? (Leaning: show it; cheap.)
