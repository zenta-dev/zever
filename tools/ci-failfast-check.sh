#!/usr/bin/env bash
# ci-failfast-check.sh — local verification harness for the CI fail-fast work.
#
# Checks that .github/workflows/ci.yml implements the fail-fast layout, where
# fast jobs gate the expensive Tier-2 jobs instead of every matrix job running
# to completion behind `fail-fast: false`:
#
#   (a) no `fail-fast: false` remains in any matrix job (grep over
#       uncommented lines);
#   (b) a `fast-gate` job exists whose `needs:` covers fmt, lint and test;
#   (c) every Tier-2 job (test-race, conformance, fuzz-smoke, scaffold,
#       vulnerability, docs-audit) lists `fast-gate` in its `needs:`;
#   (d) `ci-gate` lists `fast-gate` in its `needs:`;
#   (e) the workflow file still parses as YAML.
#
# Hermetic: file reads, grep and awk only, plus a YAML parse via the first
# available of python3+PyYAML, ruby, or a throwaway Go module pinned to the
# local module cache (GOPROXY=off). No network.
#
# Usage: bash tools/ci-failfast-check.sh [--ci PATH]
# Exit 0 when every check passes, 1 otherwise.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CI="$ROOT/.github/workflows/ci.yml"
while [ $# -gt 0 ]; do
  case "$1" in
    --ci) CI="$2"; shift 2 ;;
    *) printf 'usage: %s [--ci PATH]\n' "$0" >&2; exit 2 ;;
  esac
done

if [ ! -f "$CI" ]; then
  printf 'FAIL (e) workflow file not found: %s\n' "$CI"
  exit 1
fi

PASS=0
FAIL=0
pass() { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
fail() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }

# needs_of JOB: print the raw `needs:` value (flow list and/or `- item`
# continuation lines joined with spaces) of the top-level job JOB.
needs_of() {
  awk -v job="$1" '
    /^  [A-Za-z0-9_-]+:/ {
      cur = $1; sub(/:$/, "", cur)
      injob = (cur == job); inneeds = 0; next
    }
    injob && /^    needs:/ {
      inneeds = 1
      line = $0; sub(/^    needs:[ \t]*/, "", line); buf = line; next
    }
    injob && inneeds && /^      - / {
      item = $0; sub(/^      - /, "", item); buf = buf " " item; next
    }
    injob && inneeds { inneeds = 0 }
    END { print buf }
  ' "$CI"
}

# has_token NEEDS TOKEN: exact token match (commas/brackets are separators,
# so `test` does not match `test-race`).
has_token() {
  printf '%s' "$1" | tr ',[]' '   ' | tr -s ' ' '\n' | grep -qx "$2"
}

# (a) no `fail-fast: false` remains (ignore commented lines).
# NOTE: the consumer uses `>/dev/null`, not `-q`: with `pipefail`, `grep -q`
# closes the pipe on the first match and the producer dies of SIGPIPE,
# flipping the pipeline status and the check outcome.
if grep -v '^[[:space:]]*#' "$CI" | grep 'fail-fast:[[:space:]]*false' >/dev/null; then
  remaining=$(grep -v '^[[:space:]]*#' "$CI" | grep -c 'fail-fast:[[:space:]]*false')
  fail "(a) $remaining uncommented 'fail-fast: false' line(s) remain"
else
  pass "(a) no 'fail-fast: false' remains"
fi

# (b) a `fast-gate` job exists with `needs:` covering fmt, lint and test.
if grep -q '^  fast-gate:' "$CI"; then
  gate_needs=$(needs_of fast-gate)
  missing=""
  for dep in fmt lint test; do
    has_token "$gate_needs" "$dep" || missing="$missing $dep"
  done
  if [ -z "$missing" ]; then
    pass "(b) fast-gate needs covers fmt/lint/test"
  else
    fail "(b) fast-gate needs is missing:$missing (needs:$gate_needs)"
  fi
else
  fail "(b) no 'fast-gate' job found"
fi

# (c) every Tier-2 job lists `fast-gate` in its `needs:`.
for job in test-race conformance fuzz-smoke scaffold vulnerability docs-audit; do
  if ! grep -q "^  $job:" "$CI"; then
    fail "(c) job '$job' not found"
    continue
  fi
  job_needs=$(needs_of "$job")
  if has_token "$job_needs" "fast-gate"; then
    pass "(c) '$job' needs fast-gate"
  else
    fail "(c) '$job' does not list fast-gate in needs (needs:$job_needs)"
  fi
done

# (d) `ci-gate` lists `fast-gate` in its `needs:`.
if ! grep -q '^  ci-gate:' "$CI"; then
  fail "(d) job 'ci-gate' not found"
else
  gate_needs=$(needs_of ci-gate)
  if has_token "$gate_needs" "fast-gate"; then
    pass "(d) ci-gate needs fast-gate"
  else
    fail "(d) ci-gate does not list fast-gate in needs"
  fi
fi

# (e) the workflow file still parses as YAML. First parser wins:
# python3+PyYAML, ruby, or a throwaway Go module using the local module
# cache (GOPROXY=off, GOWORK=off — no network).
yaml_status="none"
if python3 -c 'import yaml' >/dev/null 2>&1; then
  if python3 - "$CI" <<'PYEOF' >/dev/null 2>&1; then
import sys
import yaml
yaml.safe_load(open(sys.argv[1]))
PYEOF
    yaml_status="pass-python"
  else
    yaml_status="fail-python"
  fi
elif command -v ruby >/dev/null 2>&1; then
  if ruby -ryaml -e 'YAML.load_file(ARGV[0])' "$CI" >/dev/null 2>&1; then
    yaml_status="pass-ruby"
  else
    yaml_status="fail-ruby"
  fi
elif command -v go >/dev/null 2>&1; then
  TMPD="$(mktemp -d)"
  trap 'rm -rf "$TMPD"' EXIT INT TERM
  printf 'module cifailfastcheck\n\ngo 1.23\n\nrequire gopkg.in/yaml.v3 v3.0.1\n' >"$TMPD/go.mod"
  cat >"$TMPD/main.go" <<'GOEOF'
package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var v any
	if err := yaml.Unmarshal(b, &v); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
GOEOF
  if (cd "$TMPD" && GOPROXY=off GOSUMDB=off GOWORK=off GOTOOLCHAIN=local GOFLAGS=-mod=mod go mod tidy >/dev/null 2>&1) \
    && (cd "$TMPD" && GOPROXY=off GOSUMDB=off GOWORK=off GOTOOLCHAIN=local go run . "$CI" >/dev/null 2>&1); then
    yaml_status="pass-go"
  else
    yaml_status="fail-go"
  fi
  rm -rf "$TMPD"
  trap - EXIT INT TERM
fi
case "$yaml_status" in
  pass-*) pass "(e) workflow parses as YAML (${yaml_status#pass-})" ;;
  fail-*) fail "(e) workflow does not parse as YAML (${yaml_status#fail-})" ;;
  *) fail "(e) no YAML parser available (need python3+PyYAML, ruby, or go)" ;;
esac

printf 'ci-failfast-check: %d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
