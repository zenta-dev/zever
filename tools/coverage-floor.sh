#!/usr/bin/env bash
# coverage-floor.sh — enforce a statement-coverage floor on security-critical
# packages (auth/crypto/payment). Codecov uploads are informational only;
# this gate hard-fails (exit 1) when any scoped package drops below the
# threshold.
#
# Input: coverprofiles under .coverage/ as produced by `make test-race-all`
# (one <module-slug>.out per module, `-covermode=atomic`). Aggregation is
# statement-weighted over the same profile data `go tool cover -func`
# reports; per-function display alone would mis-weight small functions.
#
# Scoped prefixes (repo-relative; profile paths carry the
# `github.com/zenta-dev/zever/` prefix):
#   core/auth/ core/crypto/ core/password/ core/authz/ core/payment/
#   adapters/payment/stripe/ adapters/payment/paddle/
# No `adapters/payment/*/webhook*` files exist (globbed); verification lives
# in the adapter roots (`WebhookEvent`), so the whole adapter dirs are scoped.
#
# Threshold 80 (not 85): measured 2026-09-30 via per-module
# `-covermode=atomic` runs, the floor was 83.0% (adapters/payment/stripe —
# uncovered Refund branches + 1-stmt Register wiring; WebhookEvent itself is
# fully covered). 85 would ship red-on-main; 80 keeps the gate green with a
# 3pt tripwire under the current minimum. Everything else scoped is >= 97%.
# Re-measure before raising: run the scoped modules, take the min, subtract
# headroom. Lowering the threshold needs maintainer review.
#
# Test-helper packages (final path component ending in `test`, e.g.
# core/auth/revocation/revocationtest, plus `testdata`) are excluded: they
# hold no production code and always read ~0% (helpers execute cross-package,
# invisible to their own profile). Fails closed otherwise: missing profiles
# or a scoped prefix with no production data both exit 1.
#
# Usage: tools/coverage-floor.sh [--threshold N] [--coverage-dir DIR]

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
THRESHOLD=80
COVERAGE_DIR="$ROOT/.coverage"

usage() {
  printf '%s\n' \
    "usage: $(basename "$0") [--threshold N] [--coverage-dir DIR]" \
    "  --threshold N       minimum percent per scoped package (default 80)" \
    "  --coverage-dir DIR  dir with *.out profiles (default <root>/.coverage)"
}

while (($# > 0)); do
  case "$1" in
    --threshold)
      THRESHOLD="${2:?missing value for --threshold}"
      shift 2
      ;;
    --coverage-dir)
      COVERAGE_DIR="${2:?missing value for --coverage-dir}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'unknown argument: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "$THRESHOLD" in
  ''|*[!0-9]*)
    printf 'threshold must be an integer 0-100, got %s\n' "$THRESHOLD" >&2
    exit 2
    ;;
esac
if [ "$THRESHOLD" -gt 100 ]; then
  printf 'threshold must be an integer 0-100, got %s\n' "$THRESHOLD" >&2
  exit 2
fi

SCOPED=(
  "core/auth/"
  "core/crypto/"
  "core/password/"
  "core/authz/"
  "core/payment/"
  "adapters/payment/stripe/"
  "adapters/payment/paddle/"
)

shopt -s nullglob
profiles=("$COVERAGE_DIR"/*.out)
shopt -u nullglob
if [ "${#profiles[@]}" -eq 0 ]; then
  printf 'coverage-floor: no *.out profiles in %s (run make test-race-all first)\n' "$COVERAGE_DIR" >&2
  exit 1
fi

agg="$(mktemp)"
# shellcheck disable=SC2064
trap "rm -f '$agg'" EXIT

# Per-directory statement totals: $1=file:range $2=stmts $3=count.
awk '
  $1 == "mode:" { next }
  {
    file = $1; sub(/:.*$/, "", file)
    dir = file; sub(/[^/]*$/, "", dir)
    stmts = $2 + 0; cnt = $3 + 0
    total[dir] += stmts
    if (cnt > 0) cov[dir] += stmts
  }
  END { for (d in total) printf "%s %d %d\n", d, total[d], cov[d] + 0 }
' "${profiles[@]}" | sort >"$agg"

printf 'coverage-floor: threshold %s%%, %s profile(s) in %s\n' "$THRESHOLD" "${#profiles[@]}" "$COVERAGE_DIR"
printf '%-72s %8s %8s %7s  %s\n' "PACKAGE" "STMTS" "COVERED" "COVER%" "STATUS"

fail=0
for scope in "${SCOPED[@]}"; do
  hits="$(grep -F -c "$scope" "$agg" || true)"
  if [ "$hits" -eq 0 ]; then
    printf '%-72s %8s %8s %7s  %s\n' "${scope%/}" "-" "-" "-" "NO DATA (fail closed)"
    fail=1
    continue
  fi
  shown=0
  while IFS=' ' read -r dir t c; do
    short="$dir"
    case "$short" in
      github.com/zenta-dev/zever/*) short="${short#github.com/zenta-dev/zever/}" ;;
    esac
    short="${short%/}"
    base="${short##*/}"
    case "$base" in
      *test|testdata) continue ;; # test-helper package: no production code
    esac
    shown=$((shown + 1))
    pct="$(awk -v cov="$c" -v tot="$t" 'BEGIN { if (tot == 0) printf "0.0"; else printf "%.1f", 100 * cov / tot }')"
    status="ok"
    if [ "$((c * 100))" -lt "$((THRESHOLD * t))" ]; then
      status="BELOW FLOOR"
      fail=1
    fi
    printf '%-72s %8s %8s %6s%%  %s\n' "$short" "$t" "$c" "$pct" "$status"
  done < <(grep -F "$scope" "$agg")
  if [ "$shown" -eq 0 ]; then
    printf '%-72s %8s %8s %7s  %s\n' "${scope%/}" "-" "-" "-" "helpers only (excluded)"
  fi
done

if [ "$fail" -ne 0 ]; then
  printf '%s\n' "coverage-floor: FAIL — scoped package(s) below ${THRESHOLD}%." \
    "Add tests for the uncovered statements; lowering the threshold needs maintainer review." >&2
  exit 1
fi
printf 'coverage-floor: PASS — all scoped packages >= %s%%\n' "$THRESHOLD"
