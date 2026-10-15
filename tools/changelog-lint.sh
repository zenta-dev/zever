#!/usr/bin/env bash
# Lint CHANGELOG.md structure for `## [Unreleased]` and the newest release
# section (older history predates these rules and is not rewritten).
# Rules, per checked section:
#   1. each `### ` heading appears at most once;
#   2. a (non-backticked) **Breaking:** marker is only allowed as the prefix of
#      a list entry (`- **Breaking:** ...`), and every entry under
#      `### Breaking` carries it;
#   3. no list entry is a truncated duplicate of the entry right after it;
#   4. continuation lines are indented by an even number of spaces.
# Usage: tools/changelog-lint.sh [CHANGELOG.md]
set -euo pipefail

file="${1:-$(cd "$(dirname "$0")/.." && pwd)/CHANGELOG.md}"
if [ ! -f "$file" ]; then
  echo "error: $file not found" >&2
  exit 1
fi

awk '
function fail(msg) { printf "%s:%d: %s\n", FILENAME, NR, msg; bad = 1 }
/^## \[/ {
  rel++
  checked = (rel <= 2)
  delete seen
  sect = ""
  prev = ""
  next
}
!checked { next }
/^### / {
  sect = substr($0, 5)
  if (sect in seen) fail("duplicate heading \"### " sect "\" in the same release section")
  seen[sect] = 1
  prev = ""
  next
}
/^- / {
  text = substr($0, 3)
  if (prev != "" && index(text, prev) == 1 && text != prev) fail("truncated duplicate entry: " prev)
  prev = text
  if (sect == "Breaking" && index($0, "- **Breaking:** ") != 1) fail("entry under ### Breaking lacks the **Breaking:** prefix")
  line = $0
  gsub(/`[^`]*`/, "", line)
  rest = line
  if (index(rest, "- **Breaking:** ") == 1) rest = substr(rest, 17)
  if (index(rest, "**Breaking:**") > 0) fail("**Breaking:** must only appear as the entry prefix")
  next
}
/^[[:space:]]*$/ { next }
{
  match($0, /^ */)
  if (RLENGTH % 2 == 1) fail("continuation line indented by an odd number of spaces")
  line = $0
  gsub(/`[^`]*`/, "", line)
  if (index(line, "**Breaking:**") > 0) fail("**Breaking:** must only appear as the entry prefix")
}
END { exit bad }
' "$file" && echo "changelog-lint: ok"
