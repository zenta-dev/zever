#!/usr/bin/env bash
# Lint CHANGELOG.md structure for `## [Unreleased]` and the newest release
# section (older history predates these rules and is not rewritten).
# Rules, per checked section:
#   1. each `### ` heading appears at most once;
#   2. a (non-backticked) **Breaking:** marker is only allowed as the prefix of
#      a list entry (`- **Breaking:** ...`), and every entry under
#      `### Breaking` carries it;
#   3. no list entry is a truncated duplicate of the entry right after it;
#   4. no list entry appears twice in the same section (the release notes
#      carry an entry exactly once);
#   5. continuation lines are indented by an even number of spaces.
# Usage: tools/changelog-lint.sh [CHANGELOG.md]
# Also lints .changelog/unreleased/*.md fragments (same entry rules), so
# parallel PRs can carry entries without touching CHANGELOG.md. A fragment
# has no "## [" header, so it is checked as a single section.
set -euo pipefail

file="${1:-$(cd "$(dirname "$0")/.." && pwd)/CHANGELOG.md}"
root="$(cd "$(dirname "$file")" && pwd)"
fragdir="$root/.changelog/unreleased"
if [ ! -f "$file" ]; then
  echo "error: $file not found" >&2
  exit 1
fi

lint_one() {
awk '
BEGIN { checked = 1 }
function fail(msg) { printf "%s:%d: %s\n", FILENAME, NR, msg; bad = 1 }
# flushentry closes the entry being read: its key is the entry lines joined
# with a literal "\n" (trailing whitespace stripped), so an entry that
# appears twice in the same section is detected verbatim.
function flushentry(   i, k, line) {
  if (en == 0) return
  k = ""
  for (i = 1; i <= en; i++) {
    line = ebuf[i]
    sub(/[ \t]+$/, "", line)
    k = k (i > 1 ? "\\n" : "") line
  }
  if (k in eseen) fail("duplicate entry in the same section: " ebuf[1])
  else eseen[k] = 1
  en = 0
}
/^## \[/ {
  flushentry()
  rel++
  checked = (rel <= 2)
  delete seen
  delete eseen
  sect = ""
  prev = ""
  next
}
!checked { next }
/^### / {
  flushentry()
  sect = substr($0, 5)
  if (sect in seen) fail("duplicate heading \"### " sect "\" in the same release section")
  seen[sect] = 1
  prev = ""
  next
}
/^- / {
  flushentry()
  ebuf[++en] = $0
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
/^[[:space:]]*$/ { flushentry(); next }
{
  match($0, /^ */)
  if (RLENGTH % 2 == 1) fail("continuation line indented by an odd number of spaces")
  if (RLENGTH > 0) ebuf[++en] = $0
  line = $0
  gsub(/`[^`]*`/, "", line)
  if (index(line, "**Breaking:**") > 0) fail("**Breaking:** must only appear as the entry prefix")
}
END { flushentry(); exit bad }
' "$1"
}

lint_one "$file"

if [ -d "$fragdir" ]; then
  for frag in "$fragdir"/*.md; do
    case "$(basename "$frag")" in
      README.md) continue ;;
    esac
    [ -e "$frag" ] || continue
    lint_one "$frag"
  done
fi

echo "changelog-lint: ok"
