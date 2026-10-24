#!/usr/bin/env bash
# ci-env-fuzz-smoke.sh — short CI smoke over stream A's config .env fuzz targets.
#
# Runs each target FUZZTIME (default 60s). Not a soak: it catches regressions
# the committed seeds plus a minute of mutation can reach. A fuzzer-found
# crasher is written under config/testdata/fuzz as FAIL* and fails the job
# red: fix the bug, commit the regressing input with the fix (like the dsl
# fuzz-smoke job), and the next run continues from there.
#
# The fuzz tests themselves are owned by stream A (config/*_test.go); this
# script only wires the CI execution. Target list must track the Fuzz*
# functions in config: FuzzParseDotEnv, FuzzSetOptionField, FuzzMatchService,
# FuzzApplyEnvSecretScrub, FuzzResolve.
#
# Usage: FUZZTIME=60s bash tools/ci-env-fuzz-smoke.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FUZZTIME="${FUZZTIME:-60s}"

command -v go >/dev/null 2>&1 || { echo "ci-env-fuzz-smoke: go toolchain not on PATH" >&2; exit 1; }

for fn in FuzzParseDotEnv FuzzSetOptionField FuzzMatchService FuzzApplyEnvSecretScrub FuzzResolve; do
  echo "== config $fn ($FUZZTIME)"
  go -C "$ROOT/config" test -run '^$' -fuzz "^${fn}\$" -fuzztime "$FUZZTIME" .
done
echo "ci-env-fuzz-smoke: all config targets green for $FUZZTIME each"
