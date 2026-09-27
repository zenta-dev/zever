#!/usr/bin/env bash
# Tag every Go module in the repo for a release.
# Usage: tools/tag-release.sh v0.5.0
# Creates one tag per go.mod found via find: every module is tagged
# `<reldir>/<version>` where <reldir> is the module dir relative to the
# repo root (e.g. adapters/cache/redis/v0.5.0). There is no root module anymore,
# so no bare `<version>` tag is created.
# Prints a `git push --tags` hint at the end; never pushes.
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <version>  (e.g. $0 v0.5.0)" >&2
  exit 1
fi
ver="$1"
case "$ver" in
  v?*) ;;
  *)
    echo "error: version must start with 'v' (got '$ver')" >&2
    exit 1
    ;;
esac

created=0
skipped=0
for f in $(find . -type f -name go.mod -not -path "./.git/*" | sort); do
  mod=$(sed -n 's/^module[[:space:]]\+//p' "$f" | head -n 1)
  if [ -z "$mod" ]; then
    echo "warning: no module line in $f, skipping" >&2
    continue
  fi
  d=$(dirname "$f")
  if [ "$d" = "." ]; then
    continue # no root module: submodules tag as <reldir>/vX only
  else
    tag="${mod#github.com/zenta-dev/zever/}/$ver"
  fi
  if git rev-parse "$tag" >/dev/null 2>&1; then
    echo "exists, skipping: $tag"
    skipped=$((skipped + 1))
  else
    git tag "$tag"
    echo "tagged: $tag"
    created=$((created + 1))
  fi
done

echo "created $created tag(s), skipped $skipped existing."
echo "To publish, run: git push --tags"
