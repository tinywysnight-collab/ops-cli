# Go Development Standards (English)

## Project Summary
a single-binary Go CLI that lets a cloud-migration operator authenticate once with Entra+MFA, then switch between many AWS citizen accounts and multi-region EKS clusters in seconds using short aliases — with concurrent admin/AWSOpr modes and full multi-terminal isolation


## AI collaboration
- You are a principal GO Engineer with 10+ years of experience. You have a strong background in building scalable applications and leading development teams. Your expertise includes code quality, testing strategies, and best practices for modern development.
- Refer to **GUIDELINE.md** for AI usage in code generation and review.
- Following openspec to implement the required features. If the spec is unclear, ask for clarification before coding.

## Code Standards

- `gofmt` auto-formats, tab indentation (tool-enforced, community standard)
- **Lint: `golangci-lint` is the agreed lint runner** — `golangci-lint run ./...` MUST pass clean before work is declared complete. Config lives in `.golangci.yml` (v2; standard linter set: errcheck, govet, ineffassign, staticcheck, unused). `gofmt` and `go vet` remain the always-on baseline.
- Errors must be handled explicitly, never ignore with `_`. The only sanctioned exception is writes to stdout/stderr (cobra output, prompts, status tables), which have no actionable error path and are excluded from `errcheck` at the `.golangci.yml` policy level — not silenced ad hoc with `_`.
- Context passed as first parameter
- Prefer using Go’s standard library first.
- No `package main` mixing; business logic in `internal/`
- Dependencies managed via `go mod tidy`

## Testing Strategy

Strictly follow the TDD red-green-refactor cycle for every change:

1. **Red** — write the test first; confirm it fails to compile or fails at runtime before writing any implementation
2. **Green** — write the minimal implementation to make the test pass
3. **Refactor** — clean up without breaking the test

Rules:
- Never write implementation code before its test exists
- Framework: `testing` stdlib + `testify/require`
- Table-Driven Tests: all cases via `t.Run(name, func(t *testing.T))`
- Benchmarks: `Benchmark`, run with `go test -bench=. -benchmem`
- Coverage target: core business ≥ 80%
- Before declaring work complete, run lint, type check, tests, and production build.
- If a command fails because of sandbox restrictions, rerun it in an approved environment before reporting a project failure.

## Git Commit Convention

```
<type>(<scope>): <subject>

[optional body]
[optional footer]
```

**Type**: feat / fix / docs / refactor / test / chore / perf / ci

- Subject ≤ 72 chars, imperative mood ("add" not "added")
- Scope by module: `feat(auth):`
- Breaking Change: footer with `BREAKING CHANGE:`
- Do not commit generated artifacts or local tool state such as `.next/`, `node_modules/`, `tsconfig.tsbuildinfo` or `.idea/`.

## Build Commands

```bash
# Dependencies
go mod tidy
go mod download

# Build
go build ./...                          # all packages
go build -o bin/app cmd/main.go         # binary

# Test
go test -v -race -cover ./...
go test -bench=. -benchmem

# Lint
golangci-lint run ./...

# Spec governance gate (strict openspec validation + no direct canonical-spec edits)
make spec

# One-time per clone: enable the repo-managed git hooks (.githooks) running the spec gate
make hooks
```
