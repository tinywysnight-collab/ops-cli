# Master roles + `opsx use --role` citizen override — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the set of master roles config-driven (selectable via `--mode`, e.g. `prod-admin`) and let `opsx use --role <role>` override the citizen role independently of the mode, isolated by `<alias>.<mode>.<role>` profile names.

**Architecture:** `mode` becomes an open, config-defined key set (keys of `auth.master_roles`, which must match `auth.citizen_roles`; `admin`/`opr` stay special as default + `--opr`). Each mode keeps a default citizen role; `opsx use --role` overrides it. The citizen AWS profile name gains a role segment so distinct `(alias, mode, citizen-role)` triples never collide. The company-specific SAML fetch is untouched (it uses the role only for a debug line).

**Tech Stack:** Go, cobra, testify/require. Design: [docs/superpowers/specs/2026-07-02-master-roles-and-citizen-role-override-design.md](../specs/2026-07-02-master-roles-and-citizen-role-override-design.md).

**Verification env (ALWAYS):** tests may write `~/.kube/config`; run every `go`/lint command with an isolated HOME:
```bash
export HOME="$SCRATCH/fakehome"; mkdir -p "$HOME/.kube"; unset OPSX_DEFAULT_KUBECONFIG
```
where `SCRATCH=/private/tmp/claude-501/-Users-tinywang-WebstormProjects-ops-cli/0cd35387-99c3-40cc-b8ab-f715c12b0d6e/scratchpad`.

---

## Task 1: Config-driven mode set (NormalizeMode + validation)

**Files:**
- Modify: `internal/config/config.go` (`NormalizeMode`, `supportedModes` usage, `validateAuth`)
- Test: `internal/config/validate_test.go`, `internal/config/config_test.go`

- [ ] **Step 1: Write failing tests** in `internal/config/validate_test.go`

```go
func TestValidateAcceptsExtraMode(t *testing.T) {
	c := fullValid()
	c.Auth.MasterRoles["prod-admin"] = "master_production_admin"
	c.Auth.CitizenRoles["prod-admin"] = "Admin"
	require.NoError(t, c.Validate())
}

func TestValidateRejectsMismatchedRoleKeys(t *testing.T) {
	c := fullValid()
	c.Auth.MasterRoles["prod-admin"] = "master_production_admin" // no citizen_roles entry
	err := c.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "prod-admin")
}

func TestValidateRequiresAdminAndOpr(t *testing.T) {
	c := fullValid()
	delete(c.Auth.MasterRoles, "opr")
	delete(c.Auth.CitizenRoles, "opr")
	err := c.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "opr")
}

func TestNormalizeModeAcceptsConfiguredToken(t *testing.T) {
	got, err := config.NormalizeMode("prod-admin")
	require.NoError(t, err)
	require.Equal(t, "prod-admin", got)
}

func TestNormalizeModeRejectsUnsafeToken(t *testing.T) {
	_, err := config.NormalizeMode("bad;mode")
	require.Error(t, err)
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/config/ -run 'TestValidateAcceptsExtraMode|TestValidateRejects|TestValidateRequires|TestNormalizeMode' -v`
Expected: FAIL (NormalizeMode rejects prod-admin; validation still enforces exactly admin/opr).

- [ ] **Step 3: Generalize `NormalizeMode`** in `internal/config/config.go`

Replace the body of `NormalizeMode` with a shell-safe token check (keep `ModeAdmin`/`ModeOpr` constants; mode validity-vs-config is enforced by `Validate` + ARN lookups):

```go
// modeTokenPattern is the charset allowed for a mode token. A mode is used both
// as an exported AWS_PROFILE segment AND as a filesystem path segment
// (paths.KubeConfig joins kube/<mode>/...), so it must be shell-safe AND must
// exclude "." — that keeps "." reserved as the profile-name separator and
// prevents a mode like ".." from escaping the kube directory.
var modeTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// NormalizeMode validates a mode token's format. Whether the token is a
// configured mode is enforced by Validate (key sets) and the *RoleARN lookups.
func NormalizeMode(s string) (string, error) {
	if !modeTokenPattern.MatchString(s) {
		return "", fmt.Errorf("invalid mode %q: must match [A-Za-z0-9_-]+", s)
	}
	return s, nil
}
```

