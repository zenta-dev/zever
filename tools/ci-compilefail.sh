#!/usr/bin/env bash
# ci-compilefail.sh — gate proving typed-Ref misuse does not compile.
#
# Every directory under container/testdata/compilefail/* holding a go.mod is
# an own tiny module (excluded from go.work, ignored by the go tool via
# testdata) that MUST fail `go build`. A fixture that suddenly compiles
# means the type-level guard regressed (e.g. Provider unsealed, ProvideN
# inference loosened) and this script fails red.
#
# Built detached from the workspace (GOWORK=off) so the module graph
# resolves exactly as an out-of-tree consumer sees it. The proxy stays
# enabled so transitive public modules resolve like every other CI go
# command; the pinned go.sum files keep the build deterministic.
#
# Mirror: container/compilefail_test.go asserts the same fixtures plus the
# expected diagnostic per fixture (message-level); this script asserts the
# build outcome only (job-level). Never restructure stream B's fixtures
# here — adapt this loop if their layout changes.
#
# Usage: bash tools/ci-compilefail.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FIXTURE_ROOT="$ROOT/container/testdata/compilefail"

command -v go >/dev/null 2>&1 || { echo "ci-compilefail: go toolchain not on PATH" >&2; exit 1; }
[ -d "$FIXTURE_ROOT" ] || { echo "ci-compilefail: $FIXTURE_ROOT missing" >&2; exit 1; }

failures=0
count=0
for dir in "$FIXTURE_ROOT"/*/; do
  [ -f "$dir/go.mod" ] || continue
  name="$(basename "$dir")"
  count=$((count + 1))
  if GOWORK=off go -C "$dir" build ./... >/tmp/ci-compilefail-"$name".log 2>&1; then
    echo "ci-compilefail: FAIL $name compiled, want a compile failure"
    failures=$((failures + 1))
  else
    echo "ci-compilefail: ok $name fails to build as designed"
  fi
done

if [ "$count" -eq 0 ]; then
  echo "ci-compilefail: no fixtures found under $FIXTURE_ROOT" >&2
  exit 1
fi

if [ "$failures" -ne 0 ]; then
  echo "ci-compilefail: $failures/$count fixture(s) compiled; typed misuse is no longer rejected" >&2
  exit 1
fi
echo "ci-compilefail: all $count fixture(s) correctly fail to build"
