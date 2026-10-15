#!/usr/bin/env bash
# Tag every Go module in the repo for a release.
# Usage: tools/tag-release.sh [--dry-run|--check|--verify] v0.6.0
# Creates one tag per tracked go.mod found via find, excluding nested
# worktrees, harness checkouts, and vendored copies: every module is tagged
# `<reldir>/<version>` where <reldir> is the module dir relative to the
# repo root (e.g. adapters/cache/redis/v0.5.0). There is no root module anymore,
# so no bare `<version>` tag is created.
# Tags are emitted in dependency order over three passes so leaf tags exist
# before dependents: (1) shared/*, (2) core/* plus config, container, orm,
# dsl and cmd/*, (3) adapters/* and everything else (examples, tools, docs).
# Within each pass, modules go alphabetically by go.mod path.
# With --dry-run (alias --check), list tags in order without creating them.
# With --verify, create nothing: confirm every module's tag exists locally or
# on origin (remote tag listing; skipped when no origin is configured) and
# exit 1 listing the missing ones. Used by the release docs.
# Before any mode runs, tools/check-version.sh must pass: every version pin and
# every intra-repo go.mod require equals cmd/zever/VERSION (== <version>).
# Prints a `git push --tags` hint at the end; never pushes.
set -euo pipefail

dry_run=0
verify=0
if [ "$#" -eq 2 ]; then
  case "$1" in
    --dry-run|--check) dry_run=1; shift ;;
    --verify) verify=1; shift ;;
    *)
      echo "usage: $0 [--dry-run|--check|--verify] <version>  (e.g. $0 v0.5.0)" >&2
      exit 1
      ;;
  esac
fi

if [ "$#" -ne 1 ]; then
  echo "usage: $0 [--dry-run|--check|--verify] <version>  (e.g. $0 v0.5.0)" >&2
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
missing=0
remote_tags=""
if [ "$verify" -eq 1 ] && git remote get-url origin >/dev/null 2>&1; then
  remote_tags=$(git ls-remote --tags origin 2>/dev/null | sed 's#.*refs/tags/##; s#\^{}$##' || true)
fi

# cmd/zever/VERSION is the CLI's fallback version (see cmd/zever/version.go).
# It must be committed before tagging so the tag carries it; update it here
# and stop so the change can be committed, then re-run.
version_file="cmd/zever/VERSION"
if [ -f "$version_file" ] && [ "$(tr -d '[:space:]' <"$version_file")" != "${ver#v}" ]; then
  if [ "$dry_run" -eq 1 ]; then
    echo "would update $version_file to ${ver#v} (commit it before tagging)"
  else
    printf '%s\n' "${ver#v}" >"$version_file"
    echo "updated $version_file to ${ver#v}; commit it, then re-run $0 $ver" >&2
    exit 1
  fi
fi

# Every hard-coded version (installers, CITATION.cff, docs pins, CHANGELOG
# top section) and every intra-repo go.mod require must equal the release
# version before anything is tagged or verified: published go.mod files
# cannot be fixed after the proxy caches them. Fix with `make release-prep`.
check_script="$(cd "$(dirname "$0")" && pwd)/check-version.sh"
if [ -f "$version_file" ] && [ -f "$check_script" ]; then
  ZEVER_ROOT="$PWD" sh "$check_script" || {
    echo "error: release pins do not match $ver; run: make release-prep VERSION=$ver" >&2
    exit 1
  }
fi

pass1=()
pass2=()
pass3=()
for f in $(find . -type f -name go.mod -not -path "./.git/*" -not -path "./.worktrees/*" -not -path "./.claude/*" -not -path "*/node_modules/*" | sort); do
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
  if [ "$verify" -eq 1 ]; then
    if git rev-parse -q --verify "refs/tags/$tag" >/dev/null 2>&1; then
      skipped=$((skipped + 1))
    elif printf '%s\n' "$remote_tags" | grep -qxF "$tag"; then
      skipped=$((skipped + 1))
    else
      echo "missing tag: $tag"
      missing=$((missing + 1))
    fi
  elif git rev-parse "$tag" >/dev/null 2>&1; then
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

if [ "$verify" -eq 1 ]; then
  if [ "$missing" -gt 0 ]; then
    echo "verify failed: $missing module tag(s) missing for $ver ($skipped present)." >&2
    exit 1
  fi
  echo "verify ok: all $skipped module tag(s) for $ver exist."
  exit 0
fi

if [ "$dry_run" -eq 1 ]; then
  echo "would create $created tag(s), $skipped existing (dry run: nothing created)."
else
  echo "created $created tag(s), skipped $skipped existing."
fi
echo "To publish, run: git push --tags"
