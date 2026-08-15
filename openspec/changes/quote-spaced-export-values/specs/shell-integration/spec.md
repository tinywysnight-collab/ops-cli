## MODIFIED Requirements

### Requirement: Export values are shell-safe by validation
The system SHALL guarantee that every value emitted as `export KEY=value` is shell-safe by validating it, not by assuming it. Account/cluster aliases and the mode token SHALL be validated at config load against a strict charset (e.g. `^[A-Za-z0-9._-]+$`); the export emitter SHALL refuse any value containing shell metacharacters. This prevents a hostile or mistyped alias from injecting commands into the user's live shell via `eval`.

Every value opsx itself constructs and emits — in particular the `KUBECONFIG` path, whose file name encodes the cluster alias — MUST pass the export validator. The kubeconfig-alias encoding therefore MUST use only characters the emitter accepts as eval-safe. A config-valid cluster alias (including one containing `.`) MUST NOT produce a `KUBECONFIG` path that the emitter rejects; `opsx kube` failing for a dot-containing alias is a defect, because alias-dot encoding and the export charset must agree.

`KUBECONFIG` is the one value that may legitimately contain spaces (Windows user directories). Its validator charset is the strict charset plus the space character, and a spaced value SHALL be emitted single-quoted (POSIX `export KEY='...'`, PowerShell `$env:KEY = '...'`; cmd already quotes `set "KEY=..."`). Because the charset still forbids quotes and every metacharacter, this quoting introduces no escaping rules. Wrapper eval guards SHALL accept the quoted form with the same fail-closed semantics; all other keys keep the strict charsets unquoted.

#### Scenario: Dot-containing cluster alias still produces an eval-safe export
- **WHEN** a cluster alias contains `.` (e.g. `dev.syd`) and `opsx shell-switch kube dev.syd` runs
- **THEN** the emitted `export KUBECONFIG=<path>` passes the export validator and is safe to `eval`
- **AND** `opsx kube` does NOT fail with an "unsafe value" error for the encoded path

#### Scenario: Hostile alias is rejected, not exported
- **WHEN** a config alias contains shell metacharacters (e.g. `dev;rm -rf ~`)
- **THEN** config load (or the export emitter) fails with a clear error
- **AND** no `export` line containing the metacharacters is ever written to stdout

#### Scenario: Space-containing kubeconfig path is quoted and eval-safe
- **WHEN** the opsx kube directory lives under a path containing a space (e.g. `C:\Users\John Smith\...`) and a cluster switch emits `KUBECONFIG`
- **THEN** the value is emitted single-quoted (or inside `set "..."` for cmd), passes the wrapper guards, and `opsx kube` completes
- **AND** a spaced value for any other key (e.g. `AWS_PROFILE`) is still rejected
