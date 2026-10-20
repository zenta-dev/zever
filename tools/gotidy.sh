#!/usr/bin/env bash
# `go mod tidy` for one module, run from inside the module directory.
# Usage: tools/gotidy.sh [-diff]
#
# Every module is tidied the same way, and it works offline:
#
#   1. Compute the transitive intra-repo closure of the module by parsing
#      the sibling go.mod files. No module graph is loaded, so siblings
#      that have no proxy release yet still resolve.
#   2. Add a TEMPORARY local `-replace` for every closure member the
#      committed go.mod does not replace yet. `go mod tidy` ignores
#      go.work, so without those replaces it resolves the pinned sibling
#      versions from the module proxy and fails for siblings that are not
#      released yet ("unknown revision <module>/vX.Y.Z"). A committed
#      replace is left alone: `go mod edit -replace` refuses an already
#      replaced module and `-dropreplace` would drop the committed entry.
#   3. Tidy, then drop the intra-repo requires that tidy added (they were
#      not in the committed file) and the temporary replaces again, so the
#      committed file stays proxy-resolvable for `go install` targets.
#      Direct intra-repo requires remain enforced by `make modgraph-check`.
#   4. With -diff, compare the result against the committed go.mod/go.sum
#      and restore both files, so CI never leaves the tree dirty.
#
# The go command runs with GOWORK=off: `go mod tidy` always works on a
# single module (it ignores the go.work workspace and the replaces it
# carries), and it refuses to run in a directory that is not one of the
# workspace modules. Every module, workspace member or not, therefore
# takes the same offline path.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
prefix="github.com/zenta-dev/zever/"

# `go mod tidy` always works on a single module: it ignores the go.work
# workspace and the replaces it carries, and it refuses to run in a
# directory that is not one of the workspace modules. Running it
# standalone keeps every module (workspace member or not) on the same
# offline path.
export GOWORK=off

# A require line: "<module> vX.Y.Z".
require_re="^[[:space:]]*(require[[:space:]]+)?(${prefix}[^[:space:]]+)[[:space:]]+v"
# A replace line: "<module> => <dir>". A versioned replace
# ("<module> vX.Y.Z => <dir>") is deliberately not matched: `go mod edit
# -dropreplace` drops every replace of that module, so such a module
# must never enter the temporary set.
replace_re="^[[:space:]]*(replace[[:space:]]+)?(${prefix}[^[:space:]]+)[[:space:]]*=>"

# The module being tidied: it is never replaced with its own directory.
self="$(sed -En 's|^module[[:space:]]+([^[:space:]]+).*|\1|p' go.mod | head -n 1)"

deps="$(sed -En "s|${require_re}.*|\2|p" go.mod | sort -u)"
if [ -z "$deps" ]; then
  exec go mod tidy "$@"
fi

# Expand deps to the transitive intra-repo closure by parsing sibling
# go.mod files. No module graph is loaded, so siblings without a proxy
# release yet (e.g. newly added leaf modules) still resolve.
deps="$(echo "$deps" | tr '\n' ' ')"
changed=1
while [ "$changed" = 1 ]; do
  changed=0
  for d in $deps; do
    dir="$root/${d#"$prefix"}"
    if [ -f "$dir/go.mod" ]; then
      sub="$(sed -En "s|${require_re}.*|\2|p" "$dir/go.mod" | sort -u)"
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
orig_requires="$(sed -En "s|${require_re}.*|\2|p" go.mod | sort -u)"

tmp="$(mktemp -d)"
cp go.mod "$tmp/go.mod"
if [ -f go.sum ]; then cp go.sum "$tmp/go.sum"; fi
restore() {
  cp "$tmp/go.mod" go.mod
  if [ -f "$tmp/go.sum" ]; then cp "$tmp/go.sum" go.sum; else rm -f go.sum; fi
}
trap 'rm -rf "$tmp"' EXIT

# The committed replaces stay untouched; only the closure members without
# one get a temporary local replace.
committed="$(sed -En "s|${replace_re}.*|\2|p" go.mod | sort -u | tr '\n' ' ')"
replace=()
drop=()
for d in $deps; do
  [ "$d" = "$self" ] && continue
  dir="$root/${d#"$prefix"}"
  [ -f "$dir/go.mod" ] || continue
  case " $committed " in
    *" $d "*) continue ;;
  esac
  replace+=("-replace=$d=$dir")
  drop+=("-dropreplace=$d")
done

if [ "${#replace[@]}" -gt 0 ]; then
  go mod edit "${replace[@]}"
fi
if ! go mod tidy; then
  restore
  exit 1
fi

# Drop tidy-added intra-repo requires that were not originally present,
# keeping the committed file proxy-resolvable (see header comment). Direct
# intra-repo requires stay enforced by `make modgraph-check`.
new_requires="$(sed -En "s|${require_re}.*|\2|p" go.mod | sort -u)"
orig_spaced="$(echo "$orig_requires" | tr '\n' ' ')"
for r in $new_requires; do
  case " $orig_spaced " in
    *" $r "*) ;;
    *) go mod edit "-droprequire=$r" ;;
  esac
done

# Drop the temporary replaces again, never a committed one.
if [ "${#drop[@]}" -gt 0 ]; then
  go mod edit "${drop[@]}"
fi

if [ "${1:-}" = "-diff" ]; then
  status=0
  if ! diff -u "$tmp/go.mod" go.mod || { [ -f "$tmp/go.sum" ] && ! diff -u "$tmp/go.sum" go.sum; }; then
    echo "go.mod/go.sum are not tidy; run: make tidy-all" >&2
    status=1
  fi
  restore
  exit "$status"
fi
