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
# Usage: tools/changelog-compile.sh [repo-root]
set -euo pipefail

root="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
changelog="$root/CHANGELOG.md"
fragdir="$root/.changelog/unreleased"

if [ ! -f "$changelog" ]; then
  echo "changelog-compile: $changelog not found" >&2
  exit 1
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
