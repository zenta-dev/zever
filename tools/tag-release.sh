#!/usr/bin/env bash
# Tag every Go module in the repo for a release.
# Usage: tools/tag-release.sh [--dry-run|--check] v0.5.0
# Creates one tag per go.mod found via find: every module is tagged
# `<reldir>/<version>` where <reldir> is the module dir relative to the
# repo root (e.g. adapters/cache/redis/v0.5.0). There is no root module anymore,
# so no bare `<version>` tag is created.
# Tags are emitted in dependency order over three passes so leaf tags exist
# before dependents: (1) shared/*, (2) core/* plus config, container, orm,
# dsl and cmd/*, (3) adapters/* and everything else (examples, tools, docs).
# Within each pass, modules go alphabetically by go.mod path.
# With --dry-run (alias --check), list tags in order without creating them.
# Prints a `git push --tags` hint at the end; never pushes.
set -euo pipefail

dry_run=0
if [ "$#" -eq 2 ]; then
  case "$1" in
    --dry-run|--check) dry_run=1; shift ;;
    *)
      echo "usage: $0 [--dry-run|--check] <version>  (e.g. $0 v0.5.0)" >&2
      exit 1
      ;;
  esac
fi

if [ "$#" -ne 1 ]; then
  echo "usage: $0 [--dry-run|--check] <version>  (e.g. $0 v0.5.0)" >&2
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

pass1=()
pass2=()
pass3=()
for f in $(find . -type f -name go.mod -not -path "./.git/*" -not -path "./.worktrees/*" | sort); do
  d=$(dirname "$f")
  # Strip leading ./ for pass matching; root "." falls through to the
  # tag step, which skips it (no root module).
  rd="${d#./}"
  case "$rd" in
    shared/*) pass1+=("$f") ;;
    core/*|config|container|orm|dsl|cmd/*) pass2+=("$f") ;;
    *) pass3+=("$f") ;;
  esac
done

tag_one() {
  f="$1"
  mod=$(sed -n 's/^module[[:space:]]\+//p' "$f" | head -n 1)
  if [ -z "$mod" ]; then
    echo "warning: no module line in $f, skipping" >&2
    return
  fi
  d=$(dirname "$f")
  if [ "$d" = "." ]; then
    return # no root module: submodules tag as <reldir>/vX only
  else
    tag="${mod#github.com/zenta-dev/zever/}/$ver"
  fi
  if git rev-parse "$tag" >/dev/null 2>&1; then
    echo "exists, skipping: $tag"
    skipped=$((skipped + 1))
  elif [ "$dry_run" -eq 1 ]; then
    echo "would tag: $tag"
    created=$((created + 1))
  else
    git tag "$tag"
    echo "tagged: $tag"
    created=$((created + 1))
  fi
}

for f in "${pass1[@]}"; do
  tag_one "$f"
done
for f in "${pass2[@]}"; do
  tag_one "$f"
done
for f in "${pass3[@]}"; do
  tag_one "$f"
done

if [ "$dry_run" -eq 1 ]; then
  echo "would create $created tag(s), $skipped existing (dry run: nothing created)."
else
  echo "created $created tag(s), skipped $skipped existing."
fi
echo "To publish, run: git push --tags"
