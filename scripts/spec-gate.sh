#!/usr/bin/env bash
# Spec governance gate for ops-cli.
#
# 1. Every openspec change and every canonical spec must pass strict validation.
# 2. Canonical specs (openspec/specs/) may only change in a commit or PR that
#    also touches openspec/changes/ — spec text moves through change deltas and
#    archive merges, never direct hand-edits (the lesson from ccbcfdc, which
#    flipped three Requirements with no change record).
#
# Usage: spec-gate.sh [--staged] [base-ref] [head-ref]
#   --staged  check the staged (index) diff instead of a commit range; used by
#             the pre-commit hook
#   base defaults to HEAD~1 locally, or origin/$GITHUB_BASE_REF in pull_request
#   events; head defaults to HEAD. When the base ref does not exist (first
#   commit of a branch), the range check is skipped after validation.
set -euo pipefail

cd "$(dirname "$0")/.."

staged=0
if [ "${1:-}" = "--staged" ]; then
	staged=1
	shift
fi

OPENSPEC_BIN="${OPENSPEC_BIN:-openspec}"
if ! command -v "$OPENSPEC_BIN" >/dev/null 2>&1; then
	echo "spec-gate: openspec CLI not found (brew install openspec, or set OPENSPEC_BIN)" >&2
	exit 1
fi

"$OPENSPEC_BIN" validate --all --strict --no-interactive

if [ "$staged" -eq 1 ]; then
	changed=$(git diff --cached --name-only)
else
	head_ref="${2:-HEAD}"
	if [ $# -ge 1 ]; then
		base_ref="$1"
	elif [ -n "${GITHUB_BASE_REF:-}" ]; then
		base_ref="origin/${GITHUB_BASE_REF}"
	else
		base_ref="HEAD~1"
	fi

	if ! git rev-parse --verify --quiet "$base_ref" >/dev/null; then
		echo "spec-gate: base ref ${base_ref} not found; skipping range check" >&2
		exit 0
	fi

	changed=$(git diff --name-only "$base_ref" "$head_ref")
fi
spec_changes=$(printf '%s\n' "$changed" | grep '^openspec/specs/' || true)
change_changes=$(printf '%s\n' "$changed" | grep '^openspec/changes/' || true)

if [ -n "$spec_changes" ] && [ -z "$change_changes" ]; then
	echo "spec-gate: canonical specs changed without any openspec/changes/ movement:" >&2
	printf '%s\n' "$spec_changes" | sed 's/^/  /' >&2
	echo "Canonical specs are only updated by archiving a change (openspec archive)." >&2
	echo "Create a change with deltas instead of editing openspec/specs/ directly." >&2
	exit 1
fi

echo "spec-gate: ok"
