## Why
Windows user directories commonly contain spaces; the strict export charsets rejected those KUBECONFIG paths, so `opsx kube` generated a kubeconfig and then failed to emit the assignment — a half-completed switch.

## What Changes
- KUBECONFIG (the only opsx-constructed path value) may contain spaces: the value is validated against the strict charset plus the space character and emitted single-quoted (POSIX/PowerShell) or inside the existing `set "..."` quotes (cmd).
- No escaping rules are introduced: the charset still forbids quotes and every shell metacharacter, so single-quoting is escape-free by construction.
- Profile, region, and mode values keep the strict charsets; all four wrapper eval guards accept the quoted form with the same fail-closed semantics.

## Capabilities
### Modified Capabilities
- `shell-integration`: export values remain shell-safe by validation, now including space-containing paths.

## Impact
internal/shell (emit.go + zsh/bash/PowerShell/cmd wrapper guards). No behavior change for values without spaces.
