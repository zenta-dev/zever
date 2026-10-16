#!/usr/bin/env bash
# coverage-core.sh — enforce a per-package statement-coverage floor on every
# core/<battery> root package. The existing tools/coverage-floor.sh gates the
# security-critical subset at a single threshold; this gate instead pins each
# Core package at its own CURRENT measured level so any single-package
# regression fails, while unrelated improvements never break the build.
#
# Floors are integers (truncated from the measured `go test -cover`
# statement percentages on 2026-10-16, giving up to ~1pt of headroom against
# float noise). Ratchet-up-only: raising a floor needs no review, but
# lowering any floor needs maintainer review — same rule as
# tools/coverage-floor.sh.
#
# Scope is the module root package only (`go test -cover .` per core
# module): `*test` helper packages (aitest, cachetest, ...) hold no
# production code and always read low, exactly like the test-helper
# exclusion in tools/coverage-floor.sh. A core/ directory with no floor
# entry fails closed so new batteries must pin their level on arrival.
#
# Usage: tools/coverage-core.sh
# Locally: run from the repo root; needs no services (zero-infra defaults).

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Measured 2026-10-16 via `go -C core/<m> test -cover ./...` (root line),
# truncated to int. Re-measure before raising: take the root package's
# `coverage:` percent and truncate.
declare -A FLOORS=(
  [agent]=91
  [ai]=97
  [analytics]=98
  [auth]=98
  [authz]=100
  [billing]=99
  [cache]=96
  [cdn]=87
  [crypto]=98
  [db]=98
  [document]=98
  [eval]=64
  [eventbus]=99
  [flag]=97
  [geo]=98
  [i18n]=98
  [idempotency]=98
  [job]=98
  [lock]=92
  [log]=96
  [mailer]=98
  [media]=99
  [middleware]=98
  [notification]=98
  [observability]=98
  [outbox]=93
  [password]=97
  [payment]=95
  [permission]=98
  [queue]=95
  [rag]=91
  [ratelimit]=97
  [resilience]=89
  [router]=99
  [scheduler]=98
  [search]=98
  [secrets]=94
  [session]=98
  [storage]=99
  [tenant]=97
  [vectorstore]=98
  [webhook]=96
  [workflow]=92
)

printf '%-16s %8s %8s %7s  %s\n' "PACKAGE" "COVER%" "FLOOR%" "STATUS" ""

fail=0
checked=0
for dir in "$ROOT"/core/*/; do
  name="$(basename "$dir")"
  floor="${FLOORS[$name]:-}"
  if [ -z "$floor" ]; then
    printf '%-16s %8s %8s  %s\n' "$name" "-" "-" "NO FLOOR (fail closed: pin it in tools/coverage-core.sh)"
    fail=1
    continue
  fi

  out="$(go -C "$dir" test -cover -count=1 . 2>&1)" || {
    printf '%s\n' "$out" >&2
    printf '%-16s %8s %8s  %s\n' "$name" "-" "$floor" "TEST FAILURE"
    fail=1
    continue
  }

  pct="$(printf '%s\n' "$out" | grep -oE 'coverage: [0-9]+\.[0-9]+%' | head -1 | grep -oE '[0-9]+\.[0-9]+' || true)"
  if [ -z "$pct" ]; then
    printf '%-16s %8s %8s  %s\n' "$name" "-" "$floor" "NO COVERAGE (fail closed)"
    fail=1
    continue
  fi

  int="${pct%%.*}"
  checked=$((checked + 1))
  if [ "$int" -lt "$floor" ]; then
    printf '%-16s %7s%% %7s%%  %s\n' "$name" "$pct" "$floor" "BELOW FLOOR"
    fail=1
  else
    printf '%-16s %7s%% %7s%%  %s\n' "$name" "$pct" "$floor" "ok"
  fi
done

if [ "$fail" -ne 0 ]; then
  printf '%s\n' "coverage-core: FAIL — $checked package(s) checked. Add tests for the uncovered statements; lowering a floor needs maintainer review." >&2
  exit 1
fi
printf 'coverage-core: PASS — %s core package(s) at or above floor\n' "$checked"
