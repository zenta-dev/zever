#!/usr/bin/env bash
# changelog-compile-test.sh - offline tests for tools/changelog-compile.sh.
#
# Every scenario runs in a throwaway git repo under a temp dir, so the
# pending-section rule (the top released section counts while its version
# has no tag yet) is exercised against real tags. No network, no writes
# outside the temp dir.
#
# Usage: bash tools/changelog-compile-test.sh
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPILE="$ROOT/tools/changelog-compile.sh"
LINT="$ROOT/tools/changelog-lint.sh"

TMPD="$(mktemp -d)"
trap 'rm -rf "$TMPD"' EXIT INT TERM

PASS=0
FAIL=0

say() { printf '%s\n' "$*" >&2; }
ok() { say "ok - $1"; PASS=$((PASS + 1)); }
bad() { say "FAIL - $1"; FAIL=$((FAIL + 1)); }

assert_rc() {
    # assert_rc DESC EXPECTED_RC ACTUAL_RC
    if [ "$2" -eq "$3" ]; then ok "$1"; else bad "$1 (want rc=$2, got rc=$3)"; fi
}

assert_contains() {
    # assert_contains DESC HAYSTACK NEEDLE
    case "$2" in
        *"$3"*) ok "$1" ;;
        *) bad "$1 (output does not contain: $3)" ;;
    esac
}

assert_not_contains() {
    # assert_not_contains DESC HAYSTACK NEEDLE
    case "$2" in
        *"$3"*) bad "$1 (output unexpectedly contains: $3)" ;;
        *) ok "$1" ;;
    esac
}

assert_file() {
    # assert_file DESC PATH
    if [ -e "$2" ]; then ok "$1"; else bad "$1 (missing file: $2)"; fi
}

assert_no_file() {
    # assert_no_file DESC PATH
    if [ -e "$2" ]; then bad "$1 (file still there: $2)"; else ok "$1"; fi
}

# new_repo NAME: create a scratch git repo with a changelog and an empty
# fragment dir; prints its root.
new_repo() {
    root="$TMPD/$1"
    mkdir -p "$root/.changelog/unreleased"
    printf '%s\n' "$2" >"$root/CHANGELOG.md"
    git -C "$root" init -q .
    git -C "$root" add -A
    git -C "$root" -c user.email=test@example.com -c user.name=test commit -qm init
    printf '%s\n' "$root"
}

# --- scenario 1: cut-but-untagged top section ----------------------------------
say ""
say "changelog-compile: pending section is the untagged top release"

repo="$(new_repo untagged '# Changelog

## [v0.8.0] - 2026-10-18
- old entry
')"
printf -- '- feat(a): first entry\n' >"$repo/.changelog/unreleased/a-first.md"
printf -- '- feat(b): second entry\n' >"$repo/.changelog/unreleased/b-second.md"
printf -- '# fragments readme\n' >"$repo/.changelog/unreleased/README.md"

out="$(bash "$COMPILE" "$repo" 2>&1)"; rc=$?
assert_rc "compile succeeds" 0 "$rc"
assert_contains "compile names the pending section" "$out" "## [v0.8.0] - 2026-10-18"

changelog="$(cat "$repo/CHANGELOG.md")"
assert_contains "first entry compiled" "$changelog" "- feat(a): first entry"
assert_contains "second entry compiled" "$changelog" "- feat(b): second entry"
assert_contains "old entry kept" "$changelog" "- old entry"
assert_not_contains "no Unreleased section invented" "$changelog" "## [Unreleased]"

# Every entry exactly once, in fragment order (a-first before b-second).
count_a="$(printf '%s\n' "$changelog" | grep -c 'feat(a): first entry')"
count_b="$(printf '%s\n' "$changelog" | grep -c 'feat(b): second entry')"
if [ "$count_a" -eq 1 ] && [ "$count_b" -eq 1 ]; then
    ok "each entry compiled exactly once"
else
    bad "each entry compiled exactly once (a=$count_a, b=$count_b)"
fi

order="$(printf '%s\n' "$changelog" | grep -n 'feat(' | head -n 1)"
case "$order" in
    *":- feat(a): first entry"*) ok "fragment order is stable" ;;
    *) bad "fragment order is stable (got: $order)" ;;
esac

assert_no_file "compiled fragment deleted" "$repo/.changelog/unreleased/a-first.md"
assert_no_file "compiled fragment deleted" "$repo/.changelog/unreleased/b-second.md"
assert_file "README.md kept" "$repo/.changelog/unreleased/README.md"

lintout="$(bash "$LINT" "$repo/CHANGELOG.md" 2>&1)"; rc=$?
assert_rc "compiled changelog passes the lint" 0 "$rc"
assert_contains "lint reports ok" "$lintout" "changelog-lint: ok"

# --- scenario 2: ## [Unreleased] wins ------------------------------------------
say ""
say "changelog-compile: ## [Unreleased] is the pending section"

repo="$(new_repo unreleased '# Changelog

## [Unreleased]

## [v0.7.2] - 2026-10-17
- old entry
')"
printf -- '- feat(u): unreleased entry\n' >"$repo/.changelog/unreleased/u-entry.md"

out="$(bash "$COMPILE" "$repo" 2>&1)"; rc=$?
assert_rc "compile succeeds" 0 "$rc"
assert_contains "compile names the Unreleased section" "$out" "## [Unreleased]"

changelog="$(cat "$repo/CHANGELOG.md")"
assert_contains "entry compiled" "$changelog" "- feat(u): unreleased entry"
assert_contains "old entry kept" "$changelog" "- old entry"

