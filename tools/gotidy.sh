#!/usr/bin/env bash
# `go mod tidy` for one module, run from inside the module directory.
# Usage: tools/gotidy.sh [-diff]
#
# Modules that commit no intra-repo `replace` directives (the go install
# targets, see noReplaceModules in tools/modgraph) cannot be tidied as-is:
# tidy ignores go.work, so it would resolve the pinned sibling versions from
# the module proxy, which fails for siblings that are not released yet. For
# those modules this script adds temporary replaces pointing at the local
# siblings, tidies, drops the temporary replaces again and (with -diff)
# compares against the original go.mod/go.sum, restoring them afterwards.
# The temporary set covers the transitive intra-repo closure (parsed from
# sibling go.mod files without loading the module graph), so newly added
# sibling modules resolve locally too. Tidy records the closure in new
# indirect requires; those are dropped again before the diff because the
# committed file must stay proxy-resolvable for `go install` targets:
# direct intra-repo requires remain enforced by `make modgraph-check`.
# Modules that still carry replaces get a plain `go mod tidy`.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
prefix="github.com/zenta-dev/zever/"

if grep -Eq '^[[:space:]]*replace[[:space:]]' go.mod; then
  exec go mod tidy "$@"
fi

deps="$(sed -En "s|^[[:space:]]*(require[[:space:]]+)?(${prefix}[^[:space:]]+)[[:space:]]+v.*|\2|p" go.mod | sort -u)"
if [ -z "$deps" ]; then
  exec go mod tidy "$@"
fi

# Expand deps to the transitive intra-repo closure by parsing sibling
# go.mod files. No module graph is loaded, so siblings without a proxy
# release yet (e.g. newly added leaf modules) still resolve.
deps="$(sed -En "s|^[[:space:]]*(require[[:space:]]+)?(${prefix}[^[:space:]]+)[[:space:]]+v.*|\2|p" go.mod | sort -u)"
deps="$(echo "$deps" | tr '\n' ' ')"
changed=1
while [ "$changed" = 1 ]; do
  changed=0
  for d in $deps; do
    dir="$root/${d#"$prefix"}"
    if [ -f "$dir/go.mod" ]; then
      sub="$(sed -En "s|^[[:space:]]*(require[[:space:]]+)?(${prefix}[^[:space:]]+)[[:space:]]+v.*|\2|p" "$dir/go.mod" | sort -u)"
      for s in $sub; do
        case " $deps " in
          *" $s "*) ;;
          *) deps="$deps $s"; changed=1 ;;
        esac
      done
    fi
  done
done

# Snapshot the committed intra-repo requires: tidy records the closure in
# new indirect requires, which are dropped again below so the committed
# file keeps resolving from the proxy for `go install` targets.
orig_requires="$(sed -En "s|^[[:space:]]*(require[[:space:]]+)?(${prefix}[^[:space:]]+)[[:space:]]+v.*|\2|p" go.mod | sort -u)"

tmp="$(mktemp -d)"
cp go.mod "$tmp/go.mod"
if [ -f go.sum ]; then cp go.sum "$tmp/go.sum"; fi
restore() {
  cp "$tmp/go.mod" go.mod
  if [ -f "$tmp/go.sum" ]; then cp "$tmp/go.sum" go.sum; else rm -f go.sum; fi
}
trap 'rm -rf "$tmp"' EXIT

replace=()
drop=()
for d in $deps; do
  dir="$root/${d#"$prefix"}"
  if [ -f "$dir/go.mod" ]; then
    replace+=("-replace=$d=$dir")
    drop+=("-dropreplace=$d")
  fi
done

go mod edit "${replace[@]}"
if ! go mod tidy; then
  restore
  exit 1
fi

# Drop tidy-added intra-repo requires that were not originally present,
# keeping the committed file proxy-resolvable (see header comment). Direct
# intra-repo requires stay enforced by `make modgraph-check`.
new_requires="$(sed -En "s|^[[:space:]]*(require[[:space:]]+)?(${prefix}[^[:space:]]+)[[:space:]]+v.*|\2|p" go.mod | sort -u)"
orig_spaced="$(echo "$orig_requires" | tr '\n' ' ')"
for r in $new_requires; do
  case " $orig_spaced " in
    *" $r "*) ;;
    *) go mod edit "-droprequire=$r" ;;
  esac
done

go mod edit "${drop[@]}"

if [ "${1:-}" = "-diff" ]; then
  status=0
  if ! diff -u "$tmp/go.mod" go.mod || { [ -f "$tmp/go.sum" ] && ! diff -u "$tmp/go.sum" go.sum; }; then
    echo "go.mod/go.sum are not tidy; run: make tidy-all" >&2
    status=1
  fi
  restore
  exit "$status"
fi
