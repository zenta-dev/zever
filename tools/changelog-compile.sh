#!/usr/bin/env bash
# Compile .changelog/unreleased/*.md fragments into the changelog's pending
# section, then delete the compiled fragments.
#
# The pending section is `## [Unreleased]` when the changelog has one, else
# the top-most released section whose version has no git tag yet: the release
# checklist cuts `## [Unreleased]` to `## [vX.Y.Z] - YYYY-MM-DD` before the
# tags exist, and the fragments must land in that section instead of being
# stranded. This is the same pending rule tools/apidiff uses for its
# breaking-change gate, so both tools agree on what is still unreleased.
#
# Entries are inserted in fragment order (sorted by file name), every fragment
# exactly once. The compiled fragments are deleted; README.md is left alone.
# Idempotent: with no fragments it changes nothing.
#
# With --check, nothing is written: the changelog must carry a pending
# section, and the top section must be `## [Unreleased]`. The release cut
# RENAMES that section (it keeps an empty `## [Unreleased]` above the new
# release section); deleting it strands the next release: the fragments can
# no longer be compiled and the apidiff gate asks for a **Breaking:** entry
# in a section that does not exist. This check is the regression guard.
#
# Usage: tools/changelog-compile.sh [--check] [repo-root]
set -euo pipefail

check_only=0
if [ "${1:-}" = "--check" ]; then
  check_only=1
  shift
fi

root="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
changelog="$root/CHANGELOG.md"
fragdir="$root/.changelog/unreleased"

if [ ! -f "$changelog" ]; then
  echo "changelog-compile: $changelog not found" >&2
  exit 1
fi

# pending_header prints the header line of the section the fragments belong
# to, or nothing when the changelog has no pending section: `## [Unreleased]`
# when present, else the top-most released section whose version carries no
# git tag yet. A git failure counts as untagged, so the check degrades to the
# pending-section rule instead of stranding the fragments.
pending_header() {
  local header ver
  if grep -q '^## \[Unreleased\]' "$changelog"; then
    printf '%s\n' '## [Unreleased]'
    return 0
  fi
  header="$(awk '/^## \[/ && $0 !~ /^## \[Unreleased\]/ { gsub(/^[ \t]+|[ \t]+$/, "", $0); print; exit }' "$changelog")"
  if [ -z "$header" ]; then
    return 0
  fi
  ver="$(printf '%s\n' "$header" | sed -n 's/^## \[[vV]\{0,1\}\([0-9][0-9.]*\)\].*$/\1/p')"
  if [ -z "$ver" ]; then
    return 0
  fi
  if [ -n "$(git -C "$root" tag --list "v$ver" "*/v$ver" 2>/dev/null)" ]; then
    return 0
  fi
  printf '%s\n' "$header"
}

# check_pending_section asserts the invariant the release cut must keep: the
# changelog's top section is `## [Unreleased]`, so the next release has a
# pending section to cut and the fragments always have a home.
check_pending_section() {
  local first
  first="$(awk '/^## \[/ { gsub(/^[ \t]+|[ \t]+$/, "", $0); print; exit }' "$changelog")"
  if [ -z "$first" ]; then
    echo "changelog-compile: $changelog has no release section; add a ## [Unreleased] section at the top" >&2
    return 1
  fi
  case "$first" in
    "## [Unreleased]"*) ;;
    *)
      echo "changelog-compile: $changelog starts with '$first', not '## [Unreleased]'; the release cut must keep an empty ## [Unreleased] section above the new release section (see .github/CONTRIBUTING.md)" >&2
      return 1
      ;;
  esac
  if [ -z "$(pending_header)" ]; then
    echo "changelog-compile: no pending section in $changelog" >&2
    return 1
  fi
  echo "changelog-compile: pending section '## [Unreleased]' is present"
}

if [ "$check_only" -eq 1 ]; then
  check_pending_section
  exit 0
fi

shopt -s nullglob
frags=()
for f in "$fragdir"/*.md; do
  case "$(basename "$f")" in
    README.md) continue ;;
    *) frags+=("$f") ;;
  esac
done

if [ "${#frags[@]}" -eq 0 ]; then
  echo "changelog-compile: no fragments"
  exit 0
fi

target="$(pending_header)"
if [ -z "$target" ]; then
  echo "changelog-compile: no pending section in $changelog (no ## [Unreleased], and the top release is already tagged); add a ## [Unreleased] section" >&2
  exit 1
fi

# Sorted for deterministic output.
IFS=$'\n' frags=($(printf '%s\n' "${frags[@]}" | LC_ALL=C sort)); unset IFS

tmp="$(mktemp)"
{
  for f in "${frags[@]}"; do
    # Strip blank lines at fragment edges; keep interior content verbatim.
    awk 'NF { started = 1 } started { buf[++n] = $0 } END { while (n > 0 && buf[n] ~ /^[[:space:]]*$/) n--; for (i = 1; i <= n; i++) print buf[i] }' "$f"
    echo
  done
} > "$tmp.entries"

# Insert below the pending section header, swallowing the one blank line
# that used to separate the header from the old entries (the compiled
# fragments already end with a blank separator).
if ! awk -v entries="$tmp.entries" -v target="$target" '
  BEGIN { inserted = 0; skipblank = 0 }
  {
    trimmed = $0
    gsub(/^[ \t]+|[ \t]+$/, "", trimmed)
  }
  !inserted && trimmed == target {
    print
    while ((getline line < entries) > 0) print line
    close(entries)
    inserted = 1
    skipblank = 1
    next
  }
  skipblank && /^[[:space:]]*$/ { skipblank = 0; next }
  { skipblank = 0; print }
  END { if (!inserted) exit 3 }
' "$changelog" > "$changelog.new"; then
  echo "changelog-compile: no pending section ($target) in $changelog" >&2
  rm -f "$tmp.entries" "$changelog.new"
  exit 1
fi

mv "$changelog.new" "$changelog"
rm -f "$tmp.entries"
rm -f "${frags[@]}"
echo "changelog-compile: compiled ${#frags[@]} fragment(s) into $target"
