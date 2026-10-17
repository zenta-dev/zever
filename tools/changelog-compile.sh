#!/usr/bin/env bash
# Compile .changelog/unreleased/*.md fragments into CHANGELOG.md
# ## [Unreleased] (inserted directly below the header, sorted by filename),
# then delete the compiled fragments.
# Idempotent: with no fragments it changes nothing.
# Usage: tools/changelog-compile.sh [repo-root]
set -euo pipefail

root="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
changelog="$root/CHANGELOG.md"
fragdir="$root/.changelog/unreleased"

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

# Insert below the first "## [Unreleased]" line, swallowing the one blank
# line that used to separate the header from the old entries (the compiled
# fragments already end with a blank separator).
awk -v entries="$tmp.entries" '
  BEGIN { inserted = 0; skipblank = 0 }
  /^## \[Unreleased\]/ && !inserted {
    print
    while ((getline line < entries) > 0) print line
    close(entries)
    inserted = 1
    skipblank = 1
    next
  }
  skipblank && /^[[:space:]]*$/ { skipblank = 0; next }
  { skipblank = 0; print }
' "$changelog" > "$changelog.new"

if ! grep -q '^## \[Unreleased\]' "$changelog.new"; then
  echo "changelog-compile: no ## [Unreleased] section in $changelog" >&2
  rm -f "$tmp.entries" "$changelog.new"
  exit 1
fi

mv "$changelog.new" "$changelog"
rm -f "$tmp.entries"
rm -f "${frags[@]}"
echo "changelog-compile: compiled ${#frags[@]} fragment(s)"