> Note: `modeTokenPattern` is also referenced in `validateAuth` (Step 4). Because it now excludes `.`, `paths.KubeConfig("dev", "syd.admin")` correctly errors again (the existing `TestKubeConfigEncodesDotAliasesUnambiguously` invariant), and `..` can never be a mode.

- [ ] **Step 4: Rewrite the mode loop in `validateAuth`** (`internal/config/config.go`, the `for _, mode := range supportedModes` block)

```go
	// master_roles and citizen_roles must define exactly the same modes, must
	// include admin (default) and opr (--opr shorthand), and every mode token
	// must be shell-safe. Additional modes (e.g. prod-admin) are allowed.
	if len(c.Auth.MasterRoles) == 0 {
		return fmt.Errorf("auth.master_roles is empty")
	}
	for _, mode := range supportedModes {
		if strings.TrimSpace(c.Auth.MasterRoles[mode]) == "" {
			return fmt.Errorf("auth.master_roles is missing required mode %q", mode)
		}
		if strings.TrimSpace(c.Auth.CitizenRoles[mode]) == "" {
			return fmt.Errorf("auth.citizen_roles is missing required mode %q", mode)
		}
	}
	for mode, role := range c.Auth.MasterRoles {
		if !modeTokenPattern.MatchString(mode) {
			return fmt.Errorf("auth.master_roles has invalid mode token %q", mode)
		}
		if strings.TrimSpace(role) == "" {
			return fmt.Errorf("auth.master_roles[%q] is blank", mode)
		}
		if strings.TrimSpace(c.Auth.CitizenRoles[mode]) == "" {
			return fmt.Errorf("mode %q is in master_roles but missing from citizen_roles", mode)
		}
	}
	for mode := range c.Auth.CitizenRoles {
		if strings.TrimSpace(c.Auth.MasterRoles[mode]) == "" {
			return fmt.Errorf("mode %q is in citizen_roles but missing from master_roles", mode)
		}
	}
```

Keep `supportedModes = []string{ModeAdmin, ModeOpr}` (now meaning "required modes").

- [ ] **Step 5: Update the existing `TestNormalizeMode`** in `internal/config/config_test.go:137`

The old cases assert `Admin` and `awsopr` are rejected (old "only admin/opr" rule). Under format-only validation they are valid *tokens* (configured-ness is enforced by `Validate`/ARN lookups). Replace the case table with:

```go
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"admin", "admin", false},
		{"opr", "opr", false},
		{"prod-admin", "prod-admin", false},
		{"Admin", "Admin", false}, // valid token; configured-ness checked elsewhere
		{"", "", true},
		{"bad;mode", "", true},
		{"has space", "", true},
	}
```

- [ ] **Step 6: Run tests to confirm pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (all config tests, including the new ones and the revised `TestNormalizeMode`).

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go internal/config/validate_test.go
git commit -m "feat(config): allow a config-driven set of modes beyond admin/opr"
```

---

## Task 2: Generalize `MasterProfile` for any mode

**Files:**
- Modify: `internal/config/naming.go`
- Test: `internal/config/naming_test.go` (create if absent)

- [ ] **Step 1: Write failing test** in `internal/config/naming_test.go`

```go
package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/config"
)

func TestMasterProfile(t *testing.T) {
	admin, err := config.MasterProfile("admin")
	require.NoError(t, err)
	require.Equal(t, "master_admin", admin)

	opr, err := config.MasterProfile("opr")
	require.NoError(t, err)
	require.Equal(t, "master_awsopr", opr)

	prod, err := config.MasterProfile("prod-admin")
	require.NoError(t, err)
	require.Equal(t, "master_prod-admin", prod)
}