unreleased_body="$(printf '%s\n' "$changelog" | sed -n '/^## \[Unreleased\]/,/^## \[v/p')"
case "$unreleased_body" in
    *"- feat(u): unreleased entry"*) ok "entry landed in the Unreleased section" ;;
    *) bad "entry landed in the Unreleased section" ;;
esac

# --- scenario 3: tagged top section is not pending ------------------------------
say ""
say "changelog-compile: a tagged top release has no pending section"

repo="$(new_repo tagged '# Changelog

## [v0.7.2] - 2026-10-17
- old entry
')"
git -C "$repo" tag v0.7.2
printf -- '- feat(t): tagged entry\n' >"$repo/.changelog/unreleased/t-entry.md"
before="$(cat "$repo/CHANGELOG.md")"

out="$(bash "$COMPILE" "$repo" 2>&1)"; rc=$?
assert_rc "compile fails on a tagged top release" 1 "$rc"
assert_contains "failure names the missing pending section" "$out" "no pending section"
assert_file "fragment kept on failure" "$repo/.changelog/unreleased/t-entry.md"
after="$(cat "$repo/CHANGELOG.md")"
if [ "$before" = "$after" ]; then
    ok "changelog untouched on failure"
else
    bad "changelog untouched on failure"
fi

# --- scenario 4: a per-module tag also marks the release as tagged --------------
say ""
say "changelog-compile: a per-module tag marks the release as tagged"

repo="$(new_repo moduletag '# Changelog

## [v0.8.0] - 2026-10-18
- old entry
')"
git -C "$repo" tag "cmd/zever/v0.8.0"
printf -- '- feat(m): module-tagged entry\n' >"$repo/.changelog/unreleased/m-entry.md"

out="$(bash "$COMPILE" "$repo" 2>&1)"; rc=$?
assert_rc "compile fails when only the module tag exists" 1 "$rc"
assert_file "fragment kept on failure" "$repo/.changelog/unreleased/m-entry.md"

# --- scenario 5: no fragments is a no-op ----------------------------------------
say ""
say "changelog-compile: no fragments changes nothing"

repo="$(new_repo nofrag '# Changelog

## [v0.8.0] - 2026-10-18
- old entry
')"
before="$(cat "$repo/CHANGELOG.md")"

out="$(bash "$COMPILE" "$repo" 2>&1)"; rc=$?
assert_rc "compile succeeds with no fragments" 0 "$rc"
assert_contains "compile reports no fragments" "$out" "no fragments"
after="$(cat "$repo/CHANGELOG.md")"
if [ "$before" = "$after" ]; then
    ok "changelog untouched without fragments"
else
    bad "changelog untouched without fragments"
fi

# --- scenario 6: the pending section is ## [Unreleased] ------------------------
say ""
say "changelog-compile: ## [Unreleased] stays the pending section after the cut"

# The release cut renames the pending section and leaves an empty
# ## [Unreleased] above it: the next release always has a pending section,
# tagged or not.
repo="$(new_repo cut '# Changelog

## [Unreleased]

## [v0.8.0] - 2026-10-18
- old entry
')"
git -C "$repo" tag v0.8.0
printf -- '- feat(c): after the cut\n' >"$repo/.changelog/unreleased/c-entry.md"

out="$(bash "$COMPILE" "$repo" 2>&1)"; rc=$?
assert_rc "compile succeeds on a tagged release with an Unreleased section" 0 "$rc"
assert_contains "compile targets the Unreleased section" "$out" "## [Unreleased]"

changelog="$(cat "$repo/CHANGELOG.md")"
assert_contains "entry compiled into the Unreleased section" "$changelog" "- feat(c): after the cut"
unreleased_body="$(printf '%s\n' "$changelog" | sed -n '/^## \[Unreleased\]/,/^## \[v/p')"
case "$unreleased_body" in
    *"- feat(c): after the cut"*) ok "entry landed in the Unreleased section" ;;
    *) bad "entry landed in the Unreleased section" ;;
esac
assert_contains "the released section is untouched" "$changelog" "## [v0.8.0] - 2026-10-18"

# --- scenario 7: --check asserts the pending section ----------------------------
say ""
say "changelog-compile: --check asserts the pending section"

repo="$(new_repo checkok '# Changelog

## [Unreleased]

## [v0.8.0] - 2026-10-18
- old entry
')"
out="$(bash "$COMPILE" --check "$repo" 2>&1)"; rc=$?
assert_rc "check passes with an Unreleased section" 0 "$rc"
assert_contains "check names the pending section" "$out" "## [Unreleased]"

# A changelog whose top section is a release (the v0.8.0 cut regression):
# the check must fail, or the next release cannot be cut.
repo="$(new_repo checkbad '# Changelog

## [v0.8.0] - 2026-10-18
- old entry
')"
out="$(bash "$COMPILE" --check "$repo" 2>&1)"; rc=$?
assert_rc "check fails without an Unreleased section" 1 "$rc"
assert_contains "failure names the missing section" "$out" "## [Unreleased]"

# The check never writes: the changelog must be untouched.
before="$(cat "$repo/CHANGELOG.md")"
bash "$COMPILE" --check "$repo" >/dev/null 2>&1 || true
after="$(cat "$repo/CHANGELOG.md")"
if [ "$before" = "$after" ]; then
    ok "check leaves the changelog untouched"
else
    bad "check leaves the changelog untouched"
fi

# --- summary -------------------------------------------------------------------
say ""
say "changelog-compile tests: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
exit 0
