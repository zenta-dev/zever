#!/usr/bin/env bash
# Prepare the repo for a lockstep release: set every hard-coded version to
# vX.Y.Z in one pass.
#   - cmd/zever/VERSION and every file listed in tools/release-pins.txt
#     (installers, CITATION.cff, docs pins, server versions, ...);
#   - every intra-repo `require github.com/zenta-dev/zever/... vX` in every
#     go.mod (tools/modgraph --set-pins; replace directives and go.work stay
#     untouched, so the workspace keeps resolving siblings locally);
#   - then `make deps-sync` (go mod tidy everywhere) and tools/check-version.sh.
# It does not touch CHANGELOG.md (cut `## [Unreleased]` yourself first) and
# never tags or pushes; run tools/tag-release.sh afterwards.
# Usage: tools/release-prep.sh vX.Y.Z   (or: make release-prep VERSION=vX.Y.Z)
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 vX.Y.Z" >&2
  exit 1
fi
ver="$1"
if ! printf '%s' "$ver" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "error: version must look like vX.Y.Z (got '$ver')" >&2
  exit 1
fi
bare="${ver#v}"
root="${ZEVER_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
cd "$root"

semver='[0-9]+\.[0-9]+\.[0-9]+'
pins="tools/release-pins.txt"
while IFS="$(printf '\t')" read -r path prefix; do
  case "$path" in ''|'#'*) continue ;; esac
  if [ ! -f "$path" ]; then
    echo "error: $path (listed in $pins) not found" >&2
    exit 1
  fi
  d=$(printf "\001")
  sed -E -i "s${d}(${prefix})${semver}${d}\\1${bare}${d}g" "$path"
done <"$pins"

go run ./tools/modgraph --set-pins --version "$ver"
make deps-sync
sh tools/check-version.sh
echo "release-prep: repo pinned at $ver; review 'git diff', commit, then run tools/tag-release.sh --dry-run $ver"