func TestCitizenProfileIncludesRole(t *testing.T) {
	require.Equal(t, "dev.admin.Admin", config.CitizenProfile("dev", "admin", "Admin"))
	require.Equal(t, "dev.admin.BAU", config.CitizenProfile("dev", "admin", "BAU"))
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/config/ -run 'TestMasterProfile|TestCitizenProfileIncludesRole' -v`
Expected: FAIL to compile (`CitizenProfile` takes 2 args) / `MasterProfile("prod-admin")` errors.

- [ ] **Step 3: Rewrite `internal/config/naming.go`**

```go
package config

import "fmt"

// MasterProfile returns the cached master profile name for a mode. admin/opr
// keep their historical names for backward compatibility; any other configured
// mode uses master_<mode>.
func MasterProfile(mode string) (string, error) {
	if _, err := NormalizeMode(mode); err != nil {
		return "", err
	}
	switch mode {
	case ModeAdmin:
		return "master_admin", nil
	case ModeOpr:
		return "master_awsopr", nil
	default:
		return "master_" + mode, nil
	}
}

// CitizenProfile returns the citizen profile name for an account alias, mode,
// and effective citizen role: <alias>.<mode>.<role>. Encoding the role isolates
// distinct (alias, mode, citizen-role) triples so a --role override never
// overwrites another switch's cached credentials.
func CitizenProfile(alias, mode, role string) string {
	return fmt.Sprintf("%s.%s.%s", alias, mode, role)
}
```

- [ ] **Step 4: Fix compile fallout (temporary)** — `CitizenProfile` has one caller, `internal/creds/citizen.go:67`. Task 5 rewrites it properly; for now the build breaks only in creds, which Task 5 fixes. To keep Task 2 self-contained and green, temporarily update that call site to `config.CitizenProfile(alias, mode, "")` — Task 5 replaces it.

Run: `go test ./internal/config/ -run 'TestMasterProfile|TestCitizenProfileIncludesRole' -v`
Expected: PASS.

- [ ] **Step 5: Verify the whole module still builds**

Run: `go build ./...`
Expected: OK (creds compiles with the temporary `""` role).

- [ ] **Step 6: Commit**

```bash
git add internal/config/naming.go internal/config/naming_test.go internal/creds/citizen.go
git commit -m "feat(config): master_<mode> profiles and role-qualified citizen profiles"
```

---

## Task 3: Citizen role resolution + ARN override

**Files:**
- Modify: `internal/config/arn.go`
- Test: `internal/config/arn_test.go` (append)

- [ ] **Step 1: Write failing tests** in `internal/config/arn_test.go`

```go
func TestEffectiveCitizenRole(t *testing.T) {
	c := fullValid() // has citizen_roles admin:Admin, opr:AWSOpr
	got, err := c.EffectiveCitizenRole("admin", "")
	require.NoError(t, err)
	require.Equal(t, "Admin", got)

	got, err = c.EffectiveCitizenRole("admin", "BAU")
	require.NoError(t, err)
	require.Equal(t, "BAU", got)
}

func TestCitizenRoleARNWithOverride(t *testing.T) {
	c := fullValid()
	arn, err := c.CitizenRoleARN("dev", "admin", "BAU")
	require.NoError(t, err)
	require.Equal(t, "arn:aws:iam::111111111111:role/BAU", arn)

	arn, err = c.CitizenRoleARN("dev", "admin", "")
	require.NoError(t, err)
	require.Equal(t, "arn:aws:iam::111111111111:role/Admin", arn)
}
```

(If `fullValid()` lives in `validate_test.go` in the same `config_test` package, it is reusable here.)

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/config/ -run 'TestEffectiveCitizenRole|TestCitizenRoleARNWithOverride' -v`
Expected: FAIL to compile (`EffectiveCitizenRole` undefined; `CitizenRoleARN` takes 2 args).

- [ ] **Step 3: Update `internal/config/arn.go`**

Add `EffectiveCitizenRole` and give `CitizenRoleARN` an override parameter:

```go
// EffectiveCitizenRole returns the citizen role for a mode: the override when
// non-empty, else the configured default citizen_roles[mode]. The override is
// assumed pre-validated shell-safe by the caller (see cli.switchAccount).
func (c *Config) EffectiveCitizenRole(mode, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	role, ok := c.Auth.CitizenRoles[mode]
	if !ok || role == "" {
		return "", fmt.Errorf("auth.citizen_roles has no entry for mode %q", mode)
	}
	return role, nil
}

// CitizenRoleARN composes the citizen role ARN for an account alias, mode, and
// optional citizen-role override:
//
//	arn:aws:iam::{accounts[alias].account_id}:role/{EffectiveCitizenRole(mode, override)}
func (c *Config) CitizenRoleARN(alias, mode, override string) (string, error) {
	mode, err := NormalizeMode(mode)
	if err != nil {
		return "", err
	}
	acct, ok := c.Accounts[alias]
	if !ok {
		return "", fmt.Errorf("unknown account alias %q", alias)
	}
	if acct.AccountID == "" {
		return "", fmt.Errorf("account %q has no account_id", alias)
	}
	role, err := c.EffectiveCitizenRole(mode, override)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("arn:aws:iam::%s:role/%s", acct.AccountID, role), nil
}
```

- [ ] **Step 4: Fix the existing `CitizenRoleARN` caller** — `internal/creds/citizen.go:95` becomes `s.Cfg.CitizenRoleARN(alias, mode, "")` (Task 5 threads the real override). Update any existing `arn_test.go` callers of the old 2-arg form to pass `""`.

Run: `go test ./internal/config/ -run 'TestEffectiveCitizenRole|TestCitizenRoleARNWithOverride|TestCitizenRoleARN' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/arn.go internal/config/arn_test.go internal/creds/citizen.go
git commit -m "feat(config): citizen role override via CitizenRoleARN + EffectiveCitizenRole"
```

---

## Task 4: Generalize `MasterRole` to carry the mode (login for any mode)

**Files:**
- Modify: `internal/auth/provider.go`
- Test: `internal/auth/provider_test.go` (create/append)

Rationale: `FetchAssertion(role)` uses `role` only for a debug line; the real role ARN is chosen by `MasterRoleARN(mode)`. Make `MasterRole` mode-carrying so login works for any configured mode without touching the SAML flow.

- [ ] **Step 1: Write failing test** in `internal/auth/provider_test.go`

```go
package auth_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tinywysnight-collab/ops-cli/internal/auth"
)

func TestMasterRoleFromModeCarriesMode(t *testing.T) {
	r, err := auth.MasterRoleFromMode("prod-admin")
	require.NoError(t, err)
	require.Equal(t, "prod-admin", r.Mode())
	require.Equal(t, "prod-admin", r.String())
}
```

Also check existing `internal/auth/*_test.go` for uses of `auth.RoleAdmin` / `auth.RoleOpr`; update them to `auth.MasterRoleFromMode("admin"|"opr")` (values, discarding err with `require.NoError`) as part of Step 3.

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/auth/ -run TestMasterRoleFromModeCarriesMode -v`
Expected: FAIL (`MasterRoleFromMode("prod-admin")` errors under the current enum).

- [ ] **Step 3: Rewrite `MasterRole` in `internal/auth/provider.go`**

```go
// MasterRole identifies which master role a login targets. It carries the mode
// token; the concrete role ARN is composed by config.MasterRoleARN(mode).
type MasterRole struct{ mode string }

// Mode returns the mode token for the role.
func (r MasterRole) Mode() string { return r.mode }

// String implements fmt.Stringer.
func (r MasterRole) String() string { return r.mode }

// MasterRoleFromMode wraps a validated mode token as a MasterRole.
func MasterRoleFromMode(mode string) (MasterRole, error) {
	mode, err := config.NormalizeMode(mode)
	if err != nil {
		return MasterRole{}, err
	}
	return MasterRole{mode: mode}, nil
}
```

Remove the `RoleAdmin`/`RoleOpr` const block. Update any test/impl references (found in Step 1) to use `MasterRoleFromMode`. `entra.go`'s `FetchAssertion(ctx, role MasterRole)` signature is unchanged (it prints `role`).

- [ ] **Step 4: Run tests to confirm pass**

Run: `go test ./internal/auth/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/provider.go internal/auth/provider_test.go internal/auth/*_test.go
git commit -m "feat(auth): make MasterRole carry the mode token for config-driven modes"
```

---

## Task 5: Thread the citizen-role override through `creds.Use`

**Files:**
- Modify: `internal/creds/citizen.go` (`Use` signature + body)
- Test: `internal/creds/citizen_test.go` (append)

- [ ] **Step 1: Write failing test** in `internal/creds/citizen_test.go`

Mirror an existing `Use` test's setup (fake `Assume`, temp `Store`/`state.Store`). Assert the override changes the profile name and the assumed role ARN, and that override vs default do not collide:

```go
func TestUseRoleOverrideDistinctProfile(t *testing.T) {
	svc, capture := newTestCitizenService(t) // helper mirroring existing tests
	// default (mode admin -> Admin)
	p1, err := svc.Use(context.Background(), "dev", "admin", "")
	require.NoError(t, err)
	require.Equal(t, "dev.admin.Admin", p1)
	require.Contains(t, capture.lastRoleARN, ":role/Admin")

	// override -> BAU: distinct profile, distinct role ARN
	p2, err := svc.Use(context.Background(), "dev", "admin", "BAU")
	require.NoError(t, err)
	require.Equal(t, "dev.admin.BAU", p2)
	require.Contains(t, capture.lastRoleARN, ":role/BAU")
	require.NotEqual(t, p1, p2)
}
```

If the existing tests don't expose the assumed role ARN, capture it in the fake `Assume` (`func(_ ctx, _ Credentials, roleARN, _, _ string) {...}`) within the test.

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/creds/ -run TestUseRoleOverrideDistinctProfile -v`
Expected: FAIL to compile (`Use` takes 3 args).

- [ ] **Step 3: Update `Use` in `internal/creds/citizen.go`**

Signature and the two derived values:

```go
func (s *CitizenService) Use(ctx context.Context, alias, mode, roleOverride string) (string, error) {
	mode, err := config.NormalizeMode(mode)
	if err != nil {
		return "", err
	}
	if _, ok := s.Cfg.Accounts[alias]; !ok {
		return "", fmt.Errorf("unknown account alias %q", alias)
	}
	role, err := s.Cfg.EffectiveCitizenRole(mode, roleOverride)
	if err != nil {
		return "", err
	}
	profile := config.CitizenProfile(alias, mode, role)
	// ... unchanged lock/reuse block ...
```

And the ARN line (was `s.Cfg.CitizenRoleARN(alias, mode)`):

```go
	roleARN, err := s.Cfg.CitizenRoleARN(alias, mode, roleOverride)
```

Everything else (locking, reuse, state.Put with `Mode: mode`, `[default]` mirror) is unchanged. Note: `state.Entry.Mode` stays the mode (not the role); the role is visible in the profile name.

- [ ] **Step 4: Run tests to confirm pass**

Run: `go test ./internal/creds/ -v`
Expected: PASS (new test + existing, after Step 5 fixes callers).

- [ ] **Step 5: Fix `Use` callers** — the only non-test caller is `internal/cli/use.go` `switchAccount` (Task 6). Update existing `creds` tests that call `Use(ctx, alias, mode)` to pass a role arg (`""` for default). Re-run `go test ./internal/creds/ -v`.

- [ ] **Step 6: Commit**

```bash
git add internal/creds/citizen.go internal/creds/citizen_test.go
git commit -m "feat(creds): thread citizen role override through Use + profile name"
```

---

## Task 6: `opsx use --role` flag + threading in the CLI

**Files:**
- Modify: `internal/cli/use.go` (`switchAccount` signature + validation + `newUseCommand` flag)
- Modify: `internal/cli/shellswitch.go` (`newShellSwitchUseCommand` flag + call)
- Modify: `internal/cli/kube.go` (`switchCluster` call passes `""`)
- Test: `internal/cli/use_role_test.go` (create)

- [ ] **Step 1: Write failing tests** in `internal/cli/use_role_test.go`

```go
package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUseRoleOverrideProfile(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	run(t, "login")
	out := run(t, "shell-switch", "use", "dev", "--role", "BAU")
	require.Equal(t, "dev.admin.BAU", exportValue(out, "AWS_PROFILE"))
}

func TestUseDefaultRoleProfile(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	run(t, "login")
	out := run(t, "shell-switch", "use", "dev")
	require.Equal(t, "dev.admin.Admin", exportValue(out, "AWS_PROFILE"))
}

func TestUseRoleRejectsUnsafe(t *testing.T) {
	setupFakeEnv(t, integrationConfig)
	run(t, "login")
	_, _, err := runOutErr(t, "shell-switch", "use", "dev", "--role", "bad;rm -rf ~")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid --role")
}
```

Note: `integrationConfig` has `citizen_roles.admin: Admin`, so the default profile is `dev.admin.Admin`. (This changes the existing assertion in `TestIntegrationLoginUseKubeStatus` at `internal/cli/integration_test.go:157` from `dev.admin` to `dev.admin.Admin` — update it in Step 5.)

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/cli/ -run 'TestUseRoleOverrideProfile|TestUseDefaultRoleProfile|TestUseRoleRejectsUnsafe' -v`
Expected: FAIL (`--role` unknown flag; profiles lack the role segment).

- [ ] **Step 3: Update `switchAccount` in `internal/cli/use.go`**

Add a role-override param + validation (mirror `regionOverridePattern`); reuse `regionOverridePattern` charset via a sibling `roleOverridePattern`:

```go
// roleOverridePattern is the charset allowed for an explicit --role value: a
// strict subset of valid IAM role-name and shell-safe export characters, since
// the role becomes part of the exported AWS_PROFILE (<alias>.<mode>.<role>).
var roleOverridePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func switchAccount(ctx context.Context, alias, mode, regionOverride, roleOverride string) (profile, region string, err error) {
	if r := strings.TrimSpace(roleOverride); r != "" && !roleOverridePattern.MatchString(r) {
		return "", "", fmt.Errorf("invalid --role %q: must contain only letters, digits, '.', '_', '-'", roleOverride)
	}
	cfg, err := loadConfig()
	if err != nil {
		return "", "", err
	}
	cs, err := credStore()
	if err != nil {
		return "", "", err
	}
	ss, err := stateStore()
	if err != nil {
		return "", "", err
	}
	svc := &creds.CitizenService{Cfg: cfg, Creds: cs, State: ss, Assume: citizenAssumer}
	profile, err = svc.Use(ctx, alias, mode, strings.TrimSpace(roleOverride))
	if err != nil {
		return "", "", err
	}
	if r := strings.TrimSpace(regionOverride); r != "" {
		if !regionOverridePattern.MatchString(r) {
			return "", "", fmt.Errorf("invalid --region %q: must contain only letters, digits, '.', '_', '-'", regionOverride)
		}
		return profile, r, nil
	}
	region, err = cfg.ResolveCitizenRegion(alias)
	if err != nil {
		return "", "", err
	}
	return profile, region, nil
}
```

- [ ] **Step 4: Add the `--role` flag and pass it** in `internal/cli/use.go` `newUseCommand` and `internal/cli/shellswitch.go` `newShellSwitchUseCommand`.

`use.go` RunE:
```go
			regionOverride, _ := cmd.Flags().GetString("region")
			roleOverride, _ := cmd.Flags().GetString("role")
			profile, region, err := switchAccount(cmd.Context(), args[0], mode, regionOverride, roleOverride)
```
and after building the command:
```go
	cmd.Flags().String("role", "", "override the citizen role to assume (default: the mode's configured citizen role)")
```

`shellswitch.go` `newShellSwitchUseCommand` RunE:
```go
			regionOverride, _ := cmd.Flags().GetString("region")
			roleOverride, _ := cmd.Flags().GetString("role")
			profile, region, err := switchAccount(cmd.Context(), args[0], mode, regionOverride, roleOverride)
```
and:
```go
	cmd.Flags().String("role", "", "override the citizen role to assume (default: the mode's configured citizen role)")
```

- [ ] **Step 5: Fix the other `switchAccount` caller + stale assertions**

`internal/cli/kube.go` `switchCluster`: `switchAccount(ctx, cluster.Account, mode, "", "")`.
`internal/cli/integration_test.go:157`: change expected `AWS_PROFILE` from `dev.admin` to `dev.admin.Admin`. Scan `internal/cli/*_test.go` and `internal/cli/review_fifth_test.go` for other `dev.admin` profile-name assertions and update to `dev.admin.Admin` (state entry keys, status output). The recorded cluster/account/mode assertions are unaffected.

- [ ] **Step 6: Run tests to confirm pass**

Run: `go test ./internal/cli/ -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/use.go internal/cli/shellswitch.go internal/cli/kube.go internal/cli/use_role_test.go internal/cli/integration_test.go internal/cli/review_fifth_test.go
git commit -m "feat(cli): add opsx use --role to override the citizen role"
```

---

## Task 7: `logout` recognizes any configured mode

**Files:**
- Modify: `internal/cli/logout.go` (`isOpsxManagedEntry`)
- Test: `internal/cli/logout_test.go` (append)

- [ ] **Step 1: Write failing test** in `internal/cli/logout_test.go`

```go
func TestLogoutAllRemovesExtraModeProfiles(t *testing.T) {
	entries := map[string]state.Entry{
		"dev.prod-admin.Admin": {Account: "dev", Mode: "prod-admin"},
		"master_prod-admin":    {Account: "master", Mode: "prod-admin"},
	}
	got, err := logoutProfiles("admin", true, entries)
	require.NoError(t, err)
	require.Contains(t, got, "dev.prod-admin.Admin")
	require.Contains(t, got, "master_prod-admin")
}
```

(Match the existing `logout_test.go` package/imports; add `state` import if needed.)

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/cli/ -run TestLogoutAllRemovesExtraModeProfiles -v`
Expected: FAIL (`isOpsxManagedEntry` returns false for mode `prod-admin`).

- [ ] **Step 3: Fix `isOpsxManagedEntry`** in `internal/cli/logout.go`

```go
// isOpsxManagedEntry reports whether a state entry is opsx-managed. state.json
// holds only opsx's own entries and opsx always records a Mode, so any entry
// with a non-empty Mode is ours (covers admin, opr, and any configured mode).
func isOpsxManagedEntry(entry state.Entry) bool {
	return entry.Mode != ""
}
```

- [ ] **Step 4: Run tests to confirm pass**

Run: `go test ./internal/cli/ -run 'TestLogout' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/logout.go internal/cli/logout_test.go
git commit -m "fix(cli): logout recognizes any configured mode, not just admin/opr"
```

---

## Task 8: Specs + docs

**Files:**
- Modify: `openspec/specs/config/spec.md`, `openspec/specs/account-switching/spec.md`, `openspec/specs/entra-auth/spec.md`
- Modify: `README.md`, `USAGE.md`, `USAGE.zh-CN.md`

- [ ] **Step 1: config spec** — document that the mode set is the (identical) key set of `master_roles`/`citizen_roles`, must include `admin`+`opr`, tokens match `[A-Za-z0-9._-]+`; each mode has a default citizen role.

- [ ] **Step 2: account-switching spec** — add a requirement + scenarios: `opsx use --role <role>` overrides the mode's default citizen role; profile is `<alias>.<mode>.<role>`; `--role` is validated `[A-Za-z0-9._-]+`; invalid `--role` fails with `invalid --role`. Note distinct `(alias, mode, role)` never collide.

- [ ] **Step 3: entra-auth spec** — `opsx login --mode <name>` assumes `master_roles[<name>]` (config-driven); the SAML assertion fetch is role-agnostic (one Entra app), the role ARN is chosen at AssumeRoleWithSAML.

- [ ] **Step 4: README + USAGE(.zh-CN)** — add the `prod-admin` mode example (`opsx login --mode prod-admin`), the `opsx use dev --role BAU` example, and the migration note (citizen profile names gain a `.role` segment; run `opsx logout --all` or let creds expire).

- [ ] **Step 5: Commit**

```bash
git add openspec/specs README.md USAGE.md USAGE.zh-CN.md
git commit -m "docs: config-driven modes + opsx use --role"
```

---

## Task 9: Full verification + push

- [ ] **Step 1: Guarded full verification**

```bash
export HOME="$SCRATCH/fakehome"; mkdir -p "$HOME/.kube"; unset OPSX_DEFAULT_KUBECONFIG
go build ./...
go test -race ./...
golangci-lint run ./...
ls -la /Users/tinywang/.kube/config   # unchanged (13 bytes)
```
Expected: build OK, all tests pass, 0 lint issues, real kubeconfig untouched.

- [ ] **Step 2: Push**

```bash
git push origin main
```

---

## Notes / risks

- **Migration is breaking for citizen profile names** (`dev.admin` → `dev.admin.Admin`). Existing cached citizen creds/state are orphaned but harmless (short-lived; cleared by `opsx logout --all`). Master `admin`/`opr` profile names are unchanged.
- **`NormalizeMode` no longer gate-keeps configured-ness** — a bogus `--mode`/`--role` surfaces at the ARN lookup / validation with a clear, name-bearing error. Acceptable and tested (Task 1, Task 6).
- **SAML flow untouched:** the third master role reuses the same Entra assertion; only `MasterRoleARN(mode)` selects the ARN. No proxy-gated re-verification required for this change.
